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

//nolint:dupword // Disable for whole file, because the error is in a comment.
package ast

import (
	"iter"
	"reflect"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
)

// TypeAny is any Type* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a Type*
// type, such as [TypePath.AsAny]. It can be type-asserted back to any of
// the concrete Type* types using its own As* methods.
//
// This type is used in lieu of a putative Type interface type to avoid heap
// allocations in functions that would return one of many different Type*
// types.
//
// # Grammar
//
//	Type := TypePath | TypePrefixed | TypeGeneric
//
// Note that parsing a type cannot always be greedy. Consider that, if parsed
// as a type, "optional optional foo" could be parsed as:
//
//	TypePrefix{Optional, TypePrefix{Optional, TypePath("foo")}}
//
// However, if we want to parse a type followed by a [Path], it needs to parse
// as follows:
//
//	TypePrefix{Optional, TypePath("optional")}, Path("foo")
//
// Thus, parsing a type is greedy except when the containing production contains
// "Type Path?" or similar, in which case parsing must be greedy up to the last
// [Path] it would otherwise consume.
type TypeAny struct {
	withContext // Must be nil if raw is nil.

	raw rawType
}

type rawType = pathLike[TypeKind]

func newTypeAny(ctx Context, t rawType) TypeAny {
	if ctx == nil || (t == rawType{}) {
		return TypeAny{}
	}
	return TypeAny{internal.NewWith(ctx), t}
}

// Kind returns the kind of type this is. This is suitable for use
// in a switch statement.
func (t TypeAny) Kind() TypeKind {
	if t.IsZero() {
		return TypeKindInvalid
	}

	if kind, ok := t.raw.kind(); ok {
		return kind
	}
	return TypeKindPath
}

// AsError converts a TypeAny into a TypeError, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsError() TypeError {
	ptr := unwrapPathLike[arena.Pointer[rawTypeError]](TypeKindError, t.raw)
	if ptr.Nil() {
		return TypeError{}
	}

	return TypeError{typeImpl[rawTypeError]{
		t.withContext,
		t.Context().Nodes().types.errors.Deref(ptr),
	}}
}

// AsPath converts a TypeAny into a TypePath, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsPath() TypePath {
	path, _ := t.raw.path(t.Context())
	// Don't need to check ok; path() returns zero on failure.
	return TypePath{path}
}

// AsPrefixed converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsPrefixed() TypePrefixed {
	ptr := unwrapPathLike[arena.Pointer[rawTypePrefixed]](TypeKindPrefixed, t.raw)
	if ptr.Nil() {
		return TypePrefixed{}
	}

	return TypePrefixed{typeImpl[rawTypePrefixed]{
		t.withContext,
		t.Context().Nodes().types.prefixes.Deref(ptr),
	}}
}

// AsGeneric converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsGeneric() TypeGeneric {
	ptr := unwrapPathLike[arena.Pointer[rawTypeGeneric]](TypeKindGeneric, t.raw)
	if ptr.Nil() {
		return TypeGeneric{}
	}

	return TypeGeneric{typeImpl[rawTypeGeneric]{
		t.withContext,
		t.Context().Nodes().types.generics.Deref(ptr),
	}}
}

// Prefixes is an iterator over all [TypePrefix]es wrapping this type.
func (t TypeAny) Prefixes() iter.Seq[TypePrefixed] {
	return func(yield func(TypePrefixed) bool) {
		for t.Kind() == TypeKindPrefixed {
			prefixed := t.AsPrefixed()
			if !yield(prefixed) {
				return
			}
			t = prefixed.Type()
		}
	}
}

// RemovePrefixes removes all [TypePrefix] values wrapping this type.
func (t TypeAny) RemovePrefixes() TypeAny {
	for t.Kind() == TypeKindPrefixed {
		t = t.AsPrefixed().Type()
	}
	return t
}

// report.Span implements [report.Spanner].
func (t TypeAny) Span() report.Span {
	// At most one of the below will produce a non-zero type, and that will be
	// the span selected by report.Join. If all of them are zero, this produces
	// the zero span.
	return report.Join(
		t.AsPath(),
		t.AsPrefixed(),
		t.AsGeneric(),
	)
}

// TypeError represents an unrecoverable parsing error in a type context.
//
// This type is so named to adhere to package ast's naming convention. It does
// not represent a "type error" as in "type-checking failure".
type TypeError struct{ typeImpl[rawTypeError] }

// Span implements [report.Spanner].
func (t TypeError) Span() report.Span {
	if t.IsZero() {
		return report.Span{}
	}

	return report.Span(*t.raw)
}

type rawTypeError report.Span

// typeImpl is the common implementation of pointer-like Type* types.
type typeImpl[Raw any] struct {
	// NOTE: These fields are sorted by alignment.
	withContext
	raw *Raw
}

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (t typeImpl[Raw]) AsAny() TypeAny {
	if t.IsZero() {
		return TypeAny{}
	}

	kind, arena := typeArena[Raw](&t.Context().Nodes().types)
	return newTypeAny(t.Context(), wrapPathLike(kind, arena.Compress(t.raw)))
}

// types is storage for every kind of Type in a Context.raw.
type types struct {
	prefixes arena.Arena[rawTypePrefixed]
	generics arena.Arena[rawTypeGeneric]
	errors   arena.Arena[rawTypeError]
}

func typeArena[Raw any](types *types) (TypeKind, *arena.Arena[Raw]) {
	var (
		kind TypeKind
		raw  Raw
		// Needs to be an any because Go doesn't know that only the case below
		// with the correct type for arena_ (if it were *arena.Arena[Raw]) will
		// be evaluated.
		arena_ any //nolint:revive // Named arena_ to avoid clashing with package arena.
	)

	switch any(raw).(type) {
	case rawTypePrefixed:
		kind = TypeKindPrefixed
		arena_ = &types.prefixes
	case rawTypeGeneric:
		kind = TypeKindGeneric
		arena_ = &types.generics
	case rawTypeError:
		kind = TypeKindError
		arena_ = &types.errors
	default:
		panic("unknown type type " + reflect.TypeOf(raw).Name())
	}

	return kind, arena_.(*arena.Arena[Raw]) //nolint:errcheck
}
