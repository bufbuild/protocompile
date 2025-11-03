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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
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

	types            []id.ID[Type]
	topLevelTypesEnd int // Index of the last top-level type in types.

	extns            []id.ID[Member]
	topLevelExtnsEnd int // Index of the last top-level extension in extns.

	extends            []id.ID[Extend]
	topLevelExtendsEnd int // Index of last top-level extension in extends.

	options  id.ID[Value]
	services []id.ID[Service]
	features id.ID[FeatureSet]

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
		extendees arena.Arena[rawExtend]
		oneofs    arena.Arena[rawOneof]

		services arena.Arena[rawService]
		methods  arena.Arena[rawMethod]

		values   arena.Arena[rawValue]
		messages arena.Arena[rawMessageValue]
		arrays   arena.Arena[[]rawValueBits]
		features arena.Arena[rawFeatureSet]

		symbols arena.Arena[rawSymbol]
	}
}

type withContext = id.HasContext[*Context]

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
	if f.Context() == primitiveCtx {
		return "<predeclared>"
	}
	c := f.Context()
	return c.session.intern.Value(c.path)
}

// InternedPath returns the intern ID for the value of [File.Path].
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
	if f.Context() == primitiveCtx {
		return ""
	}
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
		func(_ int, p id.ID[Type]) Type {
			return id.Wrap(f.Context(), p)
		},
	)
}

// AllTypes returns all types defined in this file.
func (f File) AllTypes() seq.Indexer[Type] {
	return seq.NewFixedSlice(
		f.Context().types,
		func(_ int, p id.ID[Type]) Type {
			return id.Wrap(f.Context(), p)
		},
	)
}

// Extensions returns the top level extensions defined in this file (i.e.,
// the contents of any top-level `extends` blocks).
func (f File) Extensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.Context().extns[:f.Context().topLevelExtnsEnd],
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(f.Context(), p)
		},
	)
}

// AllExtensions returns all extensions defined in this file.
func (f File) AllExtensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.Context().extns,
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(f.Context(), p)
		},
	)
}

// Extends returns the top level extend blocks in this file.
func (f File) Extends() seq.Indexer[Extend] {
	return seq.NewFixedSlice(
		f.Context().extends[:f.Context().topLevelExtendsEnd],
		func(_ int, p id.ID[Extend]) Extend {
			return id.Wrap(f.Context(), p)
		},
	)
}

// AllExtends returns all extend blocks in this file.
func (f File) AllExtends() seq.Indexer[Extend] {
	return seq.NewFixedSlice(
		f.Context().extends,
		func(_ int, p id.ID[Extend]) Extend {
			return id.Wrap(f.Context(), p)
		},
	)
}

// AllMembers returns all fields defined in this file, including extensions
// and enum values.
func (f File) AllMembers() iter.Seq[Member] {
	i := 0
	return iterx.Map(f.Context().arenas.members.Values(), func(raw *rawMember) Member {
		i++
		return id.WrapRaw(f.Context(), id.ID[Member](i), raw)
	})
}

// Services returns all services defined in this file.
func (f File) Services() seq.Indexer[Service] {
	return seq.NewFixedSlice(
		f.Context().services,
		func(_ int, p id.ID[Service]) Service {
			return id.Wrap(f.Context(), p)
		},
	)
}

// Options returns the top level options applied to this file.
func (f File) Options() MessageValue {
	return id.Wrap(f.Context(), f.Context().options).AsMessage()
}

// FeatureSet returns the Editions features associated with this file.
func (f File) FeatureSet() FeatureSet {
	return id.Wrap(f.Context(), f.Context().features)
}

// Deprecated returns whether this file is deprecated, by returning the
// relevant option value for setting deprecation.
func (f File) Deprecated() Value {
	if f.IsZero() {
		return Value{}
	}
	builtins := f.Context().builtins()
	d := f.Options().Field(builtins.FileDeprecated)
	if b, _ := d.AsBool(); b {
		return d
	}
	return Value{}
}

// Symbols returns this file's symbol table.
//
// The symbol table includes both symbols defined in this file, and symbols
// imported by the file. The symbols are returned in an arbitrary but fixed
// order.
func (f File) Symbols() seq.Indexer[Symbol] {
	return seq.NewFixedSlice(
		f.Context().imported,
		func(_ int, r Ref[Symbol]) Symbol {
			return GetRef(f.Context(), r)
		},
	)
}

// FindSymbol finds a symbol among [File.Symbols] with the given fully-qualified
// name.
func (f File) FindSymbol(fqn FullName) Symbol {
	return GetRef(f.Context(),
		f.Context().imported.lookupBytes(f.Context(),
			unsafex.BytesAlias[[]byte](string(fqn))))
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

func (c *Context) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawType:
		return c.arenas.types.Deref(arena.Pointer[rawType](id))
	case **rawMember:
		return c.arenas.members.Deref(arena.Pointer[rawMember](id))
	case **rawReservedRange:
		return c.arenas.ranges.Deref(arena.Pointer[rawReservedRange](id))
	case **rawExtend:
		return c.arenas.extendees.Deref(arena.Pointer[rawExtend](id))
	case **rawOneof:
		return c.arenas.oneofs.Deref(arena.Pointer[rawOneof](id))

	case **rawService:
		return c.arenas.services.Deref(arena.Pointer[rawService](id))
	case **rawMethod:
		return c.arenas.methods.Deref(arena.Pointer[rawMethod](id))

	case **rawValue:
		return c.arenas.values.Deref(arena.Pointer[rawValue](id))
	case **rawMessageValue:
		return c.arenas.messages.Deref(arena.Pointer[rawMessageValue](id))
	case **rawFeatureSet:
		return c.arenas.features.Deref(arena.Pointer[rawFeatureSet](id))

	case **rawSymbol:
		return c.arenas.symbols.Deref(arena.Pointer[rawSymbol](id))

	default:
		return c.File().AST().Context().FromID(id, want)
	}
}
