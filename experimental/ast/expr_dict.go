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

	"github.com/bufbuild/protocompile/internal/arena"
)

// ExprDict represents a an array of message fields between curly braces.
type ExprDict struct{ exprImpl[rawExprDict] }

type rawExprDict struct {
	braces rawToken
	fields []withComma[arena.Pointer[rawExprField]]
}

var _ Commas[ExprField] = ExprDict{}

// Braces returns the token tree corresponding to the whole {...}.
//
// May be missing for a synthetic expression.
func (e ExprDict) Braces() Token {
	return e.raw.braces.With(e)
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
		e.Context().exprs.fields.Deref(ptr),
		ptr,
		ExprKindField,
	}}
}

// Iter implements [Slice].
func (e ExprDict) Iter(yield func(int, ExprField) bool) {
	for i, f := range e.raw.fields {
		e := ExprField{exprImpl[rawExprField]{
			e.withContext,
			e.Context().exprs.fields.Deref(f.Value),
			f.Value,
			ExprKindField,
		}}
		if !yield(i, e) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprDict) Append(expr ExprField) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter].
func (e ExprDict) Insert(n int, expr ExprField) {
	e.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter].
func (e ExprDict) Delete(n int) {
	e.raw.fields = slices.Delete(e.raw.fields, n, n+1)
}

// Comma implements [Commas].
func (e ExprDict) Comma(n int) Token {
	return e.raw.fields[n].Comma.With(e)
}

// AppendComma implements [Commas].
func (e ExprDict) AppendComma(expr ExprField, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprDict) InsertComma(n int, expr ExprField, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)
	if expr.Nil() {
		panic("protocompile/ast: cannot append nil ExprField to ExprMessage")
	}

	e.raw.fields = slices.Insert(e.raw.fields, n, withComma[arena.Pointer[rawExprField]]{expr.ptr, comma.raw})
}

// AsMessage implements [ExprAny].
func (e ExprDict) AsMessage() Commas[ExprField] {
	return e
}

// Span implements [Spanner].
func (e ExprDict) Span() Span {
	return e.Braces().Span()
}

// ExprField is a key-value pair within an [ExprDict].
//
// It implements [ExprAny], since it can appear inside of e.g. an array if the user incorrectly writes [foo: bar].
type ExprField struct{ exprImpl[rawExprField] }

type rawExprField struct {
	key, value rawExpr
	colon      rawToken
}

// ExprKVArgs is arguments for [Context.NewExprKV].
type ExprKVArgs struct {
	Key   ExprAny
	Colon Token
	Value ExprAny
}

// Key returns the key for this field.
//
// May be nil if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprField) Key() ExprAny {
	return e.raw.key.With(e)
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
func (e ExprField) Colon() Token {
	return e.raw.colon.With(e)
}

// Value returns the value for this field.
func (e ExprField) Value() ExprAny {
	return e.raw.value.With(e)
}

// SetValue sets the value for this field.
//
// If passed nil, this clears the expression.
func (e ExprField) SetValue(expr ExprAny) {
	e.raw.value = expr.raw
}

// Span implements [Spanner].
func (e ExprField) Span() Span {
	return JoinSpans(e.Key(), e.Colon(), e.Value())
}
