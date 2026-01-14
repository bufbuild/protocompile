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
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ExprRange represents a range of values, such as 1 to 4 or 5 to max.
//
// Note that max is not special syntax; it will appear as an [ExprPath] with the name "max".
//
// # Grammar
//
//	ExprRange := ExprPrefixed `to` ExprOp
type ExprRange id.Node[ExprRange, *File, *rawExprRange]

type rawExprRange struct {
	start, end id.Dyn[ExprAny, ExprKind]
	to         token.ID
}

// ExprRangeArgs is arguments for [Context.NewExprRange].
type ExprRangeArgs struct {
	Start ExprAny
	To    token.Token
	End   ExprAny
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprRange) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindRange, id.ID[ExprAny](e.ID())))
}

// Bounds returns this range's bounds. These are inclusive bounds.
func (e ExprRange) Bounds() (start, end ExprAny) {
	if e.IsZero() {
		return ExprAny{}, ExprAny{}
	}

	return id.WrapDyn(e.Context(), e.Raw().start), id.WrapDyn(e.Context(), e.Raw().end)
}

// SetBounds set the expressions for this range's bounds.
//
// Clears the respective expressions when passed a zero expression.
func (e ExprRange) SetBounds(start, end ExprAny) {
	e.Raw().start = start.ID()
	e.Raw().end = end.ID()
}

// Keyword returns the "to" keyword for this range.
func (e ExprRange) Keyword() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return id.Wrap(e.Context().Stream(), e.Raw().to)
}

// Span implements [source.Spanner].
func (e ExprRange) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}

	lo, hi := e.Bounds()
	return source.Join(lo, e.Keyword(), hi)
}
