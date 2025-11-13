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

// ExprCall is a function call/indexing expression, consisting of an expression
// followed by bracketed [Params].
//
// # Grammar
//
//	ExprCall := Expr (`(` Params `)` | `[` Params `]`)
type ExprCall id.Node[ExprCall, *File, *rawExprCall]

type rawExprCall struct {
	callee id.Dyn[ExprAny, ExprKind]
	args   id.ID[Params]
}

// AsAny type-erases this type value.
//
// See [ExprAny] for more information.
func (e ExprCall) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindCall, id.ID[ExprAny](e.ID())))
}

// Callee returns the expression's callee.
func (e ExprCall) Callee() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}
	return id.WrapDyn(e.Context(), e.Raw().callee)
}

// Args returns the expression's arguments.
func (e ExprCall) Args() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().args)
}

// Brackets returns the brackets for this call.
func (e ExprCall) Brackets() token.Token {
	return e.Args().Brackets()
}

// IsCall returns whether this is a function call expression.
func (e ExprCall) IsCall() bool {
	return e.Args().Brackets().Keyword() == keyword.Parens
}

// IsIndex returns whether this is a indexing expression.
func (e ExprCall) IsIndex() bool {
	return e.Args().Brackets().Keyword() == keyword.Brackets
}

// Span implements [source.Spanner].
func (e ExprCall) Span() source.Span {
	return source.Join(e.Callee(), e.Args())
}
