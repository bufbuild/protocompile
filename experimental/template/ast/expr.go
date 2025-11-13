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
)

// ExprAny is any Expr* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a Expr*
// type, such as [ExprToken.AsAny]. It can be type-asserted back to any of
// the concrete Expr* types using its own As* methods.
//
// This type is used in lieu of a putative ExprAny interface type to avoid heap
// allocations in functions that would return one of many different Expr*
// types.
//
// Note that the expression and type grammars for the language are the same, so
// ExprAny appears in places you might expect to see a "TypeAny" node.
//
// # Grammar
//
//	Expr      :=
type ExprAny id.DynNode[ExprAny, ExprKind, *File]

// AsLiteral converts a ExprAny into a ExprLiteral, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsLiteral() ExprToken {
	if e.Kind() != ExprKindToken {
		return ExprToken{}
	}
	return ExprToken{
		File:  e.Context(),
		Token: id.Wrap(e.Context().Stream(), id.ID[token.Token](e.ID().Value())),
	}
}

// Span implements [source.Spanner].
func (e ExprAny) Span() source.Span {
	return source.Join(
		e.AsLiteral(),
	)
}

func (ExprKind) DecodeDynID(lo, _ int32) ExprKind {
	return ExprKind(lo)
}

func (k ExprKind) EncodeDynID(value int32) (int32, int32, bool) {
	return int32(k), value, true
}
