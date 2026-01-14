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

// ExprDict represents a an array of message fields between curly braces.
//
// # Grammar
//
//	ExprDict := `{` fields `}` | `<` fields `>`
//	fields := (Expr (`,` | `;`)?)*
//
// Note that if a non-[ExprField] occurs as a field of a dict, the parser will
// rewrite it into an [ExprField] with a missing key.
type ExprDict id.Node[ExprDict, *File, *rawExprDict]

type rawExprDict struct {
	braces token.ID
	fields []withComma[id.ID[ExprField]]
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprDict) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindDict, id.ID[ExprAny](e.ID())))
}

// Braces returns the token tree corresponding to the whole {...}.
//
// May be missing for a synthetic expression.
func (e ExprDict) Braces() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return id.Wrap(e.Context().Stream(), e.Raw().braces)
}

// Elements returns the sequence of expressions in this array.
func (e ExprDict) Elements() Commas[ExprField] {
	type slice = commas[ExprField, id.ID[ExprField]]
	if e.IsZero() {
		return slice{}
	}
	return slice{
		file: e.Context(),
		SliceInserter: seq.NewSliceInserter(
			&e.Raw().fields,
			func(_ int, c withComma[id.ID[ExprField]]) ExprField {
				return id.Wrap(e.Context(), c.Value)
			},
			func(_ int, e ExprField) withComma[id.ID[ExprField]] {
				e.Context().Nodes().panicIfNotOurs(e)
				return withComma[id.ID[ExprField]]{Value: e.ID()}
			},
		),
	}
}

// Span implements [source.Spanner].
func (e ExprDict) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
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
type ExprField id.Node[ExprField, *File, *rawExprField]

type rawExprField struct {
	key, value id.Dyn[ExprAny, ExprKind]
	colon      token.ID
}

// ExprFieldArgs is arguments for [Context.NewExprKV].
type ExprFieldArgs struct {
	Key   ExprAny
	Colon token.Token
	Value ExprAny
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprField) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindField, id.ID[ExprAny](e.ID())))
}

// Key returns the key for this field.
//
// May be zero if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprField) Key() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), e.Raw().key)
}

// SetKey sets the key for this field.
//
// If passed zero, this clears the key.
func (e ExprField) SetKey(expr ExprAny) {
	e.Raw().key = expr.ID()
}

// Colon returns the colon between Key() and Value().
//
// May be zero: it is valid for a field name to be immediately followed by its value and be syntactically
// valid (unlike most "optional" punctuation, this is permitted by Protobuf, not just our permissive AST).
func (e ExprField) Colon() token.Token {
	if e.IsZero() {
		return token.Zero
	}

	return id.Wrap(e.Context().Stream(), e.Raw().colon)
}

// Value returns the value for this field.
func (e ExprField) Value() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), e.Raw().value)
}

// SetValue sets the value for this field.
//
// If passed zero, this clears the expression.
func (e ExprField) SetValue(expr ExprAny) {
	e.Raw().value = expr.ID()
}

// Span implements [source.Spanner].
func (e ExprField) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}

	return source.Join(e.Key(), e.Colon(), e.Value())
}
