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

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ExprArray represents an array of expressions between square brackets.
//
// ExprArray implements [Commas][ExprAny].
//
// # Grammar
//
//	ExprArray := `[` (Expr ,)* Expr? `]`
type ExprArray struct{ exprImpl[rawExprArray] }

type rawExprArray struct {
	brackets token.ID
	args     []withComma[rawExpr]
}

var _ Commas[ExprAny] = ExprArray{}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e ExprArray) Brackets() token.Token {
	return e.raw.brackets.In(e.Context())
}

// Len implements [Slice].
func (e ExprArray) Len() int {
	return len(e.raw.args)
}

// At implements [Slice].
func (e ExprArray) At(n int) ExprAny {
	return newExprAny(e.Context(), e.raw.args[n].Value)
}

// Iter implements [Slice].
func (e ExprArray) Iter(yield func(int, ExprAny) bool) {
	for i, arg := range e.raw.args {
		if !yield(i, newExprAny(e.Context(), arg.Value)) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprArray) Append(expr ExprAny) {
	e.InsertComma(e.Len(), expr, token.Nil)
}

// Insert implements [Inserter].
func (e ExprArray) Insert(n int, expr ExprAny) {
	e.InsertComma(n, expr, token.Nil)
}

// Delete implements [Inserter].
func (e ExprArray) Delete(n int) {
	e.raw.args = slices.Delete(e.raw.args, n, n+1)
}

// Comma implements [Commas].
func (e ExprArray) Comma(n int) token.Token {
	return e.raw.args[n].Comma.In(e.Context())
}

// AppendComma implements [Commas].
func (e ExprArray) AppendComma(expr ExprAny, comma token.Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprArray) InsertComma(n int, expr ExprAny, comma token.Token) {
	e.Context().Nodes().panicIfNotOurs(expr, comma)

	e.raw.args = slices.Insert(e.raw.args, n, withComma[rawExpr]{expr.raw, comma.ID()})
}

// Span implements [report.Spanner].
func (e ExprArray) Span() report.Span {
	return e.Brackets().Span()
}
