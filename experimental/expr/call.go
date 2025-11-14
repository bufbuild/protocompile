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
)

// Call is a function call/indexing expression, consisting of an expression
// followed by bracketed [Params].
//
// # Grammar
//
//	Call := Expr (`(` Params `)` | `[` Params `]` | `{` Params `}`)
type Call id.Node[Call, *Context, *rawCall]

// CallArgs is arguments for [Nodes.NewCall].
type CallArgs struct {
	Callee Expr
	Args   Params
}

type rawCall struct {
	callee id.Dyn[Expr, Kind]
	args   id.ID[Params]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Call) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindCall, id.ID[Expr](e.ID())))
}

// Callee returns the expression's callee.
func (e Call) Callee() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().callee)
}

// Args returns the expression's arguments.
func (e Call) Args() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().args)
}

// Brackets returns the brackets for this call.
func (e Call) Brackets() token.Token {
	return e.Args().Brackets()
}

// Span implements [source.Spanner].
func (e Call) Span() source.Span {
	return source.Join(e.Callee(), e.Args())
}
