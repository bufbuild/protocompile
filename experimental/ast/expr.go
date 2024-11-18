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
	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	ExprKindLiteral ExprKind = iota + 1
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
	withContext
	raw rawExpr
}

// rawExpr is the raw representation of an expression.
//
// Similar to rawType (see type.go), this makes use of the fact that for rawPath,
// if the first element is negative, the other must be zero. See also rawType.With.
type rawExpr rawPath

// Kind returns the kind of expression this is. This is suitable for use
// in a switch statement.
func (e ExprAny) Kind() ExprKind {
	if e.raw[0] < 0 && e.raw[1] != 0 {
		return ExprKind(^e.raw[0])
	}
	return ExprKindPath
}

// AsLiteral converts a ExprAny into a ExprLiteral, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsLiteral() ExprLiteral {
	if e.Kind() != ExprKindLiteral {
		return ExprLiteral{}
	}

	return ExprLiteral{
		//nolint:unconvert // Conversion to rawToken included for clarity.
		Token: rawToken(e.raw[1]).With(e),
	}
}

// AsPath converts a ExprAny into a ExprPath, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsPath() ExprPath {
	if e.Kind() != ExprKindPath {
		return ExprPath{}
	}

	return ExprPath{
		Path: rawPath(e.raw).With(e),
	}
}

func (e ExprAny) ptr() arena.Untyped {
	return arena.Untyped(e.raw[1])
}

// AsPrefixed converts a ExprAny into a ExprPrefixed, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsPrefixed() ExprPrefixed {
	if e.Kind() != ExprKindPrefixed {
		return ExprPrefixed{}
	}

	ptr := arena.Pointer[rawExprPrefixed](e.ptr())
	return ExprPrefixed{exprImpl[rawExprPrefixed]{
		e.withContext,
		e.Context().exprs.prefixes.Deref(ptr),
		ptr,
		ExprKindPrefixed,
	}}
}

// AsRange converts a ExprAny into a ExprRange, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsRange() ExprRange {
	if e.Kind() != ExprKindRange {
		return ExprRange{}
	}

	ptr := arena.Pointer[rawExprRange](e.ptr())
	return ExprRange{exprImpl[rawExprRange]{
		e.withContext,
		e.Context().exprs.ranges.Deref(ptr),
		ptr,
		ExprKindPrefixed,
	}}
}

// AsArray converts a ExprAny into a ExprArray, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsArray() ExprArray {
	if e.Kind() != ExprKindArray {
		return ExprArray{}
	}

	ptr := arena.Pointer[rawExprArray](e.ptr())
	return ExprArray{exprImpl[rawExprArray]{
		e.withContext,
		e.Context().exprs.arrays.Deref(ptr),
		ptr,
		ExprKindPrefixed,
	}}
}

// AsDict converts a ExprAny into a ExprDict, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsDict() ExprDict {
	if e.Kind() != ExprKindDict {
		return ExprDict{}
	}

	ptr := arena.Pointer[rawExprDict](e.ptr())
	return ExprDict{exprImpl[rawExprDict]{
		e.withContext,
		e.Context().exprs.dicts.Deref(ptr),
		ptr,
		ExprKindPrefixed,
	}}
}

// AsKV converts a ExprAny into a ExprKV, if that is the type
// it contains.
//
// Otherwise, returns nil.
func (e ExprAny) AsKV() ExprField {
	if e.Kind() != ExprKindField {
		return ExprField{}
	}

	ptr := arena.Pointer[rawExprField](e.ptr())
	return ExprField{exprImpl[rawExprField]{
		e.withContext,
		e.Context().exprs.fields.Deref(ptr),
		ptr,
		ExprKindPrefixed,
	}}
}

// Span implements [Spanner].
func (e ExprAny) Span() Span {
	// At most one of the below will produce a non-nil type, and that will be
	// the span selected by JoinSpans. If all of them are nil, this produces
	// the nil span.
	return JoinSpans(
		e.AsLiteral(),
		e.AsPath(),
		e.AsPrefixed(),
		e.AsRange(),
		e.AsArray(),
		e.AsDict(),
		e.AsKV(),
	)
}

// typeImpl is the common implementation of pointer-like Expr* types.
type exprImpl[Raw any] struct {
	// NOTE: These fields are sorted by alignment.
	withContext
	raw  *Raw
	ptr  arena.Pointer[Raw]
	kind ExprKind
}

// AsAny type-erases this expression value.
//
// See [ExprAny] for more information.
func (e exprImpl[Raw]) AsAny() ExprAny {
	if e.Nil() {
		return ExprAny{}
	}
	return ExprAny{
		e.withContext,
		rawExpr{^rawToken(e.kind), rawToken(e.ptr)},
	}
}

func (e rawExpr) With(c Contextual) ExprAny {
	ctx := c.Context()
	if ctx == nil || (e == rawExpr{}) {
		return ExprAny{}
	}

	return ExprAny{withContext{ctx}, e}
}

// exprs is storage for the various kinds of Exprs in a Context.
type exprs struct {
	prefixes arena.Arena[rawExprPrefixed]
	ranges   arena.Arena[rawExprRange]
	arrays   arena.Arena[rawExprArray]
	dicts    arena.Arena[rawExprDict]
	fields   arena.Arena[rawExprField]
}