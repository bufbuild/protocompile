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
	"iter"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
	"github.com/bufbuild/protocompile/internal/toposort"
)

// Context is where all of the book-keeping for an IR session is kept.
//
// Unlike [ast.Context], this Context is shared by many files.
//
//nolint:govet // For some reason, this lint mangles the field order on this struct. >:(
type Context struct {
	session *Session
	ast     ast.File

	// The path for this file. This need not be what ast.Span() reports, because
	// it has been passed through filepath.Clean() and filepath.ToSlash() first,
	// to normalize it.
	path intern.ID

	syntax syntax.Syntax
	pkg    intern.ID

	imports imports

	types            []arena.Pointer[rawType]
	topLevelTypesEnd int // Index of the last top-level type in types.

	extns            []arena.Pointer[rawMember]
	topLevelExtnsEnd int // Index of the last top-level extension in extns.

	options  arena.Pointer[rawValue]
	services []arena.Pointer[rawService]

	// Table of all symbols transitively imported by this file. This is all
	// local symbols plus the imported tables of all direct imports. Importing
	// everything and checking visibility later allows us to diagnose
	// missing import errors.

	// This file's symbol tables. Each file has two symbol tables: its imported
	// symbols and its exported symbols.
	//
	// The exported symbols are formed from the file's local symbols, and the
	// exported symbols of each transitive public import.
	//
	// The imported symbols are the exported symbols plus the exported symbols
	// of each direct import.
	exported, imported symtab

	dpBuiltins *builtins // Only non-nil for descriptor.proto.

	arenas struct {
		types     arena.Arena[rawType]
		members   arena.Arena[rawMember]
		ranges    arena.Arena[rawReservedRange]
		extendees arena.Arena[rawExtendee]
		oneofs    arena.Arena[rawOneof]
		symbols   arena.Arena[rawSymbol]

		values   arena.Arena[rawValue]
		messages arena.Arena[rawMessageValue]
		arrays   arena.Arena[[]rawValueBits]

		services arena.Arena[rawService]
		methods  arena.Arena[rawMethod]
	}
}

type withContext = internal.With[*Context]

// ref is an arena.Pointer[T] along with information for retrieving which file
// it's in, relative to a specific file's imports.
type ref[T any] struct {
	// The file this ref is defined in. If zero, it refers to the current file.
	// If -1, it refers to a predeclared type. Otherwise, it refers to an
	// import (with its index offset by 1).
	file int32
	ptr  arena.Pointer[T]
}

// context returns the context for this reference relative to a base context.
func (r ref[T]) context(base *Context) *Context {
	switch r.file {
	case 0:
		return base
	case -1:
		return primitiveCtx
	default:
		return base.imports.files[r.file-1].file.Context()
	}
}

// File returns the file associated with this context.
//
// The file can be used to access top-level elements of the IR, for walking it
// recursively (as is needed for e.g. assembling a FileDescriptorProto).
func (c *Context) File() File {
	return File{withContext2{internal.NewWith(c)}}
}

// builtins returns the builtin descriptor.proto names.
func (c *Context) builtins() *builtins {
	if c.dpBuiltins != nil {
		return c.dpBuiltins
	}
	return c.imports.DescriptorProto().Context().dpBuiltins
}

// File is an IR file, which provides access to the top-level declarations of
// a Protobuf file.
type File struct{ withContext2 }

// withContext2 is a workaround for go.dev/issue/50729, a bug in how Go
// handles embedded type aliases. In this case, Go incorrectly believes that
// File is a recursive type of infinite size. This issue is fixed in recent Go
// versions but not in some of the versions we support.
//
// We can't just embed With into File, because then it would be an exported
// field.
type withContext2 struct{ internal.With[*Context] }

// AST returns the AST this file was parsed from.
func (f File) AST() ast.File {
	if f.IsZero() {
		return ast.File{}
	}
	return f.Context().ast
}

// Syntax returns the syntax pragma that applies to this file.
func (f File) Syntax() syntax.Syntax {
	if f.IsZero() {
		return syntax.Unknown
	}
	return f.Context().syntax
}

// Path returns the canoniocal path for this file.
//
// This need not be the same as [File.AST]().Span().Path().
func (f File) Path() string {
	if f.IsZero() {
		return ""
	}
	c := f.Context()
	return c.session.intern.Value(c.path)
}

