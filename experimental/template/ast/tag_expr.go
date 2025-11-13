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

// TagImport is a tag which evaluates some expression.
//
// # Grammar
//
//	TagExpr := `[:` Expr `:]`
type TagExpr id.Node[TagExpr, *File, *rawTagExpr]

// TagExprArgs sis arguments for [Nodes.NewTagExpr].
type TagExprArgs struct {
	Brackets token.Token
	Expr     ExprAny
}

type rawTagExpr struct {
	brackets token.ID
	expr     id.Dyn[ExprAny, ExprKind]
}

// AsAny type-erases this tag value.
//
// See [TagAny] for more information.
func (t TagExpr) AsAny() TagAny {
	if t.IsZero() {
		return TagAny{}
	}
	return id.WrapDyn(t.Context(), id.NewDyn(TagKindExpr, id.ID[TagAny](t.ID())))
}

// Brackets returns the bracket token that makes up this tag.
func (t TagExpr) Brackets() token.Token {
	if t.IsZero() {
		return token.Zero
	}
	return id.Wrap(t.Context().Stream(), t.Raw().brackets)
}

// Expr returns the expression this tag evaluates.
//
// May be zero, if the user forgot it.
func (d TagExpr) Expr() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().expr)
}

// Span implements [source.Spanner].
func (t TagExpr) Span() source.Span {
	return t.Brackets().Span()
}
