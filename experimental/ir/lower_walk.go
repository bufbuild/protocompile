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
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
)

// walker is the state struct for the AST-walking logic.
type walker struct {
	File
	*report.Report
}

// walk is the first step in constructing an IR module: converting an AST file
// into a type-and-field graph, and recording the AST source for each structure.
//
// This operation performs no name resolution.
func (w *walker) walk() {
	c := w.Context()

	path := filepath.Clean(w.AST().Span().File.Path())
	path = filepath.ToSlash(path)
	c.path = c.session.intern.Intern(path)

	if pkg := w.AST().Package(); !pkg.IsZero() {
		c.pkg = c.session.intern.Intern(pkg.Path().Canonicalized())
	}

	w.recurse(w.AST().AsAny(), nil)
}

type extend struct {
	parent Type
	extend ast.DefExtend
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
		sorry("ranges")

	case ast.DeclKindDef:
		def := decl.AsDef()
		if def.IsCorrupt() {
			return
		}

		switch kind := def.Classify(); kind {
		case ast.DefKindMessage, ast.DefKindEnum:
			ty := w.newType(def, parent)
			ty.raw.isEnum = kind == ast.DefKindEnum

			w.recurse(def.Body().AsAny(), ty)

		case ast.DefKindField, ast.DefKindEnumValue:
			w.newField(def, parent)

		case ast.DefKindGroup:
			sorry("groups")

		case ast.DefKindOneof:
			oneofDef := def.AsOneof()
			w.recurse(def.Body().AsAny(), oneof{
				parent: extractParentType(parent),
				Oneof:  w.newOneof(oneofDef, parent),
			})

		case ast.DefKindExtend:
			w.recurse(def.Body().AsAny(), extend{
				parent: extractParentType(parent),
				extend: def.AsExtend(),
			})

		case ast.DefKindService:
			sorry("services")

		case ast.DefKindMethod:
			sorry("methods")

		case ast.DefKindOption:
			return // Handled later, after symbol table building.
		}
	}
}

func (w *walker) newType(def ast.DeclDef, parent any) Type {
	parentTy := extractParentType(parent)

	name := def.Name().AsIdent().Name()
	var fqn string

	raw := w.Context().arenas.types.New(rawType{
		def:  def,
		name: w.Context().session.intern.Intern(name),
		fqn:  w.Context().session.intern.Intern(fqn),
	})

	if !parentTy.IsZero() {
		parentTy.raw.nested = append(parentTy.raw.nested,
			w.Context().arenas.types.Compress(raw),
		)
	}

	return Type{internal.NewWith(w.Context()), raw}
}

func (w *walker) newField(def ast.DeclDef, parent any) Field {
	parentTy := extractParentType(parent)

	name := def.Name().AsIdent().Name()
	var fqn string
	if !parentTy.IsZero() {
		parentName := parentTy.Name()
		if parentTy.IsEnum() {
			// Protobuf dumps the names of enum values in the enum's parent, so
			// we need to drop a path component.
			parentName = parentName[:strings.LastIndexByte(parentName, '.')]
		}
		fqn = parentName + "." + name
	} else {
		fqn = "." + w.Package() + "." + name
	}

	id := w.Context().arenas.fields.NewCompressed(rawField{
		def:    def,
		name:   w.Context().session.intern.Intern(name),
		fqn:    w.Context().session.intern.Intern(fqn),
		parent: w.Context().arenas.types.Compress(parentTy.raw),
	})
	raw := w.Context().arenas.fields.Deref(id)

	switch parent := parent.(type) {
	case oneof:
		raw.oneof = w.Context().arenas.oneofs.Compress(parent.Oneof.raw)
		parent.Oneof.raw.members = append(parent.Oneof.raw.members, id)
	case extend:
		// TODO: Cram the extension type somewhere so we can resolve it later.
	}

	if !parentTy.IsZero() {
		parentTy.raw.fields = append(parentTy.raw.fields, id)

		if _, ok := parent.(extend); !ok {
			parentTy.raw.fieldsExtnStart++
		}
	} else if _, ok := parent.(extend); !ok {
		file := &w.Context().file
		file.extns = append(file.extns, id)
	}

	return Field{internal.NewWith(w.Context()), raw}
}

func (w *walker) newOneof(def ast.DefOneof, parent any) Oneof {
	parentTy := extractParentType(parent)

	name := def.Name.Name()
	var fqn string
	if !parentTy.IsZero() {
		fqn = parentTy.Name() + "." + name
	} else {
		fqn = "." + w.Package() + "." + name
	}

	raw := w.Context().arenas.oneofs.New(rawOneof{
		def:  def.Decl,
		name: w.Context().session.intern.Intern(name),
		fqn:  w.Context().session.intern.Intern(fqn),
	})

	if !parentTy.IsZero() {
		parentTy.raw.oneofs = append(parentTy.raw.oneofs,
			w.Context().arenas.oneofs.Compress(raw),
		)

		if _, ok := parent.(extend); !ok {
			parentTy.raw.fieldsExtnStart++
		}
	}

	return Oneof{internal.NewWith(w.Context()), raw}
}
