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

//nolint:dupword // Disable for whole file, because the error is in a comment.
package ast

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// ExprAny is any ExprAny* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a ExprAny*
// type, such as [ExprPath.AsAny]. It can be type-asserted back to any of
// the concrete ExprAny* types using its own As* methods.
//
// This type is used in lieu of a putative ExprAny interface type to avoid heap
// allocations in functions that would return one of many different ExprAny*
// types.
//
// # Grammar
//
// In addition to the Expr type, we define some exported productions for
// handling operator precedence.
//
//	Expr      := ExprField | ExprOp
//	ExprJuxta := ExprFieldWithColon | ExprOp
//	ExprOp    := ExprRange | ExprPrefix | ExprSolo
//	ExprSolo  := ExprLiteral | ExprPath | ExprArray | ExprDict
//
// Note: ExprJuxta is the expression production that is unambiguous when
// expressions are juxtaposed with each other; i.e., ExprJuxta* does not make
// e.g. "foo {}" ambiguous between an [ExprField] or an [ExprPath] followed by
// an [ExprDict].
type ExprAny id.DynNode[ExprAny, ExprKind, *File]

// AsError converts a ExprAny into a ExprError, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsError() ExprError {
	if e.Kind() != ExprKindError {
		return ExprError{}
	}
	return id.Wrap(e.Context(), id.ID[ExprError](e.ID().Value()))
}

// AsLiteral converts a ExprAny into a ExprLiteral, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsLiteral() ExprLiteral {
	if e.Kind() != ExprKindLiteral {
		return ExprLiteral{}
	}
	return ExprLiteral{
		File:  e.Context(),
		Token: id.Wrap(e.Context().Stream(), id.ID[token.Token](e.ID().Value())),
	}
}

// AsPath converts a ExprAny into a ExprPath, if that is the type
// it contains.q
//
// Otherwise, returns zero.
func (e ExprAny) AsPath() ExprPath {
	if e.Kind() != ExprKindPath {
		return ExprPath{}
	}

	start, end := e.ID().Raw()
	return ExprPath{Path: PathID{start: token.ID(start), end: token.ID(end)}.In(e.Context())}
}

// AsPrefixed converts a ExprAny into a ExprPrefixed, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsPrefixed() ExprPrefixed {
	if e.Kind() != ExprKindPrefixed {
		return ExprPrefixed{}
	}
	return id.Wrap(e.Context(), id.ID[ExprPrefixed](e.ID().Value()))
}

// AsRange converts a ExprAny into a ExprRange, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsRange() ExprRange {
	if e.Kind() != ExprKindRange {
		return ExprRange{}
	}
	return id.Wrap(e.Context(), id.ID[ExprRange](e.ID().Value()))
}

// AsArray converts a ExprAny into a ExprArray, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsArray() ExprArray {
	if e.Kind() != ExprKindArray {
		return ExprArray{}
	}
	return id.Wrap(e.Context(), id.ID[ExprArray](e.ID().Value()))
}

// AsDict converts a ExprAny into a ExprDict, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsDict() ExprDict {
	if e.Kind() != ExprKindDict {
		return ExprDict{}
	}
	return id.Wrap(e.Context(), id.ID[ExprDict](e.ID().Value()))
}

// AsField converts a ExprAny into a ExprKV, if that is the type
// it contains.
//
// Otherwise, returns zero.
func (e ExprAny) AsField() ExprField {
	if e.Kind() != ExprKindField {
		return ExprField{}
	}
	return id.Wrap(e.Context(), id.ID[ExprField](e.ID().Value()))
}

// Span implements [source.Spanner].
func (e ExprAny) Span() source.Span {
	// At most one of the below will produce a non-nil type, and that will be
	// the span selected by source.Join. If all of them are nil, this produces
	// the nil span.
	return source.Join(
		e.AsLiteral(),
		e.AsPath(),
		e.AsPrefixed(),
		e.AsRange(),
		e.AsArray(),
		e.AsDict(),
		e.AsField(),
	)
}

// ExprError represents an unrecoverable parsing error in an expression context.
type ExprError id.Node[ExprError, *File, *rawExprError]

type rawExprError source.Span

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e ExprError) AsAny() ExprAny {
	if e.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(e.Context(), id.NewDyn(ExprKindError, id.ID[ExprAny](e.ID())))
}

// Span implements [source.Spanner].
func (e ExprError) Span() source.Span {
	if e.IsZero() {
		return source.Span{}
	}

	return source.Span(*e.Raw())
}

func (ExprKind) DecodeDynID(lo, hi int32) ExprKind {
	switch {
	case lo == 0:
		return ExprKindInvalid
	case lo < 0 && hi > 0:
		return ExprKind(^lo)
	default:
		return ExprKindPath
	}
}

func (k ExprKind) EncodeDynID(value int32) (int32, int32, bool) {
	return ^int32(k), value, true
}

// exprs is storage for the various kinds of Exprs in a Context.
type exprs struct {
	errors   arena.Arena[rawExprError]
	prefixes arena.Arena[rawExprPrefixed]
	ranges   arena.Arena[rawExprRange]
	arrays   arena.Arena[rawExprArray]
	dicts    arena.Arena[rawExprDict]
	fields   arena.Arena[rawExprField]
}
