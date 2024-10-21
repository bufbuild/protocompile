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

// ExprRange represents a range of values, such as 1 to 4 or 5 to max.
//
// Note that max is not special syntax; it will appear as an [ExprPath] with the name "max".
type ExprRange struct{ exprImpl[rawExprRange] }

type rawExprRange struct {
	start, end rawExpr
	to         rawToken
}

// ExprRangeArgs is arguments for [Context.NewExprRange].
type ExprRangeArgs struct {
	Start ExprAny
	To    Token
	End   ExprAny
}

// Bounds returns this range's bounds. These are inclusive bounds.
func (e ExprRange) Bounds() (start, end ExprAny) {
	return e.raw.start.With(e), e.raw.end.With(e)
}

// SetBounds set the expressions for this range's bounds.
//
// Clears the respective expressions when passed a nil expression.
func (e ExprRange) SetBounds(start, end ExprAny) {
	e.raw.start = start.raw
	e.raw.end = end.raw
}

// Keyword returns the "to" keyword for this range.
func (e ExprRange) Keyword() Token {
	return e.raw.to.With(e)
}

// Span implements [Spanner].
func (e ExprRange) Span() Span {
	lo, hi := e.Bounds()
	return JoinSpans(lo, e.Keyword(), hi)
}
