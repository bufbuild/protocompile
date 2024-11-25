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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ExprRange represents a range of values, such as 1 to 4 or 5 to max.
//
// Note that max is not special syntax; it will appear as an [ExprPath] with the name "max".
type ExprRange struct{ exprImpl[rawExprRange] }

type rawExprRange struct {
	start, end rawExpr
	to         token.ID
}

// ExprRangeArgs is arguments for [Context.NewExprRange].
type ExprRangeArgs struct {
	Start ExprAny
	To    token.Token
	End   ExprAny
}

// Bounds returns this range's bounds. These are inclusive bounds.
func (e ExprRange) Bounds() (start, end ExprAny) {
	return newExprAny(e.Context(), e.raw.start), newExprAny(e.Context(), e.raw.end)
}

// SetBounds set the expressions for this range's bounds.
//
// Clears the respective expressions when passed a nil expression.
func (e ExprRange) SetBounds(start, end ExprAny) {
	e.raw.start = start.raw
	e.raw.end = end.raw
}

// Keyword returns the "to" keyword for this range.
func (e ExprRange) Keyword() token.Token {
	return e.raw.to.In(e.Context())
}

// Span implements [report.Spanner].
func (e ExprRange) Span() report.Span {
	lo, hi := e.Bounds()
	return report.Join(lo, e.Keyword(), hi)
}
