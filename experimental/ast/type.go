// Copyright 2020-2024 Buf Technologies, Inc.
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
	"reflect"

	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	TypeKindPath TypeKind = iota + 1
	TypeKindPrefixed
	TypeKindGeneric
)

// TypeKind is a kind of type. There is one value of TypeKind for each
// Type* type in this package.
type TypeKind int8

// TypeAny is any Type* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a Type*
// type, such as [TypePath.AsAny]. It can be type-asserted back to any of
// the concrete Type* types using its own As* methods.
//
// This type is used in lieu of a putative Type interface type to avoid heap
// allocations in functions that would return one of many different Type*
// types.
type TypeAny struct {
	withContext
	raw rawType
}

// rawType is the raw representation of a type.
//
// The vast, vast majority of types are paths. To avoid needing to waste
// space for such types, we use the following encoding for rawType.
//
// First, note that if the first half of a rawPath is negative, the other
// must be zero. Thus, if the first "token" of the rawPath is negative and
// the second is not, the first is ^typeKind and the second is an index
// into a table in a Context. Otherwise, it's a path type. This logic is
// implemented in With().
type rawType rawPath

// Kind returns the kind of type this is. This is suitable for use
// in a switch statement.
func (t TypeAny) Kind() TypeKind {
	if t.raw[0] < 0 && t.raw[1] != 0 {
		return TypeKind(^t.raw[0])
	}
	return TypeKindPath
}

// AsPath converts a TypeAny into a TypePath, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsPath() TypePath {
	if t.Kind() != TypeKindPath {
		return TypePath{}
	}

	return TypePath{rawPath(t.raw).With(t)}
}

// AsPrefixed converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsPrefixed() TypePrefixed {
	if t.Kind() != TypeKindPrefixed {
		return TypePrefixed{}
	}

	ptr := arena.Pointer[rawTypePrefixed](t.ptr())
	return TypePrefixed{typeImpl[rawTypePrefixed]{
		t.withContext,
		t.Context().types.prefixes.Deref(ptr),
	}}
}

// AsGeneric converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsGeneric() TypeGeneric {
	if t.Kind() != TypeKindGeneric {
		return TypeGeneric{}
	}

	ptr := arena.Pointer[rawTypeGeneric](t.ptr())
	return TypeGeneric{typeImpl[rawTypeGeneric]{
		t.withContext,
		t.Context().types.generics.Deref(ptr),
	}}
}

// Span implements [Spanner].
func (t TypeAny) Span() Span {
	// At most one of the below will produce a non-nil type, and that will be
	// the span selected by JoinSpans. If all of them are nil, this produces
	// the nil span.
	return JoinSpans(
		t.AsPath(),
		t.AsPrefixed(),
		t.AsGeneric(),
	)
}

func (t TypeAny) ptr() arena.Untyped {
	return arena.Untyped(t.raw[1])
}

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
	if t.Nil() {
		return TypeAny{}
	}

	kind, arena := typeArena[Raw](&t.ctx.types)
	return TypeAny{
		t.withContext,
		rawType{^rawToken(kind), rawToken(arena.Compress(t.raw))},
	}
}

func (t rawType) With(c Contextual) TypeAny {
	ctx := c.Context()
	if ctx == nil || (t == rawType{}) {
		return TypeAny{}
	}

	return TypeAny{withContext{ctx}, t}
}

// types is storage for every kind of Type in a Context.raw.
type types struct {
	prefixes arena.Arena[rawTypePrefixed]
	generics arena.Arena[rawTypeGeneric]
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
	default:
		panic("unknown type type " + reflect.TypeOf(raw).Name())
	}

	return kind, arena_.(*arena.Arena[Raw]) //nolint:errcheck
}
