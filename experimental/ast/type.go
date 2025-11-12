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

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
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
type TypeAny id.DynNode[TypeAny, TypeKind, *File]

// AsError converts a TypeAny into a TypeError, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (t TypeAny) AsError() TypeError {
	if t.Kind() != TypeKindError {
		return TypeError{}
	}
	return id.Wrap(t.Context(), id.ID[TypeError](t.ID().Value()))
}

// AsPath converts a TypeAny into a TypePath, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsPath() TypePath {
	if t.Kind() != TypeKindPath {
		return TypePath{}
	}

	start, end := t.ID().Raw()
	return TypePath{Path: PathID{start: token.ID(start), end: token.ID(end)}.In(t.Context())}
}

// AsPrefixed converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsPrefixed() TypePrefixed {
	if t.Kind() != TypeKindPrefixed {
		return TypePrefixed{}
	}
	return id.Wrap(t.Context(), id.ID[TypePrefixed](t.ID().Value()))
}

// AsGeneric converts a TypeAny into a TypePrefix, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (t TypeAny) AsGeneric() TypeGeneric {
	if t.Kind() != TypeKindGeneric {
		return TypeGeneric{}
	}
	return id.Wrap(t.Context(), id.ID[TypeGeneric](t.ID().Value()))
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

// source.Span implements [source.Spanner].
func (t TypeAny) Span() source.Span {
	// At most one of the below will produce a non-zero type, and that will be
	// the span selected by source.Join. If all of them are zero, this produces
	// the zero span.
	return source.Join(
		t.AsPath(),
		t.AsPrefixed(),
		t.AsGeneric(),
	)
}

// TypeError represents an unrecoverable parsing error in a type context.
//
// This type is so named to adhere to package ast's naming convention. It does
// not represent a "type error" as in "type-checking failure".
type TypeError id.Node[TypeError, *File, *rawTypeError]

type rawTypeError source.Span

// AsAny type-erases this type value.
//
// See [TypeAny] for more information.
func (t TypeError) AsAny() TypeAny {
	if t.IsZero() {
		return TypeAny{}
	}

	return id.WrapDyn(t.Context(), id.NewDyn(TypeKindError, id.ID[TypeAny](t.ID())))
}

// Span implements [source.Spanner].
func (t TypeError) Span() source.Span {
	if t.IsZero() {
		return source.Span{}
	}

	return source.Span(*t.Raw())
}

func (TypeKind) DecodeDynID(lo, hi int32) TypeKind {
	switch {
	case lo == 0:
		return TypeKindInvalid
	case lo < 0 && hi > 0:
		return TypeKind(^lo)
	default:
		return TypeKindPath
	}
}

func (k TypeKind) EncodeDynID(value int32) (int32, int32, bool) {
	return ^int32(k), value, true
}

// types is storage for every kind of Type in a Context.Raw().
type types struct {
	prefixes arena.Arena[rawTypePrefixed]
	generics arena.Arena[rawTypeGeneric]
	errors   arena.Arena[rawTypeError]
}
