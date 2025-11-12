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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ExprArray represents an array of expressions between square brackets.
//
// # Grammar
//
//	ExprArray := `[` (ExprJuxta `,`?)*`]`
type ExprArray id.Node[ExprArray, *File, *rawExprArray]

type rawExprArray struct {
	brackets token.ID
	args     []withComma[id.Dyn[ExprAny, ExprKind]]
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprArray) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindArray, id.ID[ExprAny](e.ID())))
}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e ExprArray) Brackets() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return id.Wrap(e.Context().Stream(), e.Raw().brackets)
}

// Elements returns the sequence of expressions in this array.
func (e ExprArray) Elements() Commas[ExprAny] {
	type slice = commas[ExprAny, id.Dyn[ExprAny, ExprKind]]
	if e.IsZero() {
		return slice{}
	}
	return slice{
		file: e.Context(),
		SliceInserter: seq.NewSliceInserter(
			&e.Raw().args,
			func(_ int, c withComma[id.Dyn[ExprAny, ExprKind]]) ExprAny {
				return id.WrapDyn(e.Context(), c.Value)
			},
			func(_ int, e ExprAny) withComma[id.Dyn[ExprAny, ExprKind]] {
				e.Context().Nodes().panicIfNotOurs(e)
				return withComma[id.Dyn[ExprAny, ExprKind]]{Value: e.ID()}
			},
		),
	}
}

// Span implements [source.Spanner].
func (e ExprArray) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}

	return e.Brackets().Span()
}
