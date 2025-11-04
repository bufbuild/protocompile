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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
	"github.com/bufbuild/protocompile/internal/intern"
	"github.com/bufbuild/protocompile/internal/toposort"
)

// File is an IR file, which provides access to the top-level declarations of
// a Protobuf *File.
//
//nolint:govet // For some reason, this lint mangles the field order on this struct. >:(
type File struct {
	_       unsafex.NoCopy
	session *Session
	ast     *ast.File

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

type withContext = id.HasContext[*File]

// builtins returns the builtin descriptor.proto names.
func (f *File) builtins() *builtins {
	if f.dpBuiltins != nil {
		return f.dpBuiltins
	}
	return f.imports.DescriptorProto().dpBuiltins
}

// AST returns the AST this file was parsed from.
func (f *File) AST() *ast.File {
	if f == nil {
		return nil
	}
	return f.ast
}

// Syntax returns the syntax pragma that applies to this file.
func (f *File) Syntax() syntax.Syntax {
	if f == nil {
		return syntax.Unknown
	}
	return f.syntax
}

// Path returns the canonical path for this file.
//
// This need not be the same as [File.AST]().Span().Path().
func (f *File) Path() string {
	if f == nil {
		return ""
	}
	if f == primitiveCtx {
		return "<predeclared>"
	}
	c := f
	return c.session.intern.Value(c.path)
}

// InternedPath returns the intern ID for the value of [File.Path].
func (f *File) InternedPath() intern.ID {
	if f == nil {
		return 0
	}
	return f.path
}

// IsDescriptorProto returns whether this is the special file
// google/protobuf/descriptor.proto, which is given special treatment in
// the language.
func (f *File) IsDescriptorProto() bool {
	return f.InternedPath() == f.session.builtins.DescriptorFile
}

// Package returns the package name for this file.
//
// The name will not include a leading dot. It will be empty for the empty
// package.
func (f *File) Package() FullName {
	if f == nil {
		return ""
	}
	c := f
	if f == primitiveCtx {
		return ""
	}
	return FullName(c.session.intern.Value(c.pkg))
}

// InternedPackage returns the intern ID for the value of [File.Package].
func (f *File) InternedPackage() intern.ID {
	if f == nil {
		return 0
	}
	return f.pkg
}

// Imports returns an indexer over the imports declared in this file.
func (f *File) Imports() seq.Indexer[Import] {
	return f.imports.Directs()
}

// TransitiveImports returns an indexer over the transitive imports for this
// file.
//
// This function does not report whether those imports are weak or not.
func (f *File) TransitiveImports() seq.Indexer[Import] {
	return f.imports.Transitive()
}

// ImportFor returns import metadata for a given file, if this file imports it.
func (f *File) ImportFor(that *File) Import {
	idx, ok := f.imports.byPath[that.InternedPath()]
	if !ok {
		return Import{}
	}

	return f.TransitiveImports().At(int(idx))
}

// Types returns the top level types of this file.
func (f *File) Types() seq.Indexer[Type] {
	return seq.NewFixedSlice(
		f.types[:f.topLevelTypesEnd],
		func(_ int, p id.ID[Type]) Type {
			return id.Wrap(f, p)
		},
	)
}

// AllTypes returns all types defined in this file.
func (f *File) AllTypes() seq.Indexer[Type] {
	return seq.NewFixedSlice(
		f.types,
		func(_ int, p id.ID[Type]) Type {
			return id.Wrap(f, p)
		},
	)
}

// Extensions returns the top level extensions defined in this file (i.e.,
// the contents of any top-level `extends` blocks).
func (f *File) Extensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.extns[:f.topLevelExtnsEnd],
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(f, p)
		},
	)
}

// AllExtensions returns all extensions defined in this file.
func (f *File) AllExtensions() seq.Indexer[Member] {
	return seq.NewFixedSlice(
		f.extns,
		func(_ int, p id.ID[Member]) Member {
			return id.Wrap(f, p)
		},
	)
}

