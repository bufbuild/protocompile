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
	"cmp"
	"iter"
	"slices"
	"sync"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum symbol_kind.yaml

// Symbol is an entry in a [File]'s symbol table.
//
// [Symbol.Context] returns the context for the file which imported this
// symbol. To map this to the context in which the symbol was defined, use
// [Symbol.InDefFile].
type Symbol id.Node[Symbol, *File, *rawSymbol]

type rawSymbol struct {
	kind SymbolKind
	fqn  intern.ID
	data arena.Untyped
}

// FullName returns this symbol's fully-qualified name.
func (s Symbol) FullName() FullName {
	if s.IsZero() {
		return ""
	}
	if s.Kind() == SymbolKindScalar {
		return s.AsType().FullName()
	}
	return FullName(s.Context().session.intern.Value(s.Raw().fqn))
}

// InternedFullName returns the intern ID for [Symbol.FullName].
func (s Symbol) InternedFullName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.Raw().fqn
}

// Kind returns which kind of symbol this is.
func (s Symbol) Kind() SymbolKind {
	if s.IsZero() {
		return SymbolKindInvalid
	}
	return s.Raw().kind
}

// AsType returns the type this symbol refers to, if it is one.
func (s Symbol) AsType() Type {
	if !s.Kind().IsType() {
		return Type{}
	}
	return id.Wrap(s.Context(), id.ID[Type](s.Raw().data))
}

// AsMember returns the member this symbol refers to, if it is one.
func (s Symbol) AsMember() Member {
	if !s.Kind().IsMember() {
		return Member{}
	}
	return id.Wrap(s.Context(), id.ID[Member](s.Raw().data))
}

// AsOneof returns the oneof this symbol refers to, if it is one.
func (s Symbol) AsOneof() Oneof {
	if s.Kind() != SymbolKindOneof {
		return Oneof{}
	}
	return id.Wrap(s.Context(), id.ID[Oneof](s.Raw().data))
}

// AsService returns the service this symbol refers to, if it is one.
func (s Symbol) AsService() Service {
	if s.Kind() != SymbolKindService {
		return Service{}
	}
	return id.Wrap(s.Context(), id.ID[Service](s.Raw().data))
}

// AsMethod returns the method this symbol refers to, if it is one.
func (s Symbol) AsMethod() Method {
	if s.Kind() != SymbolKindMethod {
		return Method{}
	}
	return id.Wrap(s.Context(), id.ID[Method](s.Raw().data))
}

// FeatureSet returns the features associated with this symbol.
func (s Symbol) FeatureSet() FeatureSet {
	switch s.Kind() {
	case SymbolKindMessage, SymbolKindEnum:
		return s.AsType().FeatureSet()
	case SymbolKindField, SymbolKindEnumValue, SymbolKindExtension:
		return s.AsMember().FeatureSet()
	case SymbolKindOneof:
		return s.AsOneof().FeatureSet()
	case SymbolKindService:
		return s.AsService().FeatureSet()
	case SymbolKindMethod:
		return s.AsMethod().FeatureSet()
	default:
		return FeatureSet{}
	}
}

// Deprecated returns whether this symbol is deprecated, but returning the
// relevant option value for setting deprecation.
//
// Note that although files can be marked as deprecated, packages cannot,
// so package symbols never show up as deprecated.
func (s Symbol) Deprecated() Value {
	switch s.Kind() {
	case SymbolKindMessage, SymbolKindEnum:
		return s.AsType().Deprecated()
	case SymbolKindField, SymbolKindExtension, SymbolKindEnumValue:
		return s.AsType().Deprecated()
	case SymbolKindService:
		return s.AsService().Deprecated()
	case SymbolKindMethod:
		return s.AsMethod().Deprecated()
	default:
		return Value{}
	}
}

// Visible returns whether or not this symbol is visible according to Protobuf's
// import semantics, within s.Context().File().
func (s Symbol) Visible(in *File) bool {
	if s.Context() == in || s.Context() == primitiveCtx || s.Kind() == SymbolKindPackage {
		// Packages don't get visibility checks.
		return true
	}

	// fmt.Println(s.Context().File().Path)
	idx, imported := in.imports.byPath[s.Context().InternedPath()]
	if !imported {
		return false
	}

	imp := in.imports.files[idx]
	return imp.visible
}

// Definition returns a span for the definition site of this symbol;
// specifically, this is (typically) just an identifier.
func (s Symbol) Definition() source.Span {
	switch s.Kind() {
	case SymbolKindPackage:
		return s.Context().AST().Package().Span()
	case SymbolKindMessage, SymbolKindEnum:
		ty := s.AsType()
		if mf := ty.MapField(); !mf.IsZero() {
			return mf.TypeAST().Span()
		}

		return ty.AST().Name().Span()
	case SymbolKindField, SymbolKindEnumValue, SymbolKindExtension:
		return s.AsMember().AST().Name().Span()
	case SymbolKindOneof:
		return s.AsOneof().AST().Name().Span()
	case SymbolKindService:
		return s.AsService().AST().Name().Span()
	case SymbolKindMethod:
		return s.AsMethod().AST().Name().Span()
	}

	return source.Span{}
}

