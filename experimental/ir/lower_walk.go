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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// walker is the state struct for the AST-walking logic.
type walker struct {
	*File
	*report.Report

	pkg FullName
}

// walk is the first step in constructing an IR module: converting an AST file
// into a type-and-field graph, and recording the AST source for each structure.
//
// This operation performs no name resolution.
func (w *walker) walk() {
	if pkg := w.AST().Package(); !pkg.IsZero() {
		w.File.pkg = w.session.intern.Intern(pkg.Path().Canonicalized())
	}
	w.pkg = w.Package()

	w.syntax = syntax.Proto2
	if syn := w.AST().Syntax(); !syn.IsZero() {
		text := syn.Value().Span().Text()
		if unquoted := syn.Value().AsLiteral().AsString(); !unquoted.IsZero() {
			text = unquoted.Text()
		}

		// NOTE: This matches fallback behavior in parser/legalize_file.go.
		w.syntax = syntax.Lookup(text)
		if w.syntax == syntax.Unknown {
			if syn.IsEdition() {
				// If they wrote edition = "garbage" they probably want *an*
				// edition, so we pick the oldest one.
				w.syntax = syntax.Edition2023
			} else {
				w.syntax = syntax.Proto2
			}
		}
	}

	for decl := range seq.Values(w.AST().Decls()) {
		w.recurse(decl, nil)
	}
}

type extend struct {
	parent   Type
	extendee id.ID[Extend]
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
				extendee: w.newExtendee(def.AsExtend(), parent).ID(),
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
	parentTy := extractParentType(parent)

	name := def.Name().AsIdent().Name()
	fqn := w.fullname(parentTy, name)

	ty := id.Wrap(w.File, id.ID[Type](w.arenas.types.NewCompressed(rawType{
		def:    def.ID(),
		name:   w.session.intern.Intern(name),
		fqn:    w.session.intern.Intern(fqn),
		parent: parentTy.ID(),

		isEnum: def.Keyword() == keyword.Enum,
	})))
	ty.Raw().memberByName = sync.OnceValue(ty.makeMembersByName)

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

				ty.Raw().reservedNames = append(ty.Raw().reservedNames, rawReservedName{
					ast:  v,
					name: ty.Context().session.intern.Intern(name),
				})
				continue
			}

			raw := id.ID[ReservedRange](w.arenas.ranges.NewCompressed(rawReservedRange{
				decl:          rangeDecl.ID(),
				value:         v.ID(),
				forExtensions: rangeDecl.IsExtensions(),
			}))

			if rangeDecl.IsReserved() {
				ty.Raw().ranges = slices.Insert(ty.Raw().ranges, int(ty.Raw().rangesExtnStart), raw)
				ty.Raw().rangesExtnStart++
			} else {
				ty.Raw().ranges = append(ty.Raw().ranges, raw)
			}
		}
	}

	if !parentTy.IsZero() {
		parentTy.Raw().nested = append(parentTy.Raw().nested, ty.ID())
		w.File.types = append(w.File.types, ty.ID())
	} else {
		w.File.types = slices.Insert(w.File.types, w.File.topLevelTypesEnd, ty.ID())
		w.File.topLevelTypesEnd++
	}

	return ty
}

//nolint:unparam // Complains about the return value for some reason.
func (w *walker) newField(def ast.DeclDef, parent any, group bool) Member {
	parentTy := extractParentType(parent)
	name := def.Name().AsIdent().Name()
	if group {
		name = strings.ToLower(name)
	}
	fqn := w.fullname(parentTy, name)

	member := id.Wrap(w.File, id.ID[Member](w.arenas.members.NewCompressed(rawMember{
		def:     def.ID(),
		name:    w.session.intern.Intern(name),
		fqn:     w.session.intern.Intern(fqn),
		parent:  parentTy.ID(),
		oneof:   math.MinInt32,
		isGroup: group,
	})))

	switch parent := parent.(type) {
	case oneof:
		member.Raw().oneof = int32(parent.Index())
		parent.Raw().members = append(parent.Raw().members, member.ID())
	case extend:
		member.Raw().extendee = parent.extendee

		block := id.Wrap(w.File, parent.extendee)
		block.Raw().members = append(block.Raw().members, member.ID())
	}

	if !parentTy.IsZero() {
		if _, ok := parent.(extend); ok {
			parentTy.Raw().members = append(parentTy.Raw().members, member.ID())
			w.File.extns = append(w.File.extns, member.ID())
		} else {
			parentTy.Raw().members = slices.Insert(parentTy.Raw().members, int(parentTy.Raw().extnsStart), member.ID())
			parentTy.Raw().extnsStart++
		}
	} else if _, ok := parent.(extend); ok {
		w.File.extns = slices.Insert(w.File.extns, w.File.topLevelExtnsEnd, member.ID())
		w.File.topLevelExtnsEnd++
	}

	return member
}

func (w *walker) newOneof(def ast.DefOneof, parent any) Oneof {
	parentTy := extractParentType(parent)
	name := def.Name.Name()
	fqn := w.fullname(parentTy, name)

	if parentTy.IsZero() {
		return Oneof{}
	}

	oneof := id.Wrap(w.File, id.ID[Oneof](w.arenas.oneofs.NewCompressed(rawOneof{
		def:       def.Decl.ID(),
		name:      w.session.intern.Intern(name),
		fqn:       w.session.intern.Intern(fqn),
		index:     uint32(len(parentTy.Raw().oneofs)),
		container: parentTy.ID(),
	})))

	parentTy.Raw().oneofs = append(parentTy.Raw().oneofs, oneof.ID())
	return oneof
}

func (w *walker) newExtendee(def ast.DefExtend, parent any) Extend {
	c := w.File
	parentTy := extractParentType(parent)

	extend := id.Wrap(w.File, id.ID[Extend](w.arenas.extendees.NewCompressed(rawExtend{
		def:    def.Decl.ID(),
		parent: parentTy.ID(),
	})))

	if !parentTy.IsZero() {
		parentTy.Raw().extends = append(parentTy.Raw().extends, extend.ID())
		c.extends = append(c.extends, extend.ID())
	} else {
		c.extends = slices.Insert(c.extends, c.topLevelExtendsEnd, extend.ID())
		c.topLevelExtendsEnd++
	}

	return extend
}

func (w *walker) newService(def ast.DeclDef, parent any) Service {
	if parent != nil {
		return Service{}
	}

	name := def.Name().AsIdent().Name()
	fqn := w.pkg.Append(name)

	service := id.Wrap(w.File, id.ID[Service](w.arenas.services.NewCompressed(rawService{
		def:  def.ID(),
		name: w.session.intern.Intern(name),
		fqn:  w.session.intern.Intern(string(fqn)),
	})))

	w.File.services = append(w.File.services, service.ID())
	return service
}

func (w *walker) newMethod(def ast.DeclDef, parent any) Method {
	service, ok := parent.(Service)
	if !ok {
		return Method{}
	}

	name := def.Name().AsIdent().Name()
	fqn := service.FullName().Append(name)

	method := id.Wrap(w.File, id.ID[Method](w.arenas.methods.NewCompressed(rawMethod{
		def:     def.ID(),
		name:    w.session.intern.Intern(name),
		fqn:     w.session.intern.Intern(string(fqn)),
		service: service.ID(),
	})))

	service.Raw().methods = append(service.Raw().methods, method.ID())
	return method
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
