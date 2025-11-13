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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// ExprStruct is an array/map/tuple expression, consisting of a bracketed
// [Params]. There are four kinds of expression represented by this node:
//
//  1. Groupings. A single expression in (...) with no key or condition, and
//     no trailing comma.
//
//  2. Tuples. Any other sequence of expressions in (...). Keys and conditions
//     are not allowed.
//
//  3. Slices. Zero or more expressions without keys in [...].
//
//  4. Maps. Any other sequence of expressions in [...]. All entries must have
//     keys.
//
// [ExprStruct.Kind] classifies which of these kinds it is.
//
// # Grammar
//
//	ExprStruct := `(` Params `)` | `[` Params `]`
type ExprStruct id.Node[ExprStruct, *File, *rawParams]

// AsAny type-erases this type value.
//
// See [ExprAny] for more information.
func (e ExprStruct) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindStruct, id.ID[ExprAny](e.ID())))
}

// Kind returns which kind of struct expression this is.
//
// This will try to return a valid kind even if this expression contains an
// invalid combination, such as [a, b: c].
func (e ExprStruct) Kind() StructKind {
	if e.IsZero() {
		return StructKindInvalid
	}

	if e.Brackets().Keyword() == keyword.Parens {
		entries := e.Entries()
		if entries.Len() != 1 {
			return StructKindTuple
		}

		first := entries.At(0)
		if !first.Name.IsZero() || !first.Cond.IsZero() {
			return StructKindTuple
		}

		if !entries.Comma(0).IsZero() {
			return StructKindTuple
		}

		return StructKindGrouping
	}

	for entry := range seq.Values(e.Entries()) {
		if !entry.Name.IsZero() {
			return StructKindMap
		}
	}

	return StructKindSlice
}

// Entries returns the expression's entries.
func (e ExprStruct) Entries() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.WrapRaw(e.Context(), id.ID[Params](e.ID()), e.Raw())
}

// Brackets returns the brackets from this
func (e ExprStruct) Brackets() token.Token {
	return e.Entries().Brackets()
}

// Span implements [source.Spanner].
func (e ExprStruct) Span() source.Span {
	return e.Entries().Span()
}
