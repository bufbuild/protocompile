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
	"github.com/bufbuild/protocompile/experimental/token"
)

// Token is an expression corresponding to a single token: a string, a
// number, or an identifier.
//
// # Grammar
//
//	Token := token.Number | token.String | token.Ident
type Token struct {
	ExprContext *Context
	// The token backing this expression. Must be [token.String], [token.Number],
	// or [token.Ident].
	token.Token
}

// Context returns this token's context.
//
// This returns a [Context] rather than a [token.Stream], which would otherwise
// be returned because Token embeds [token.Token].
func (e Token) Context() *Context {
	return e.ExprContext
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e Token) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}

	return id.WrapDyn(
		e.Context(),
		id.NewDyn(KindToken, id.ID[Expr](e.ID())),
	)
}
