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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// ExprPrefixed is an expression prefixed with an operator.
//
// # Grammar
//
//	ExprPrefix := `-` ExprSolo
type ExprPrefixed struct{ exprImpl[rawExprPrefixed] }

type rawExprPrefixed struct {
	prefix token.ID
	expr   rawExpr
}

// ExprPrefixedArgs is arguments for [Context.NewExprPrefixed].
type ExprPrefixedArgs struct {
	Prefix token.Token
	Expr   ExprAny
}

// Prefix returns this expression's prefix.
//
// Returns [keyword.Unknown] if [TypePrefixed.PrefixToken] does not contain
// a known prefix.
func (e ExprPrefixed) Prefix() keyword.Keyword {
	return e.PrefixToken().Keyword()
}

// PrefixToken returns the token representing this expression's prefix.
func (e ExprPrefixed) PrefixToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return id.Get(token.Context(e.Context()), e.raw.prefix)
}

// Expr returns the expression the prefix is applied to.
func (e ExprPrefixed) Expr() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return newExprAny(e.Context(), e.raw.expr)
}

// SetExpr sets the expression that the prefix is applied to.
//
// If passed zero, this clears the expression.
func (e ExprPrefixed) SetExpr(expr ExprAny) {
	e.raw.expr = expr.raw
}

// report.Span implements [report.Spanner].
func (e ExprPrefixed) Span() report.Span {
	if e.IsZero() {
		return report.Span{}
	}

	return report.Join(e.PrefixToken(), e.Expr())
}
