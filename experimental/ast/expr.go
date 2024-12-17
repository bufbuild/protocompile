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
	"reflect"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

const (
	ExprKindNil ExprKind = iota
	ExprKindLiteral
	ExprKindPrefixed
	ExprKindPath
	ExprKindRange
	ExprKindArray
	ExprKindDict
	ExprKindField
)

// ExprKind is a kind of expression. There is one value of ExprKind for each
// Expr* type in this package.
type ExprKind int8

// ExprAny is any ExprAny* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a ExprAny*
// type, such as [ExprPath.AsAny]. It can be type-asserted back to any of
// the concrete ExprAny* types using its own As* methods.
//
// This type is used in lieu of a putative ExprAny interface type to avoid heap
// allocations in functions that would return one of many different ExprAny*
// types.
type ExprAny struct {
	withContext // Must be nil if raw is nil.

	raw rawExpr
}

// rawExpr is the raw representation of an expression.
//
// Similar to rawType (see type.go), this makes use of the fact that for rawPath,
// if the first element is negative, the other must be zero. See also rawType.With.
type rawExpr = pathLike[ExprKind]

func newExprAny(c Context, e rawExpr) ExprAny {
	if c == nil || (e == rawExpr{}) {
		return ExprAny{}
	}

	return ExprAny{internal.NewWith(c), e}
}

// Kind returns the kind of expression this is. This is suitable for use
// in a switch statement.
func (e ExprAny) Kind() ExprKind {
	if e.Nil() {
		return ExprKindNil
	}

	if kind, ok := e.raw.kind(); ok {
		return kind
	}
	return ExprKindPath
}

// AsLiteral converts a ExprAny into a ExprLiteral, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsLiteral() ExprLiteral {
	tok := unwrapPathLike[token.ID](ExprKindLiteral, e.raw)
	if tok.Nil() {
		return ExprLiteral{}
	}

	return ExprLiteral{tok.In(e.Context())}
}

// AsPath converts a ExprAny into a ExprPath, if that is the type
// it contains.q
//
// Otherwise, returns nil.
func (e ExprAny) AsPath() ExprPath {
	path, _ := e.raw.path(e.Context())
	// Don't need to check ok; path() returns nil on failure.
	return ExprPath{path}
}

// AsPrefixed converts a ExprAny into a ExprPrefixed, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsPrefixed() ExprPrefixed {
	ptr := unwrapPathLike[arena.Pointer[rawExprPrefixed]](ExprKindPrefixed, e.raw)
	if ptr.Nil() {
		return ExprPrefixed{}
	}

	return ExprPrefixed{exprImpl[rawExprPrefixed]{
		e.withContext,
		e.Context().Nodes().exprs.prefixes.Deref(ptr),
	}}
}

// AsRange converts a ExprAny into a ExprRange, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsRange() ExprRange {
	ptr := unwrapPathLike[arena.Pointer[rawExprRange]](ExprKindRange, e.raw)
	if ptr.Nil() {
		return ExprRange{}
	}

	return ExprRange{exprImpl[rawExprRange]{
		e.withContext,
		e.Context().Nodes().exprs.ranges.Deref(ptr),
	}}
}

// AsArray converts a ExprAny into a ExprArray, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsArray() ExprArray {
	ptr := unwrapPathLike[arena.Pointer[rawExprArray]](ExprKindArray, e.raw)
	if ptr.Nil() {
		return ExprArray{}
	}

	return ExprArray{exprImpl[rawExprArray]{
		e.withContext,
		e.Context().Nodes().exprs.arrays.Deref(ptr),
	}}
}

// AsDict converts a ExprAny into a ExprDict, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsDict() ExprDict {
	ptr := unwrapPathLike[arena.Pointer[rawExprDict]](ExprKindDict, e.raw)
	if ptr.Nil() {
		return ExprDict{}
	}

	return ExprDict{exprImpl[rawExprDict]{
		e.withContext,
		e.Context().Nodes().exprs.dicts.Deref(ptr),
	}}
}

// AsField converts a ExprAny into a ExprKV, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsField() ExprField {
	ptr := unwrapPathLike[arena.Pointer[rawExprField]](ExprKindField, e.raw)
	if ptr.Nil() {
		return ExprField{}
	}

	return ExprField{exprImpl[rawExprField]{
		e.withContext,
		e.Context().Nodes().exprs.fields.Deref(ptr),
	}}
}

// Span implements [report.Spanner].
func (e ExprAny) Span() report.Span {
	// At most one of the below will produce a non-nil type, and that will be
	// the span selected by report.Join. If all of them are nil, this produces
	// the nil span.
	return report.Join(
		e.AsLiteral(),
		e.AsPath(),
		e.AsPrefixed(),
		e.AsRange(),
		e.AsArray(),
		e.AsDict(),
		e.AsField(),
	)
}

// typeImpl is the common implementation of pointer-like Expr* types.
type exprImpl[Raw any] struct {
	// NOTE: These fields are sorted by alignment.
	withContext
	raw *Raw
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e exprImpl[Raw]) AsAny() ExprAny {
	if e.Nil() {
		return ExprAny{}
	}

	kind, arena := exprArena[Raw](&e.Context().Nodes().exprs)
	return newExprAny(
		e.Context(),
		wrapPathLike(kind, arena.Compress(e.raw)),
	)
}

// Trace returns a stack trace for the site at which e was constructed using
// a [Nodes].
//
// Returns "" if a trace was not recorded. See Nodes.EnableTracing.
func (e exprImpl[Raw]) Trace() string {
	return e.Context().Nodes().traces[unsafex.Addr(e.raw)]
}

// exprs is storage for the various kinds of Exprs in a Context.
type exprs struct {
	prefixes arena.Arena[rawExprPrefixed]
	ranges   arena.Arena[rawExprRange]
	arrays   arena.Arena[rawExprArray]
	dicts    arena.Arena[rawExprDict]
	fields   arena.Arena[rawExprField]
}

func exprArena[Raw any](exprs *exprs) (ExprKind, *arena.Arena[Raw]) {
	var (
		kind ExprKind
		raw  Raw
		// Needs to be an any because Go doesn't know that only the case below
		// with the correct type for arena_ (if it were *arena.Arena[Raw]) will
		// be evaluated.
		arena_ any //nolint:revive // Named arena_ to avoid clashing with package arena.
	)

	switch any(raw).(type) {
	case rawExprPrefixed:
		kind = ExprKindPrefixed
		arena_ = &exprs.prefixes
	case rawExprRange:
		kind = ExprKindRange
		arena_ = &exprs.ranges
	case rawExprArray:
		kind = ExprKindArray
		arena_ = &exprs.arrays
	case rawExprDict:
		kind = ExprKindDict
		arena_ = &exprs.dicts
	case rawExprField:
		kind = ExprKindField
		arena_ = &exprs.fields
	default:
		panic("unknown expr type " + reflect.TypeOf(raw).Name())
	}

	return kind, arena_.(*arena.Arena[Raw]) //nolint:errcheck
}
