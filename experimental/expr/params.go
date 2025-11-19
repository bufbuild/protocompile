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
	"slices"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Params is a parameter list, which is used for all expression lists in the
// grammar.
//
// # Grammar
//
//	Params := (Param `,`)* Param?
//	Param := Expr (`:` Expr)? (`if` Expr)
type Params id.Node[Params, *Context, *rawParams]

// Param is a parameter in a [Params].
type Param struct {
	// The name for this parameter. In a map context, this would be the key;
	// in a function parameter context, this would be the argument name.
	Name Expr

	Colon token.Token
	// The expression for this parameter. In a map context, this would be the value;
	// in a function parameter context, this would be the argument type.
	Expr Expr

	If token.Token
	// A condition for the parameter; not allowed in all productions.
	Cond Expr
}

// ParamsArgs is arguments for [Nodes.NewParams].
type ParamsArgs struct {
	Brackets token.Token
}

type rawParams struct {
	params   []rawParam
	brackets token.ID
}

type rawParam struct {
	name, ty, cond             id.ID[Expr]
	nameKind, tyKind, condKind Kind
	colon, if_, comma          token.ID //nolint:revive
}

// Brackets returns the token tree for the brackets wrapping the argument list.
//
// May be zero, if the user forgot to include brackets.
func (p Params) Brackets() token.Token {
	if p.IsZero() {
		return token.Zero
	}

	return id.Wrap(p.Context().Stream(), p.Raw().brackets)
}

// SetBrackets sets the token tree for the brackets wrapping the argument list.
func (p Params) SetBrackets(brackets token.Token) {
	p.Context().Nodes().panicIfNotOurs(brackets)
	p.Raw().brackets = brackets.ID()
}

// Len implements [seq.Indexer].
func (p Params) Len() int {
	if p.IsZero() {
		return 0
	}

	return len(p.Raw().params)
}

// At implements [seq.Indexer].
func (p Params) At(n int) Param {
	v := p.Raw().params[n]
	return Param{
		Name:  id.WrapDyn(p.Context(), id.NewDyn(v.nameKind, v.name)),
		Colon: id.Wrap(p.Context().Stream(), v.colon),
		Expr:  id.WrapDyn(p.Context(), id.NewDyn(v.tyKind, v.ty)),
		If:    id.Wrap(p.Context().Stream(), v.if_),
		Cond:  id.WrapDyn(p.Context(), id.NewDyn(v.condKind, v.cond)),
	}
}

// SetAt implements [seq.Setter].
func (p Params) SetAt(n int, v Param) {
	p.Context().Nodes().panicIfNotOurs(v.Name, v.Colon, v.Expr, v.If, v.Cond)

	r := &p.Raw().params[n]

	r.name = v.Name.ID().Value()
	r.nameKind = v.Name.Kind()
	r.ty = v.Expr.ID().Value()
	r.tyKind = v.Expr.Kind()
	r.cond = v.Cond.ID().Value()
	r.condKind = v.Cond.Kind()

	r.colon = v.Colon.ID()
	r.if_ = v.If.ID()
}

// Insert implements [seq.Inserter].
func (p Params) Insert(n int, v Param) {
	p.InsertComma(n, v, token.Zero)
}

// Delete implements [seq.Inserter].
func (p Params) Delete(n int) {
	p.Raw().params = slices.Delete(p.Raw().params, n, n+1)
}

// Comma implements [Commas].
func (p Params) Comma(n int) token.Token {
	return id.Wrap(p.Context().Stream(), p.Raw().params[n].comma)
}

// AppendComma implements [Commas].
func (p Params) AppendComma(v Param, comma token.Token) {
	p.InsertComma(p.Len(), v, comma)
}

// InsertComma implements [Commas].
func (p Params) InsertComma(n int, v Param, comma token.Token) {
	p.Context().Nodes().panicIfNotOurs(v.Name, v.Colon, v.Expr, comma)

	p.Raw().params = slices.Insert(p.Raw().params, n, rawParam{
		name:     v.Name.ID().Value(),
		nameKind: v.Name.Kind(),
		ty:       v.Expr.ID().Value(),
		tyKind:   v.Expr.Kind(),
		cond:     v.Cond.ID().Value(),
		condKind: v.Cond.Kind(),
		colon:    v.Colon.ID(),
		if_:      v.If.ID(),
		comma:    comma.ID(),
	})
}

// Span implements [source.Spanner].
func (p Param) Span() source.Span {
	return source.Join(p.Name, p.Colon, p.Expr)
}

// Span implements [source.Spanner].
func (p Params) Span() source.Span {
	switch {
	case p.IsZero():
		return source.Span{}
	case !p.Brackets().IsZero():
		return p.Brackets().Span()
	case p.Len() == 0:
		return source.Span{}
	default:
		return source.Join(p.At(0), p.At(p.Len()-1))
	}
}
