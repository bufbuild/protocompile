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

// For is a loop which assigns items out of an iterator to variables in
// a [Params].
//
// # Grammar
//
//	For := `for` Params `in` Expr Block
type For id.Node[For, *Context, *rawFor]

// ForArgs is arguments for [Nodes.NewFor].
type ForArgs struct {
	For      token.Token
	Vars     Params
	In       token.Token
	Iterator Expr
	Block    Block
}

type rawFor struct {
	iter  id.Dyn[Expr, Kind]
	forT  token.ID
	inT   token.ID
	vars  id.ID[Params]
	block id.ID[Block]
}

// AsAny type-erases this type value.
//
// See [Expr] for more information.
func (e For) AsAny() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), id.NewDyn(KindFor, id.ID[Expr](e.ID())))
}

// Keywords returns this expression's for token.
func (e For) ForToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().forT)
}

// Keywords returns this expression's in token.
func (e For) InToken() token.Token {
	if e.IsZero() {
		return token.Zero
	}
	return id.Wrap(e.Context().Stream(), e.Raw().inT)
}

// Vars returns the variables this for assigns to.
func (e For) Vars() Params {
	if e.IsZero() {
		return Params{}
	}
	return id.Wrap(e.Context(), e.Raw().vars)
}

// Iterator returns the expression this loop iterates over.
func (e For) Iterator() Expr {
	if e.IsZero() {
		return Expr{}
	}
	return id.WrapDyn(e.Context(), e.Raw().iter)
}

// Block returns this for's block.
func (e For) Block() Block {
	if e.IsZero() {
		return Block{}
	}
	return id.Wrap(e.Context(), e.Raw().block)
}

// Span implements [source.Spanner].
func (e For) Span() source.Span {
	return source.Join(e.ForToken(), e.Vars(), e.InToken(), e.Iterator(), e.Block())
}
