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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// ExprDict represents a an array of message fields between curly braces.
//
// # Grammar
//
//	ExprDict := `{` fields `}` | `<` fields `>`
//	fields := (Expr (`,` | `;`)?)*
//
// Note that if a non-[ExprField] occurs as a field of a dict, the parser will
// rewrite it into an [ExprField] with a missing key.
type ExprDict struct{ exprImpl[rawExprDict] }

type rawExprDict struct {
	braces token.ID
	fields []withComma[arena.Pointer[rawExprField]]
}

// Braces returns the token tree corresponding to the whole {...}.
//
// May be missing for a synthetic expression.
func (e ExprDict) Braces() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return e.raw.braces.In(e.Context())
}

// Elements returns the sequence of expressions in this array.
func (e ExprDict) Elements() Commas[ExprField] {
	type slice = commas[ExprField, arena.Pointer[rawExprField]]
	if e.IsZero() {
		return slice{}
	}
	return slice{
		ctx: e.Context(),
		SliceInserter: seq.NewSliceInserter(
			&e.raw.fields,
			func(_ int, c withComma[arena.Pointer[rawExprField]]) ExprField {
				return ExprField{exprImpl[rawExprField]{
					e.withContext,
					e.Context().Nodes().exprs.fields.Deref(c.Value),
				}}
			},
			func(_ int, e ExprField) withComma[arena.Pointer[rawExprField]] {
				e.Context().Nodes().panicIfNotOurs(e)
				ptr := e.Context().Nodes().exprs.fields.Compress(e.raw)
				return withComma[arena.Pointer[rawExprField]]{Value: ptr}
			},
		),
	}
}

// Span implements [report.Spanner].
func (e ExprDict) Span() report.Span {
	if e.IsZero() {
		return report.Span{}
	}

	return e.Braces().Span()
}

// ExprField is a key-value pair within an [ExprDict].
//
// It implements [ExprAny], since it can appear inside of e.g. an array if the
// user incorrectly writes [foo: bar].
//
// # Grammar
//
//	ExprField := ExprFieldWithColon | Expr (ExprDict | ExprArray)
//	ExprFieldWithColon := Expr (`:` | `=`) Expr
//
// Note: ExprFieldWithColon appears in ExprJuxta, the expression production that
// is unambiguous when expressions are juxtaposed with each other.
type ExprField struct{ exprImpl[rawExprField] }

type rawExprField struct {
	key, value rawExpr
	colon      token.ID
}

// ExprFieldArgs is arguments for [Context.NewExprKV].
type ExprFieldArgs struct {
	Key   ExprAny
	Colon token.Token
	Value ExprAny
}

// Key returns the key for this field.
//
// May be zero if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprField) Key() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return newExprAny(e.Context(), e.raw.key)
}

// SetKey sets the key for this field.
//
// If passed zero, this clears the key.
func (e ExprField) SetKey(expr ExprAny) {
	e.raw.key = expr.raw
}

// Colon returns the colon between Key() and Value().
//
// May be zero: it is valid for a field name to be immediately followed by its value and be syntactically
// valid (unlike most "optional" punctuation, this is permitted by Protobuf, not just our permissive AST).
func (e ExprField) Colon() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return e.raw.colon.In(e.Context())
}

// Value returns the value for this field.
func (e ExprField) Value() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return newExprAny(e.Context(), e.raw.value)
}

// SetValue sets the value for this field.
//
// If passed zero, this clears the expression.
func (e ExprField) SetValue(expr ExprAny) {
	e.raw.value = expr.raw
}

// Span implements [report.Spanner].
func (e ExprField) Span() report.Span {
	if e.IsZero() {
		return report.Span{}
	}

	return report.Join(e.Key(), e.Colon(), e.Value())
}