// Extends returns the top level extend blocks in this file.
func (f *File) Extends() seq.Indexer[Extend] {
	return seq.NewFixedSlice(
		f.extends[:f.topLevelExtendsEnd],
		func(_ int, p id.ID[Extend]) Extend {
			return id.Wrap(f, p)
		},
	)
}

// AllExtends returns all extend blocks in this file.
func (f *File) AllExtends() seq.Indexer[Extend] {
	return seq.NewFixedSlice(
		f.extends,
		func(_ int, p id.ID[Extend]) Extend {
			return id.Wrap(f, p)
		},
	)
}

// AllMembers returns all fields defined in this file, including extensions
// and enum values.
func (f *File) AllMembers() iter.Seq[Member] {
	i := 0
	return iterx.Map(f.arenas.members.Values(), func(raw *rawMember) Member {
		i++
		return id.WrapRaw(f, id.ID[Member](i), raw)
	})
}

// Services returns all services defined in this file.
func (f *File) Services() seq.Indexer[Service] {
	return seq.NewFixedSlice(
		f.services,
		func(_ int, p id.ID[Service]) Service {
			return id.Wrap(f, p)
		},
	)
}

// Options returns the top level options applied to this file.
func (f *File) Options() MessageValue {
	return id.Wrap(f, f.options).AsMessage()
}

// FeatureSet returns the Editions features associated with this file.
func (f *File) FeatureSet() FeatureSet {
	return id.Wrap(f, f.features)
}

// Deprecated returns whether this file is deprecated, by returning the
// relevant option value for setting deprecation.
func (f *File) Deprecated() Value {
	if f == nil {
		return Value{}
	}
	builtins := f.builtins()
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
func (f *File) Symbols() seq.Indexer[Symbol] {
	return seq.NewFixedSlice(
		f.imported,
		func(_ int, r Ref[Symbol]) Symbol {
			return GetRef(f, r)
		},
	)
}

// FindSymbol finds a symbol among [File.Symbols] with the given fully-qualified
// name.
func (f *File) FindSymbol(fqn FullName) Symbol {
	return GetRef(f,
		f.imported.lookupBytes(f,
			unsafex.BytesAlias[[]byte](string(fqn))))
}

// topoSort sorts a graph of [File]s according to their dependency graph,
// in topological order. Files with no dependencies are yielded first.
func topoSort(files []*File) iter.Seq[*File] {
	// NOTE: This cannot panic because Files, by construction, do not contain
	// graph cycles.
	return toposort.Sort(
		files,
		func(f *File) *File { return f },
		func(f *File) iter.Seq[*File] {
			return seq.Map(
				f.imports.Directs(),
				func(i Import) *File { return i.File },
			)
		},
	)
}

func (f *File) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawType:
		return f.arenas.types.Deref(arena.Pointer[rawType](id))
	case **rawMember:
		return f.arenas.members.Deref(arena.Pointer[rawMember](id))
	case **rawReservedRange:
		return f.arenas.ranges.Deref(arena.Pointer[rawReservedRange](id))
	case **rawExtend:
		return f.arenas.extendees.Deref(arena.Pointer[rawExtend](id))
	case **rawOneof:
		return f.arenas.oneofs.Deref(arena.Pointer[rawOneof](id))

	case **rawService:
		return f.arenas.services.Deref(arena.Pointer[rawService](id))
	case **rawMethod:
		return f.arenas.methods.Deref(arena.Pointer[rawMethod](id))

	case **rawValue:
		return f.arenas.values.Deref(arena.Pointer[rawValue](id))
	case **rawMessageValue:
		return f.arenas.messages.Deref(arena.Pointer[rawMessageValue](id))
	case **rawFeatureSet:
		return f.arenas.features.Deref(arena.Pointer[rawFeatureSet](id))

	case **rawSymbol:
		return f.arenas.symbols.Deref(arena.Pointer[rawSymbol](id))

	default:
		return f.AST().FromID(id, want)
	}
}
