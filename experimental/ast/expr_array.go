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
	"slices"
)

// ExprArray represents an array of expressions between square brackets.
//
// ExprArray implements [Commas[ExprAny]].
type ExprArray struct{ exprImpl[rawExprArray] }

type rawExprArray struct {
	brackets rawToken
	args     []withComma[rawExpr]
}

var _ Commas[ExprAny] = ExprArray{}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e ExprArray) Brackets() Token {
	return e.raw.brackets.With(e)
}

// Len implements [Slice].
func (e ExprArray) Len() int {
	return len(e.raw.args)
}

// At implements [Slice].
func (e ExprArray) At(n int) ExprAny {
	return e.raw.args[n].Value.With(e)
}

// Iter implements [Slice].
func (e ExprArray) Iter(yield func(int, ExprAny) bool) {
	for i, arg := range e.raw.args {
		if !yield(i, arg.Value.With(e)) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprArray) Append(expr ExprAny) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter].
func (e ExprArray) Insert(n int, expr ExprAny) {
	e.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter].
func (e ExprArray) Delete(n int) {
	e.raw.args = slices.Delete(e.raw.args, n, n+1)
}

// Comma implements [Commas].
func (e ExprArray) Comma(n int) Token {
	return e.raw.args[n].Comma.With(e)
}

// AppendComma implements [Commas].
func (e ExprArray) AppendComma(expr ExprAny, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprArray) InsertComma(n int, expr ExprAny, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)

	e.raw.args = slices.Insert(e.raw.args, n, withComma[rawExpr]{expr.raw, comma.raw})
}

// Span implements [Spanner].
func (e ExprArray) Span() Span {
	return e.Brackets().Span()
}
