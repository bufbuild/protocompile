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

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

const (
	TypeKindNil TypeKind = iota
	TypeKindPath
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
	if t.Nil() {
		return TypeKindNil
	}

	if kind, ok := t.raw.kind(); ok {
		return kind
	}
	return TypeKindPath
}

// AsPath converts a TypeAny into a TypePath, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsPath() TypePath {
	path, _ := t.raw.path(t.Context())
	// Don't need to check ok; path() returns nil on failure.
	return TypePath{path}
}

// AsPrefixed converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns nil.
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
// Otherwise, returns nil.
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

// report.Span implements [report.Spanner].
func (t TypeAny) Span() report.Span {
	// At most one of the below will produce a non-nil type, and that will be
	// the span selected by report.Join. If all of them are nil, this produces
	// the nil span.
	return report.Join(
		t.AsPath(),
		t.AsPrefixed(),
		t.AsGeneric(),
	)
}

// Trace returns a stack trace for the site at which t was constructed using
// a [Nodes].
//
// Returns "" if a trace was not recorded. See Nodes.EnableTracing.
func (t TypeAny) Trace() string {
	switch t.Kind() {
	case TypeKindGeneric:
		return t.AsGeneric().Trace()
	case TypeKindPrefixed:
		return t.AsPrefixed().Trace()
	case TypeKindPath:
		// TypeKindPath does not currently record traces.
		fallthrough
	default:
		return ""
	}
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

	kind, arena := typeArena[Raw](&t.Context().Nodes().types)
	return newTypeAny(t.Context(), wrapPathLike(kind, arena.Compress(t.raw)))
}

// Trace returns a stack trace for the site at which t was constructed using
// a [Nodes].
//
// Returns "" if a trace was not recorded. See Nodes.EnableTracing.
func (t typeImpl[Raw]) Trace() string {
	return t.Context().Nodes().traces[unsafex.Addr(t.raw)]
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
