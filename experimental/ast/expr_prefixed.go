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

const (
	ExprPrefixUnknown ExprPrefix = iota
	ExprPrefixMinus
)

// TypePrefix is a prefix for an expression, such as a minus sign.
type ExprPrefix int8

// ExprPrefixByName looks up a prefix kind by name.
//
// If name is not a known prefix, returns [ExprPrefixUnknown].
func ExprPrefixByName(name string) ExprPrefix {
	switch name {
	case "-":
		return ExprPrefixMinus
	default:
		return ExprPrefixUnknown
	}
}

// ExprPrefixed is an expression prefixed with an operator.
type ExprPrefixed struct{ exprImpl[rawExprPrefixed] }

type rawExprPrefixed struct {
	prefix rawToken
	expr   rawExpr
}

// ExprPrefixedArgs is arguments for [Context.NewExprPrefixed].
type ExprPrefixedArgs struct {
	Prefix Token
	Expr   ExprAny
}

// Prefix returns this expression's prefix.
func (e ExprPrefixed) Prefix() ExprPrefix {
	return ExprPrefixByName(e.PrefixToken().Text())
}

// Prefix returns the token representing this expression's prefix.
func (e ExprPrefixed) PrefixToken() Token {
	return e.raw.prefix.With(e)
}

// Expr returns the expression the prefix is applied to.
func (e ExprPrefixed) Expr() ExprAny {
	return e.raw.expr.With(e)
}

// SetExpr sets the expression that the prefix is applied to.
//
// If passed nil, this clears the expression.
func (e ExprPrefixed) SetExpr(expr ExprAny) {
	e.raw.expr = expr.raw
}

// Span implements [Spanner].
func (e ExprPrefixed) Span() Span {
	return JoinSpans(e.PrefixToken(), e.Expr())
}