// InternedPackage returns the intern ID for the value of [File.Path].
func (f File) InternedPath() intern.ID {
	if f.IsZero() {
		return 0
	}
	return f.Context().path
}

// IsDescriptorProto returns whether this is the special file
// google/protobuf/descriptor.proto, which is given special treatment in
// the language.
func (f File) IsDescriptorProto() bool {
	return f.InternedPath() == f.Context().session.builtins.DescriptorFile
}

// Package returns the package name for this file.
//
// The name will not include a leading dot. It will be empty for the empty
// package.
func (f File) Package() FullName {
	if f.IsZero() {
		return ""
	}
	c := f.Context()
	return FullName(c.session.intern.Value(c.pkg))
}

// InternedPackage returns the intern ID for the value of [File.Package].
func (f File) InternedPackage() intern.ID {
	if f.IsZero() {
		return 0
	}
	return f.Context().pkg
}

// Imports returns an indexer over the imports declared in this file.
func (f File) Imports() seq.Indexer[Import] {
	return f.Context().imports.Directs()
}

// TransitiveImports returns an indexer over the transitive imports for this
// file.
//
// This function does not report whether those imports are weak or not.
func (f File) TransitiveImports() seq.Indexer[Import] {
	return f.Context().imports.Transitive()
}

// ImportFor returns import metadata for a given file, if this file imports it.
func (f File) ImportFor(that File) Import {
	idx, ok := f.Context().imports.byPath[that.InternedPath()]
	if !ok {
		return Import{}
	}

	return f.TransitiveImports().At(int(idx))
}

// Types returns the top level types of this file.
func (f File) Types() seq.Indexer[Type] {
	return seq.NewFixedSlice(
		f.Context().types[:f.Context().topLevelTypesEnd],
		func(_ int, p arena.Pointer[rawType]) Type {
			// Implicitly in current file.
			return wrapType(f.Context(), ref[rawType]{ptr: p})
		},
	)
}

// AllTypes returns all types defined in this file.
func (f File) AllTypes() seq.Indexer[Type] {
	return seq.NewFixedSlice(
		f.Context().types,
		func(_ int, p arena.Pointer[rawType]) Type {
			// Implicitly in current file.
			return wrapType(f.Context(), ref[rawType]{ptr: p})
		},
	)
}

// Extensions returns the top level extensions defined in this file (i.e.,
// the contents of any top-level `extends` blocks).
func (f File) Extensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.Context().extns[:f.Context().topLevelExtnsEnd],
		func(_ int, p arena.Pointer[rawMember]) Member {
			// Implicitly in current file.
			return wrapMember(f.Context(), ref[rawMember]{ptr: p})
		},
	)
}

// AllExtensions returns all extensions defined in this file.
func (f File) AllExtensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.Context().extns,
		func(_ int, p arena.Pointer[rawMember]) Member {
			// Implicitly in current file.
			return wrapMember(f.Context(), ref[rawMember]{ptr: p})
		},
	)
}

// Services returns all services defined in this file.
func (f File) Services() seq.Indexer[Service] {
	return seq.NewFixedSlice(
		f.Context().services,
		func(_ int, p arena.Pointer[rawService]) Service {
			return Service{
				internal.NewWith(f.Context()),
				f.Context().arenas.services.Deref(p),
			}
		},
	)
}

// Options returns the top level options applied to this file.
func (f File) Options() MessageValue {
	return wrapValue(f.Context(), f.Context().options).AsMessage()
}

// Symbols returns this file's symbol table.
//
// The symbol table includes both symbols defined in this file, and symbols
// imported by the file. The symbols are returned in an arbitrary but fixed
// order.
func (f File) Symbols() seq.Indexer[Symbol] {
	return seq.NewFixedSlice(
		f.Context().imported,
		func(_ int, r ref[rawSymbol]) Symbol {
			return wrapSymbol(f.Context(), r)
		},
	)
}

// topoSort sorts a graph of [File]s according to their dependency graph,
// in topological order. Files with no dependencies are yielded first.
func topoSort(files []File) iter.Seq[File] {
	// NOTE: This cannot panic because Files, by construction, do not contain
	// graph cycles.
	return toposort.Sort(
		files,
		File.Context,
		func(f File) iter.Seq[File] {
			return seq.Map(
				f.Context().imports.Directs(),
				func(i Import) File { return i.File },
			)
		},
	)
}
