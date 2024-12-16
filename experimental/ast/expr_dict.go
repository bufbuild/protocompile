// Copyright 2020-2024 Buf Technologies, Inc.
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
	"slices"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// ExprDict represents a an array of message fields between curly braces.
//
// ExprArray implements [Commas].
//
// # Grammar
//
//	ExprDict := `{` fields `}` | `<` fields `>`
//	fields := (Expr [,;]?)* Expr
//
// Note that if a non-[ExprField] occurs as a field of a dict, the parser will
// rewrite it into an [ExprField] with a missing key.
type ExprDict struct{ exprImpl[rawExprDict] }

type rawExprDict struct {
	braces token.ID
	fields []withComma[arena.Pointer[rawExprField]]
}

var _ Commas[ExprField] = ExprDict{}

// Braces returns the token tree corresponding to the whole {...}.
//
// May be missing for a synthetic expression.
func (e ExprDict) Braces() token.Token {
	return e.raw.braces.In(e.Context())
}

// Len implements [Slice].
func (e ExprDict) Len() int {
	return len(e.raw.fields)
}

// At implements [Slice].
func (e ExprDict) At(n int) ExprField {
	ptr := e.raw.fields[n].Value
	return ExprField{exprImpl[rawExprField]{
		e.withContext,
		e.Context().Nodes().exprs.fields.Deref(ptr),
	}}
}

// Iter implements [Slice].
func (e ExprDict) Iter(yield func(int, ExprField) bool) {
	for i, f := range e.raw.fields {
		e := ExprField{exprImpl[rawExprField]{
			e.withContext,
			e.Context().Nodes().exprs.fields.Deref(f.Value),
		}}
		if !yield(i, e) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprDict) Append(expr ExprField) {
	e.InsertComma(e.Len(), expr, token.Nil)
}

// Insert implements [Inserter].
func (e ExprDict) Insert(n int, expr ExprField) {
	e.InsertComma(n, expr, token.Nil)
}

// Delete implements [Inserter].
func (e ExprDict) Delete(n int) {
	e.raw.fields = slices.Delete(e.raw.fields, n, n+1)
}

// Comma implements [Commas].
func (e ExprDict) Comma(n int) token.Token {
	return e.raw.fields[n].Comma.In(e.Context())
}

// AppendComma implements [Commas].
func (e ExprDict) AppendComma(expr ExprField, comma token.Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprDict) InsertComma(n int, expr ExprField, comma token.Token) {
	e.Context().Nodes().panicIfNotOurs(expr, comma)
	if expr.Nil() {
		panic("protocompile/ast: cannot append nil ExprField to ExprMessage")
	}

	ptr := e.Context().Nodes().exprs.fields.Compress(expr.raw)
	e.raw.fields = slices.Insert(e.raw.fields, n, withComma[arena.Pointer[rawExprField]]{ptr, comma.ID()})
}

// AsMessage implements [ExprAny].
func (e ExprDict) AsMessage() Commas[ExprField] {
	return e
}

// Span implements [report.Spanner].
func (e ExprDict) Span() report.Span {
	return e.Braces().Span()
}

// ExprField is a key-value pair within an [ExprDict].
//
// It implements [ExprAny], since it can appear inside of e.g. an array if the
// user incorrectly writes [foo: bar].
//
// # Grammar
//
//	ExprField := Expr [:=]
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
// May be nil if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprField) Key() ExprAny {
	return newExprAny(e.Context(), e.raw.key)
}

// SetKey sets the key for this field.
//
// If passed nil, this clears the key.
func (e ExprField) SetKey(expr ExprAny) {
	e.raw.key = expr.raw
}

// Colon returns the colon between Key() and Value().
//
// May be nil: it is valid for a field name to be immediately followed by its value and be syntactically
// valid (unlike most "optional" punctuation, this is permitted by Protobuf, not just our permissive AST).
func (e ExprField) Colon() token.Token {
	return e.raw.colon.In(e.Context())
}

// Value returns the value for this field.
func (e ExprField) Value() ExprAny {
	return newExprAny(e.Context(), e.raw.value)
}

// SetValue sets the value for this field.
//
// If passed nil, this clears the expression.
func (e ExprField) SetValue(expr ExprAny) {
	e.raw.value = expr.raw
}

// Span implements [report.Spanner].
func (e ExprField) Span() report.Span {
	return report.Join(e.Key(), e.Colon(), e.Value())
}
