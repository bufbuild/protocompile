// Copyright 2020-2025 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ir

import (
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/arena"
)

// walker is the state struct for the AST-walking logic.
type walker struct {
	File
	*report.Report

	pkg FullName
}

// walk is the first step in constructing an IR module: converting an AST file
// into a type-and-field graph, and recording the AST source for each structure.
//
// This operation performs no name resolution.
func (w *walker) walk() {
	c := w.Context()

	if pkg := w.AST().Package(); !pkg.IsZero() {
		c.pkg = c.session.intern.Intern(pkg.Path().Canonicalized())
	}
	w.pkg = w.Package()

	c.syntax = syntax.Proto2
	if syn := w.AST().Syntax(); !syn.IsZero() {
		text := syn.Value().Span().Text()
		if unquoted := syn.Value().AsLiteral().AsString(); !unquoted.IsZero() {
			text = unquoted.Text()
		}

		// NOTE: This matches fallback behavior in parser/legalize_file.go.
		c.syntax = syntax.Lookup(text)
		if c.syntax == syntax.Unknown {
			if syn.IsEdition() {
				// If they wrote edition = "garbage" they probably want *an*
				// edition, so we pick the oldest one.
				c.syntax = syntax.Edition2023
			} else {
				c.syntax = syntax.Proto2
			}
		}
	}

	w.recurse(w.AST().AsAny(), nil)
}

type extend struct {
	parent   Type
	extendee arena.Pointer[rawExtendee]
}

type oneof struct {
	parent Type
	Oneof
}

func extractParentType(parent any) Type {
	switch parent := parent.(type) {
	case Type:
		return parent
	case extend:
		return parent.parent
	case oneof:
		return parent.parent
	default:
		return Type{}
	}
}

// recurse recursively walks the AST to build the out all of the types in a file.
func (w *walker) recurse(decl ast.DeclAny, parent any) {
	switch decl.Kind() {
	case ast.DeclKindBody:
		for decl := range seq.Values(decl.AsBody().Decls()) {
			w.recurse(decl, parent)
		}

	case ast.DeclKindRange:
		// Handled in NewType.

	case ast.DeclKindDef:
		def := decl.AsDef()
		if def.IsCorrupt() {
			return
		}

		switch kind := def.Classify(); kind {
		case ast.DefKindMessage, ast.DefKindEnum, ast.DefKindGroup:
			ty := w.newType(def, parent)

			if kind == ast.DefKindGroup {
				w.newField(def, parent, true)
			}

			w.recurse(def.Body().AsAny(), ty)

		case ast.DefKindField, ast.DefKindEnumValue:
			w.newField(def, parent, false)

		case ast.DefKindOneof:
			parent := extractParentType(parent)
			if parent.IsZero() {
				return // Already diagnosed elsewhere.
			}

			oneofDef := def.AsOneof()
			w.recurse(def.Body().AsAny(), oneof{
				parent: extractParentType(parent),
				Oneof:  w.newOneof(oneofDef, parent),
			})

		case ast.DefKindExtend:
			w.recurse(def.Body().AsAny(), extend{
				parent:   extractParentType(parent),
				extendee: w.newExtendee(def.AsExtend(), parent),
			})

		case ast.DefKindService:
			service := w.newService(def, parent)
			if service.IsZero() {
				break
			}

			w.recurse(def.Body().AsAny(), service)

		case ast.DefKindMethod:
			w.newMethod(def, parent)

		case ast.DefKindOption:
			// Options are lowered elsewhere.
		}
	}
}

func (w *walker) newType(def ast.DeclDef, parent any) Type {
	c := w.Context()
	parentTy := extractParentType(parent)

	name := def.Name().AsIdent().Name()
	fqn := w.fullname(parentTy, name)

	var visibility token.ID
	for prefix := range def.Type().Prefixes() {
		switch prefix.Prefix() {
		case keyword.Local, keyword.Export:
			visibility = prefix.PrefixToken().ID()
		default:
			continue
		}
		break
	}

	isEnum := def.Keyword() == keyword.Enum
	raw := c.arenas.types.NewCompressed(rawType{
		def:    def,
		name:   c.session.intern.Intern(name),
		fqn:    c.session.intern.Intern(fqn),
		parent: c.arenas.types.Compress(parentTy.raw),

		isEnum:     isEnum,
		visibility: visibility,
	})

	ty := Type{internal.NewWith(w.Context()), c.arenas.types.Deref(raw)}
	ty.raw.memberByName = sync.OnceValue(ty.makeMembersByName)

	for decl := range seq.Values(def.Body().Decls()) {
		rangeDecl := decl.AsRange()
		if rangeDecl.IsZero() {
			continue
		}

		for v := range seq.Values(rangeDecl.Ranges()) {
			if !v.AsPath().AsIdent().IsZero() || v.AsLiteral().Kind() == token.String {
				var name string
				if id := v.AsPath().AsIdent(); !id.IsZero() {
					name = id.Text()
				} else {
					name = v.AsLiteral().AsString().Text()
				}

				ty.raw.reservedNames = append(ty.raw.reservedNames, rawReservedName{
					ast:  v,
					name: ty.Context().session.intern.Intern(name),
				})
				continue
			}

			raw := w.Context().arenas.ranges.NewCompressed(rawReservedRange{
				decl:          rangeDecl,
				value:         v,
				forExtensions: rangeDecl.IsExtensions(),
			})

			if rangeDecl.IsReserved() {
				ty.raw.ranges = slices.Insert(ty.raw.ranges, int(ty.raw.rangesExtnStart), raw)
				ty.raw.rangesExtnStart++
			} else {
				ty.raw.ranges = append(ty.raw.ranges, raw)
			}
		}
	}

	if !parentTy.IsZero() {
		parentTy.raw.nested = append(parentTy.raw.nested, raw)
		c.types = append(c.types, raw)
	} else {
		c.types = slices.Insert(c.types, c.topLevelTypesEnd, raw)
		c.topLevelTypesEnd++
	}

	return ty
}