// noun returns a [taxa.Noun] for diagnostic use.
func (s Symbol) noun() taxa.Noun {
	return s.Kind().noun()
}

// noun returns a [taxa.Noun] for diagnostic use.
func (k SymbolKind) noun() taxa.Noun {
	return symbolNouns[k]
}

var symbolNouns = [...]taxa.Noun{
	SymbolKindPackage:   taxa.Package,
	SymbolKindScalar:    taxa.ScalarType,
	SymbolKindMessage:   taxa.MessageType,
	SymbolKindEnum:      taxa.EnumType,
	SymbolKindField:     taxa.Field,
	SymbolKindEnumValue: taxa.EnumValue,
	SymbolKindExtension: taxa.Extension,
	SymbolKindOneof:     taxa.Oneof,
	SymbolKindService:   taxa.Service,
	SymbolKindMethod:    taxa.Method,
}

// IsType returns whether this is a type's symbol kind.
func (k SymbolKind) IsType() bool {
	switch k {
	case SymbolKindMessage, SymbolKindEnum, SymbolKindScalar:
		return true
	default:
		return false
	}
}

// IsMember returns whether this is a field's symbol kind. This includes
// enum values, which the ir package treats as fields of enum types.
func (k SymbolKind) IsMember() bool {
	switch k {
	case SymbolKindField, SymbolKindExtension, SymbolKindEnumValue:
		return true
	default:
		return false
	}
}

// IsMessageField returns whether this is a field's symbol kind.
func (k SymbolKind) IsMessageField() bool {
	switch k {
	case SymbolKindField, SymbolKindExtension:
		return true
	default:
		return false
	}
}

// IsScope returns whether this is a symbol that defines a scope, for the
// purposes of name lookup.
func (k SymbolKind) IsScope() bool {
	switch k {
	case SymbolKindPackage, SymbolKindMessage:
		return true
	default:
		return false
	}
}

// OptionTarget returns the OptionTarget type for a symbol of this kind.
//
// Returns [OptionTargetInvalid] if there is no corresponding target for this
// type of symbol.
func (k SymbolKind) OptionTarget() OptionTarget {
	return optionTargets[k]
}

var optionTargets = [...]OptionTarget{
	SymbolKindMessage:   OptionTargetMessage,
	SymbolKindEnum:      OptionTargetEnum,
	SymbolKindField:     OptionTargetField,
	SymbolKindEnumValue: OptionTargetEnumValue,
	SymbolKindExtension: OptionTargetField,
	SymbolKindOneof:     OptionTargetOneof,
	SymbolKindService:   OptionTargetService,
	SymbolKindMethod:    OptionTargetMethod,
}

// symtab is a symbol table: a mapping of the fully qualified names of symbols
// to the entities they refer to.
//
// The elements of a symtab are sorted by the [intern.ID] of their FQN, allowing
// for O(n) merging of symbol tables.
type symtab []Ref[Symbol]

var resolveScratch = sync.Pool{
	New: func() any { return new([]byte) },
}

// symtabMerge merges the given symbol tables in the given context.
func symtabMerge(file *File, tables iter.Seq[symtab], fileForTable func(int) *File) symtab {
	return slicesx.MergeKeySeq(
		tables,

		func(which int, elem Ref[Symbol]) intern.ID {
			f := fileForTable(which)
			return GetRef(f, elem).InternedFullName()
		},

		func(which int, elem Ref[Symbol]) Ref[Symbol] {
			// We need top map the file number from src to the current one.
			src := fileForTable(which)
			if src != file {
				theirs := GetRef(src, elem)
				ours := file.imports.byPath[theirs.Context().InternedPath()]
				elem.file = int32(ours + 1)
			}

			return elem
		},
	)
}

// sort sorts this symbol table according to the value of each intern
// ID.
func (s symtab) sort(file *File) {
	slices.SortFunc(s, func(a, b Ref[Symbol]) int {
		symA := GetRef(file, a)
		symB := GetRef(file, b)
		return cmp.Compare(symA.InternedFullName(), symB.InternedFullName())
	})
}

// lookupBytes looks up a symbol with the given fully-qualified name.
func (s symtab) lookup(file *File, fqn intern.ID) Ref[Symbol] {
	idx, ok := slicesx.BinarySearchKey(s, fqn, func(r Ref[Symbol]) intern.ID {
		return GetRef(file, r).InternedFullName()
	})
	if !ok {
		return Ref[Symbol]{}
	}

	return s[idx]
}

// lookupBytes looks up a symbol with the given fully-qualified name.
func (s symtab) lookupBytes(file *File, fqn []byte) Ref[Symbol] {
	id, ok := file.session.intern.QueryBytes(fqn)
	if !ok {
		return Ref[Symbol]{}
	}
	idx, ok := slicesx.BinarySearchKey(s, id, func(r Ref[Symbol]) intern.ID {
		return GetRef(file, r).InternedFullName()
	})
	if !ok {
		return Ref[Symbol]{}
	}

	return s[idx]
}

