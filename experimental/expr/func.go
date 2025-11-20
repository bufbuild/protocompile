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

// Func is a function literal or definition. A function definition is simply
// a function that includes a name after the func keyword.
//
// # Grammar
//
//	Func := `func` Ident? `(` Params `)` (`->` Expr Block? | Expr)?
type Func id.Node[Func, *Context, *rawFunc]

// FuncArgs is arguments for [Nodes.NewFunc].
type FuncArgs struct {
	Func   token.Token
	Name   token.Token
	Params Params
	Arrow  token.Token
	Return Expr
	Body   Expr
}

type rawFunc struct {
	funcT  token.ID
	name   token.ID
	params id.ID[Params]
	arrow  token.ID
	ret    id.Dyn[Expr, Kind]
	body   id.Dyn[Expr, Kind]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Func) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindFunc, id.ID[Expr](e.ID())))
}

// Keywords returns this expression's func token.
func (e Func) FuncToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().funcT)
}

// Name returns the declared function's name (if it is a declaration).
func (e Func) Name() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().name)
}

// Params returns the function's input parameters.
func (e Func) Params() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().params)
}

// Arrow returns the arrow that indicates a return type.
func (e Func) Arrow() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().arrow)
}

// Return returns the type expression for this function's return type.
func (e Func) Return() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().ret)
}

// Body returns this function's body expression.
func (e Func) Body() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().body)
}

// Span implements [source.Spanner].
func (e Func) Span() source.Span {
	return source.Join(e.FuncToken(), e.Name(), e.Params(), e.Return(), e.Body())
}