//nolint:unparam // Complains about the return value for some reason.
func (w *walker) newField(def ast.DeclDef, parent any, group bool) Member {
	c := w.Context()
	parentTy := extractParentType(parent)
	name := def.Name().AsIdent().Name()
	if group {
		name = strings.ToLower(name)
	}
	fqn := w.fullname(parentTy, name)

	id := c.arenas.members.NewCompressed(rawMember{
		def:     def,
		name:    c.session.intern.Intern(name),
		fqn:     c.session.intern.Intern(fqn),
		parent:  c.arenas.types.Compress(parentTy.raw),
		oneof:   math.MinInt32,
		isGroup: group,
	})
	raw := c.arenas.members.Deref(id)

	switch parent := parent.(type) {
	case oneof:
		raw.oneof = int32(parent.Index())
		parent.raw.members = append(parent.raw.members, id)
	case extend:
		raw.extendee = parent.extendee
	}

	if !parentTy.IsZero() {
		if _, ok := parent.(extend); ok {
			parentTy.raw.members = append(parentTy.raw.members, id)
			c.extns = append(c.extns, id)
		} else {
			parentTy.raw.members = slices.Insert(parentTy.raw.members, int(parentTy.raw.extnsStart), id)
			parentTy.raw.extnsStart++
		}
	} else if _, ok := parent.(extend); ok {
		c.extns = slices.Insert(c.extns, c.topLevelExtnsEnd, id)
		c.topLevelExtnsEnd++
	}

	return Member{internal.NewWith(w.Context()), raw}
}

func (w *walker) newOneof(def ast.DefOneof, parent any) Oneof {
	parentTy := extractParentType(parent)
	name := def.Name.Name()
	fqn := w.fullname(parentTy, name)

	if parentTy.IsZero() {
		return Oneof{}
	}

	raw := w.Context().arenas.oneofs.NewCompressed(rawOneof{
		def:       def.Decl,
		name:      w.Context().session.intern.Intern(name),
		fqn:       w.Context().session.intern.Intern(fqn),
		index:     uint32(len(parentTy.raw.oneofs)),
		container: w.Context().arenas.types.Compress(parentTy.raw),
	})

	parentTy.raw.oneofs = append(parentTy.raw.oneofs, raw)
	return wrapOneof(w.Context(), raw)
}

func (w *walker) newExtendee(def ast.DefExtend, parent any) arena.Pointer[rawExtendee] {
	parentTy := extractParentType(parent)

	return w.Context().arenas.extendees.NewCompressed(rawExtendee{
		def:    def.Decl,
		parent: w.Context().arenas.types.Compress(parentTy.raw),
	})
}

func (w *walker) newService(def ast.DeclDef, parent any) Service {
	if parent != nil {
		return Service{}
	}

	name := def.Name().AsIdent().Name()
	fqn := w.pkg.Append(name)

	raw := w.Context().arenas.services.NewCompressed(rawService{
		def:  def,
		name: w.Context().session.intern.Intern(name),
		fqn:  w.Context().session.intern.Intern(string(fqn)),
	})
	w.Context().services = append(w.Context().services, raw)
	return Service{
		internal.NewWith(w.Context()),
		w.Context().arenas.services.Deref(raw),
	}
}

func (w *walker) newMethod(def ast.DeclDef, parent any) Method {
	service, ok := parent.(Service)
	if !ok {
		return Method{}
	}

	name := def.Name().AsIdent().Name()
	fqn := service.FullName().Append(name)

	raw := w.Context().arenas.methods.NewCompressed(rawMethod{
		def:     def,
		name:    w.Context().session.intern.Intern(name),
		fqn:     w.Context().session.intern.Intern(string(fqn)),
		service: w.Context().arenas.services.Compress(service.raw),
	})
	service.raw.methods = append(service.raw.methods, raw)
	return Method{
		internal.NewWith(w.Context()),
		w.Context().arenas.methods.Deref(raw),
	}
}

func (w *walker) fullname(parentTy Type, name string) string {
	parentName := w.pkg
	if !parentTy.IsZero() {
		parentName = parentTy.FullName()
		if parentTy.IsEnum() {
			// Protobuf dumps the names of enum values in the enum's parent, so
			// we need to drop a path component.
			parentName = parentName.Parent()
		}
	}
	return string(parentName.Append(name))
}
