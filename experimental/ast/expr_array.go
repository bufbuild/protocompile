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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// ExprArray represents an array of expressions between square brackets.
//
// # Grammar
//
//	ExprArray := `[` (ExprJuxta `,`?)*`]`
type ExprArray struct{ exprImpl[rawExprArray] }

type rawExprArray struct {
	brackets token.ID
	args     slicesx.Inline[rawExpr]
	commas   slicesx.Inline[token.ID]
}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e ExprArray) Brackets() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return e.raw.brackets.In(e.Context())
}

// Elements returns the sequence of expressions in this array.
func (e ExprArray) Elements() Commas[ExprAny] {
	var (
		args *slicesx.Inline[rawExpr]
		toks *slicesx.Inline[token.ID]
	)
	if !e.IsZero() {
		args = &e.raw.args
		toks = &e.raw.commas
	}

	// A single return here promotes devirtualization of both the interface
	// and the funcvals within.
	return commas[ExprAny, rawExpr]{
		ctx: e.Context(),
		InserterWrapper2: seq.WrapInserter2(
			args, toks,
			func(r rawExpr, _ token.ID) ExprAny {
				return newExprAny(e.Context(), r)
			},
			func(r ExprAny) (rawExpr, token.ID) {
				e.Context().Nodes().panicIfNotOurs(r)
				return r.raw, 0
			},
		),
	}
}

// Span implements [report.Spanner].
func (e ExprArray) Span() report.Span {
	if e.IsZero() {
		return report.Span{}
	}

	return e.Brackets().Span()
}
