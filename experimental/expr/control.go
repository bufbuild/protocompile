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

// Control is a control flow expression, such as a break or continue, with an
// optional condition at the end.
//
// # Grammar
//
//	Switch := (`break` | `continue` | `return`) Params (`if` Expr)?
type Control id.Node[Control, *Context, *rawControl]

// ControlArgs is arguments for [Nodes.NewControl].
type ControlArgs struct {
	Keyword token.Token
	Args    Params

	If        token.Token
	Condition Expr
}

type rawControl struct {
	kw, ifT token.ID
	args    id.ID[Params]
	cond    id.Dyn[Expr, Kind]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Control) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindControl, id.ID[Expr](e.ID())))
}

// Kind returns what kind of control flow expression this is.
func (e Control) Kind() keyword.Keyword {
	return e.KeywordToken().Keyword()
}

// KeywordToken returns the token for the keyword for this expression.
func (e Control) KeywordToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().kw)
}

// IfToken returns the token for an `if` after this expression, if it has an
// associated condition.
func (e Control) IfToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().ifT)
}

// Args returns the arguments for this expression.
func (e Control) Args() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().args)
}

// Condition returns the condition expression for this expression.
//
// This will be zero if the expression is not conditioned.
func (e Control) Condition() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().cond)
}

// Span implements [source.Spanner].
func (e Control) Span() source.Span {
	return source.Join(e.KeywordToken(), e.Args(), e.IfToken(), e.Condition())
}