// resolve attempts to resolve the relative path name within the given scope
// (which should itself be a possibly-empty relative path).
//
// Returns zero if the symbol is not found. If the symbol is not found due to
// Protobuf's weird double-lookup semantics around nested identifiers, this
// function will try to find the name as if this language bug did not exist, and
// will report the name it had expected to find.
//
// If skipIfNot is nil, the symbol's kind will not be checked to determine if
// we should continue climbing scopes.
//
// If candidates is not nil, debugging remarks will be appended to the
// diagnostic.
func (s symtab) resolve(
	file *File,
	scope, name FullName,
	skipIfNot func(SymbolKind) bool,
	remarks *report.Diagnostic,
) (found Ref[Symbol], expected FullName) {
	// This function implements the name resolution algorithm specified at
	// https://protobuf.com/docs/language-spec#reference-resolution.

	// Symbol resolution is not quite as simple as trying p + name for all
	// ancestors of scope. Consider the following files:
	//
	//  // a.proto
	//  package foo.bar;
	//  message M {}
	//
	//  // b.proto
	//  package foo;
	//  import "a.proto";
	//  message M {}
	//
	//  // c.proto
	//  package foo.bar.baz;
	//  import "b.proto";
	//  message N {
	//    M m = 1;
	//  }
	//
	// The candidates, in order, are:
	// - foo.bar.baz.M; does not exist.
	// - foo.bar.M; not visible.
	// - foo.M; correct answer.
	// - M; not tried.
	//
	// If we do not keep going after encountering symbols that are not visible
	// to us, we will reject valid code.

	// A similar situation happens here:
	//
	//  package foo;
	//  message M {
	//    message N {}
	//    message P {
	//      enum X { N = 1; }
	//      N n = 1;
	//    }
	//  }
	//
	// If we look up N, the candidates are foo.M.P.N, foo.M.N, foo.N, and N.
	// We will find foo.M.P.N, which is not a message or enum type, so we must
	// skip it to find the correct name, foo.M.N. This is what the accept
	// predicate is for.

	// Finally, consider the following situation, which involves partial
	// names.
	//
	//  package foo;
	//  message M {
	//    message N {}
	//    message M {
	//      M.N n = 1;
	//    }
	//  }
	//
	// The candidates are foo.M.M.N, foo.M.N, M.N. However, protoc rejects this,
	// because it actually searches for M first, and then appends the rest of
	// the path and searches for that, in two phases.
	//
	// It is not clear why protoc does this, but it does mean we need to be
	// careful in how we resolve partial names.

	scopeSearch := !name.IsIdent()
	first := name.First()

	// This needs to be a mutable byte slice, because in the loop below, we
	// delete intermediate chunks of it, e.g. a.b.c.d -> a.b.d -> a.d -> d.
	//
	// To avoid the cost of allocating a tiny slice every time we come through
	// here, we us a sync.Pool. This also means we don't have to constantly
	// zero memory that we're going to immediately overwrite.
	buf := resolveScratch.Get().(*[]byte) //nolint:errcheck
	candidate := (*buf)[:0]
	defer func() {
		// Re-using the buf pointer here allows us to avoid needing to
		// re-allocate a *[]byte to stick back into the pool.
		*buf = candidate
		resolveScratch.Put(buf)
	}()
	candidate = scope.appendToBytes(candidate, first)

	// Adapt skipIfNot to account for scopeSearch and to be ok to call if nil.
	accept := func(kind SymbolKind) bool {
		if scopeSearch {
			return kind.IsScope()
		}
		return skipIfNot == nil || skipIfNot(kind)
	}

again:
	for {
		r := s.lookupBytes(file, candidate)
		remarks.Apply(report.Debugf("candidate: `%s`", candidate))

		if !r.IsZero() {
			found = r
			sym := GetRef(file, r)
			if sym.Visible(file) && accept(sym.Kind()) {
				// If the symbol is not visible, keep looking; we may find
				// another match that is actually visible.
				break
			}
		}

		if scope == "" {
			// Out of places to look. This is probably a fail.
			break
		}
		oldLen := len(scope)
		scope = scope.Parent()
		if scope == "" {
			oldLen++
		}
		// Delete in-place to avoid spamming allocations for each candidate.
		candidate = slices.Delete(candidate, len(scope), oldLen)
	}

	if scopeSearch {
		// Now search for the full name inside of the scope we found.
		candidate = append(candidate, name[len(first):]...)
		found = s.lookupBytes(file, candidate)
		if found.IsZero() {
			// Try again, this time using the full candidate name. This happens
			// expressly for the purpose of diagnostics.
			scopeSearch = false
			// Need to clear the found scope, since otherwise we might get a weird
			// error message where we say that we found the parent scope.
			found = Ref[Symbol]{}
			expected = FullName(candidate)
			goto again
		}
	}

	foundFile := found.Context(file)
	if foundFile != file {
		file.imports.MarkUsed(foundFile)
	}

	return found, expected
}
