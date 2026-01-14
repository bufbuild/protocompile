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
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// ExprPrefixed is an expression prefixed with an operator.
//
// # Grammar
//
//	ExprPrefix := `-` ExprSolo
type ExprPrefixed id.Node[ExprPrefixed, *File, *rawExprPrefixed]

type rawExprPrefixed struct {
	prefix token.ID
	expr   id.Dyn[ExprAny, ExprKind]
}

// ExprPrefixedArgs is arguments for [Context.NewExprPrefixed].
type ExprPrefixedArgs struct {
	Prefix token.Token
	Expr   ExprAny
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprPrefixed) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindPrefixed, id.ID[ExprAny](e.ID())))
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

	return id.Wrap(e.Context().Stream(), e.Raw().prefix)
}

// Expr returns the expression the prefix is applied to.
func (e ExprPrefixed) Expr() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), e.Raw().expr)
}

// SetExpr sets the expression that the prefix is applied to.
//
// If passed zero, this clears the expression.
func (e ExprPrefixed) SetExpr(expr ExprAny) {
	e.Raw().expr = expr.ID()
}

// source.Span implements [source.Spanner].
func (e ExprPrefixed) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}

	return source.Join(e.PrefixToken(), e.Expr())
}
