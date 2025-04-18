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

	extns            []arena.Pointer[rawField]
	topLevelExtnsEnd int // Index of the last top-level extension in extns.

	options arena.Pointer[rawValue]

	arenas struct {
		types  arena.Arena[rawType]
		fields arena.Arena[rawField]
		oneofs arena.Arena[rawOneof]

		values   arena.Arena[rawValue]
		messages arena.Arena[rawMessageValue]
		arrays   arena.Arena[[]rawValueBits]
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

// File returns the file associated with this context.
//
// The file can be used to access top-level elements of the IR, for walking it
// recursively (as is needed for e.g. assembling a FileDescriptorProto).
func (c *Context) File() File {
	return File{withContext2{internal.NewWith(c)}}
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
	return f.Context().ast
}

// Syntax returns the syntax pragma that applies to this file.
func (f File) Syntax() syntax.Syntax {
	return f.Context().syntax
}

// Path returns the canoniocal path for this file.
//
// This need not be the same as [File.AST]().Span().Path().
func (f File) Path() string {
	c := f.Context()
	return c.session.intern.Value(c.path)
}

// InternedPackage returns the intern ID for the value of [File.Path].
func (f File) InternedPath() intern.ID {
	return f.Context().path
}

// Package returns the package name for this file.
//
// The name will not include a leading dot. It will be empty for the empty
// package.
func (f File) Package() FullName {
	c := f.Context()
	return FullName(c.session.intern.Value(c.pkg))
}

// InternedPackage returns the intern ID for the value of [File.Package].
func (f File) InternedPackage() intern.ID {
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
func (f File) Extensions() seq.Indexer[Field] {
	return seq.NewFixedSlice(
		f.Context().extns[:f.Context().topLevelExtnsEnd],
		func(_ int, p arena.Pointer[rawField]) Field {
			// Implicitly in current file.
			return wrapField(f.Context(), ref[rawField]{ptr: p})
		},
	)
}

// AllExtensions returns all extensions defined in this file.
func (f File) AllExtensions() seq.Indexer[Field] {
	return seq.NewFixedSlice(
		f.Context().extns,
		func(_ int, p arena.Pointer[rawField]) Field {
			// Implicitly in current file.
			return wrapField(f.Context(), ref[rawField]{ptr: p})
		},
	)
}

// Options returns the top level options applied to this file.
func (f File) Options() MessageValue {
	return wrapValue(f.Context(), f.Context().options).AsMessage()
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
