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

package ast

import (
	"iter"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [DeclBody]). The
// File type provides convenience functions for extracting salient elements,
// such as the [DeclSyntax] and the [DeclPackage].
//
// # Grammar
//
//	File := DeclAny*
type File struct {
	_      unsafex.NoCopy
	stream *token.Stream
	path   string

	decls   decls
	types   types
	exprs   exprs
	options arena.Arena[rawCompactOptions]

	// A cache of raw paths that have been converted into parenthesized
	// components in NewExtensionComponent.
	extnPathCache map[PathID]token.ID
}

type withContext = id.HasContext[*File]

// New creates a fresh context for a file.
//
// path is the semantic import path of this file, which may not be the same as
// file.Path, which is used for diagnostics.
func New(path string, stream *token.Stream) *File {
	f := &File{
		stream: stream,
		path:   path,
	}
	_ = f.Nodes().NewDeclBody(token.Zero) // This is the rawBody for the whole file.

	return f
}

// Syntax returns this file's declaration, if it has one.
func (f *File) Syntax() DeclSyntax {
	for d := range seq.Values(f.Decls()) {
		if s := d.AsSyntax(); !s.IsZero() {
			return s
		}
	}
	return DeclSyntax{}
}

// Package returns this file's package declaration, if it has one.
func (f *File) Package() DeclPackage {
	for d := range seq.Values(f.Decls()) {
		if p := d.AsPackage(); !p.IsZero() {
			return p
		}
	}
	return DeclPackage{}
}

// Imports returns an iterator over this file's import declarations.
func (f *File) Imports() iter.Seq[DeclImport] {
	return iterx.FilterMap(seq.Values(f.Decls()), func(d DeclAny) (DeclImport, bool) {
		if imp := d.AsImport(); !imp.IsZero() {
			return imp, true
		}
		return DeclImport{}, false
	})
}

// Path returns the semantic import path of this file.
func (f *File) Path() string {
	if f == nil {
		return ""
	}
	return f.path
}

// Decls returns all of the top-level declarations in this file.
func (f *File) Decls() seq.Inserter[DeclAny] {
	return id.Wrap(f, id.ID[DeclBody](1)).Decls()
}

// Stream returns the underlying token stream.
func (f *File) Stream() *token.Stream {
	if f == nil {
		return nil
	}
	return f.stream
}

// Nodes returns the node arena for this file, which can be used to allocate
// new AST nodes.
func (f *File) Nodes() *Nodes {
	return (*Nodes)(f)
}

// Stream returns the underlying token stream.
func (f *File) Span() source.Span {
	return id.Wrap(f, id.ID[DeclBody](1)).Span()
}

// FromID implements [id.Context].
func (f *File) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawDeclBody:
		return f.decls.bodies.Deref(arena.Pointer[rawDeclBody](id))
	case **rawDeclDef:
		return f.decls.defs.Deref(arena.Pointer[rawDeclDef](id))
	case **rawDeclEmpty:
		return f.decls.empties.Deref(arena.Pointer[rawDeclEmpty](id))
	case **rawDeclImport:
		return f.decls.imports.Deref(arena.Pointer[rawDeclImport](id))
	case **rawDeclPackage:
		return f.decls.packages.Deref(arena.Pointer[rawDeclPackage](id))
	case **rawDeclRange:
		return f.decls.ranges.Deref(arena.Pointer[rawDeclRange](id))
	case **rawDeclSyntax:
		return f.decls.syntaxes.Deref(arena.Pointer[rawDeclSyntax](id))

	case **rawExprError:
		return f.exprs.errors.Deref(arena.Pointer[rawExprError](id))
	case **rawExprArray:
		return f.exprs.arrays.Deref(arena.Pointer[rawExprArray](id))
	case **rawExprDict:
		return f.exprs.dicts.Deref(arena.Pointer[rawExprDict](id))
	case **rawExprField:
		return f.exprs.fields.Deref(arena.Pointer[rawExprField](id))
	case **rawExprPrefixed:
		return f.exprs.prefixes.Deref(arena.Pointer[rawExprPrefixed](id))
	case **rawExprRange:
		return f.exprs.ranges.Deref(arena.Pointer[rawExprRange](id))

	case **rawTypeError:
		return f.types.errors.Deref(arena.Pointer[rawTypeError](id))
	case **rawTypeGeneric:
		return f.types.generics.Deref(arena.Pointer[rawTypeGeneric](id))
	case **rawTypePrefixed:
		return f.types.prefixes.Deref(arena.Pointer[rawTypePrefixed](id))

	case **rawCompactOptions:
		return f.options.Deref(arena.Pointer[rawCompactOptions](id))

	default:
		return f.stream.FromID(id, want)
	}
}
