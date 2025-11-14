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

package expr

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Op is an operator expression, consisting of one or two expressions and
// an operator. This subsumes all of the operator expressions in the grammar,
// including property, assignment, and range expressions.
//
// Unary do not have a left-hand side.
//
// # Grammar
//
//	ExprToken := Expr Op Expr
//
// Here, Op is any of the following tokens, ordered from least to greatest
// precedence (later tokens bind more tightly):
//
//	= := += -= *= /= %=
//	,
//	or
//	and
//	== != < > <= >=
//	.. ..= to
//	+ -
//	* / %
//	- not (unary)
//	.
//
// Note that ExprCall has higher precedence than ., so -x.f() will group as
// -((x.f)()).
type Op id.Node[Op, *Context, *rawOp]

// OpArgs is arguments for [Nodes.NewOp].
type OpArgs struct {
	Left, Right Expr
	Op          token.Token
}

type rawOp struct {
	left, right id.Dyn[Expr, Kind]
	op          token.ID
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Op) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindOp, id.ID[Expr](e.ID())))
}

// Left returns the left-hand side of this expression. Unary operators do
// not have a left-hand side.
func (e Op) Left() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().left)
}

// Right returns the right-hand side of this expression.
func (e Op) Right() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().right)
}

// IsUnary returns whether this is an unary operation.
func (e Op) IsUnary() bool {
	return e.Left().IsZero() && !e.Right().IsZero()
}

// Operator returns this expression's operator.
func (e Op) Operator() keyword.Keyword {
	return e.OperatorToken().Keyword()
}

// OperatorToken returns the token for this expression's operator.
func (e Op) OperatorToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().op)
}

// Span implements [source.Spanner].
func (e Op) Span() source.Span {
	return source.Join(e.Left(), e.OperatorToken(), e.Right())
}
