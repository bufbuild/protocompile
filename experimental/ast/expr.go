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
	"math"
	"slices"

	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	ExprPrefixUnknown ExprPrefix = iota
	ExprPrefixMinus
)

// TypePrefix is a prefix for an expression, such as a minus sign.
type ExprPrefix int8

// ExprPrefixByName looks up a prefix kind by name.
//
// If name is not a known prefix, returns [ExprPrefixUnknown].
func ExprPrefixByName(name string) ExprPrefix {
	switch name {
	case "-":
		return ExprPrefixMinus
	default:
		return ExprPrefixUnknown
	}
}

// Expr is an expression, primarily occurring on the right hand side of an =.
//
// Expr provides methods for interpreting it as various Go types, as a shorthand
// for introspecting the concrete type of the expression. These methods return
// (T, bool), returning false if the expression cannot be interpreted as that
// type. The [Commas]-returning methods return nil instead.
// TODO: Return a diagnostic instead.
//
// This is implemented by types in this package of the form Expr*.
type Expr interface {
	Spanner

	AsBool() (bool, bool)
	AsInt32() (int32, bool)
	AsInt64() (int64, bool)
	AsUInt32() (uint32, bool)
	AsUInt64() (uint64, bool)
	AsFloat32() (float32, bool)
	AsFloat64() (float64, bool)
	AsString() (string, bool)
	AsArray() Commas[Expr]
	AsMessage() Commas[ExprKV]

	exprRaw() (exprKind, arena.Untyped)
}

// exprs is storage for the various kinds of Exprs in a Context.
type exprs struct {
	prefixes arena.Arena[rawExprPrefixed]
	ranges   arena.Arena[rawExprRange]
	arrays   arena.Arena[rawExprArray]
	dicts    arena.Arena[rawExprDict]
	kvs      arena.Arena[rawExprKV]
}

// ExprLiteral is an expression corresponding to a string or number literal.
type ExprLiteral struct {
	baseExpr

	// The token backing this expression. Must be [TokenString] or [TokenNumber].
	Token
}

var _ Expr = ExprLiteral{}

// AsInt32 implements [Expr].
func (e ExprLiteral) AsInt32() (int32, bool) {
	n, ok := e.Token.AsInt()
	return int32(n), ok && n <= uint64(math.MaxInt32)
}

// AsInt64 implements [Expr].
func (e ExprLiteral) AsInt64() (int64, bool) {
	n, ok := e.Token.AsInt()
	return int64(n), ok && n <= uint64(math.MaxInt64)
}

// AsUInt32 implements [Expr].
func (e ExprLiteral) AsUInt32() (uint32, bool) {
	n, ok := e.Token.AsInt()
	return uint32(n), ok && n <= uint64(math.MaxUint32)
}

// AsUInt64 implements [Expr].
func (e ExprLiteral) AsUInt64() (uint64, bool) {
	return e.Token.AsInt()
}

// AsFloat32 implements [Expr].
func (e ExprLiteral) AsFloat32() (float32, bool) {
	n, ok := e.Token.AsFloat()
	return float32(n), ok // Loss of precision is intentional.
}

// AsFloat64 implements [Expr].
func (e ExprLiteral) AsFloat64() (float64, bool) {
	return e.Token.AsFloat()
}

// AsString implements [Expr].
func (e ExprLiteral) AsString() (string, bool) {
	return e.Token.AsString()
}

func (e ExprLiteral) exprRaw() (exprKind, arena.Untyped) {
	return exprLiteral,
		// This is technically not a real pointer; we're instead smuggling a
		// token ID through the pointer value.
		arena.Untyped(e.Token.raw)
}

// ExprPath is a Protobuf path in expression position.
type ExprPath struct {
	baseExpr

	// The path backing this expression.
	Path
}

var _ Expr = ExprPath{}

// AsBool implements [Expr].
func (e ExprPath) AsBool() (bool, bool) {
	switch e.AsIdent().Text() {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// AsFloat32 implements [Expr].
func (e ExprPath) AsFloat32() (float32, bool) {
	n, ok := e.AsFloat64()
	return float32(n), ok
}

// AsFloat64 implements [Expr].
func (e ExprPath) AsFloat64() (float64, bool) {
	// TODO: Message literals also allow for an "infinity" token. Whether this
	// is allowed is context-dependent, so we will probably need to replace the
	// As* methods with something else.
	switch e.AsIdent().Text() {
	case "inf":
		return math.Inf(1), true
	case "nan":
		return math.NaN(), true
	default:
		return 0, false
	}
}

func (ExprPath) exprRaw() (exprKind, arena.Untyped) {
	return exprPath, arena.Nil()
}

// ExprPrefixed is an expression prefixed with an operator.
type ExprPrefixed struct {
	baseExpr
	withContext

	ptr arena.Pointer[rawExprPrefixed]
	raw *rawExprPrefixed
}

type rawExprPrefixed struct {
	prefix rawToken
	expr   rawExpr
}

// ExprPrefixedArgs is arguments for [Context.NewExprPrefixed].
type ExprPrefixedArgs struct {
	Prefix Token
	Expr   Expr
}

var _ Expr = ExprPrefixed{}

// Prefix returns this expression's prefix.
func (e ExprPrefixed) Prefix() ExprPrefix {
	return ExprPrefixByName(e.PrefixToken().Text())
}

// Prefix returns the token representing this expression's prefix.
func (e ExprPrefixed) PrefixToken() Token {
	return e.raw.prefix.With(e)
}

// Expr returns the expression the prefix is applied to.
func (e ExprPrefixed) Expr() Expr {
	return e.raw.expr.With(e)
}

// SetExpr sets the expression that the prefix is applied to.
//
// If passed nil, this clears the expression.
func (e ExprPrefixed) SetExpr(expr Expr) {
	e.raw.expr = toRawExpr(expr)
}

// Span implements [Spanner].
func (e ExprPrefixed) Span() Span {
	return JoinSpans(e.PrefixToken(), e.Expr())
}

// AsInt32 implements [Expr].
func (e ExprPrefixed) AsInt32() (int32, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		n, ok := e.AsInt64()
		if !ok || n < int64(math.MinInt32) || n > int64(math.MaxInt32) {
			return 0, false
		}

		return int32(n), true
	default:
		return 0, false
	}
}

// AsInt64 implements [Expr].
func (e ExprPrefixed) AsInt64() (int64, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		switch n, ok := e.Expr().AsUInt64(); {
		case !ok:
			return 0, false
		case n <= uint64(math.MaxInt64):
			return -int64(n), true
		case n == uint64(math.MaxInt64)+1:
			// Need to handle the funny case where someone wrote -9223372036854775808, since
			// 9223372036854775808 is not representable as an int64.
			return math.MinInt64, true
		default:
			return 0, false
		}
	default:
		return 0, false
	}
}

// AsUInt32 implements [Expr].
func (e ExprPrefixed) AsUInt32() (uint32, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		// NOTE: - is not treated as two's complement here; we only allow -0
		n, ok := e.Expr().AsUInt32()
		return 0, ok && n == 0
	default:
		return 0, false
	}
}

// AsUInt64 implements [Expr].
func (e ExprPrefixed) AsUInt64() (uint64, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		// NOTE: - is not treated as two's complement here; we only allow -0
		n, ok := e.Expr().AsUInt64()
		return 0, ok && n == 0
	default:
		return 0, false
	}
}

// AsFloat32 implements [Expr].
func (e ExprPrefixed) AsFloat32() (float32, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		n, ok := e.Expr().AsFloat32()
		return -n, ok
	default:
		return 0, false
	}
}

// AsFloat64 implements [Expr].
func (e ExprPrefixed) AsFloat64() (float64, bool) {
	switch e.Prefix() {
	case ExprPrefixMinus:
		n, ok := e.Expr().AsFloat64()
		return -n, ok
	default:
		return 0, false
	}
}

func (e ExprPrefixed) exprRaw() (exprKind, arena.Untyped) {
	return exprPrefixed, e.ptr.Untyped()
}

// ExprRange represents a range of values, such as 1 to 4 or 5 to max.
//
// Note that max is not special syntax; it will appear as an [ExprPath] with the name "max".
type ExprRange struct {
	baseExpr
	withContext

	ptr arena.Pointer[rawExprRange]
	raw *rawExprRange
}

type rawExprRange struct {
	start, end rawExpr
	to         rawToken
}

// ExprRangeArgs is arguments for [Context.NewExprRange].
type ExprRangeArgs struct {
	Start Expr
	To    Token
	End   Expr
}

var _ Expr = ExprRange{}

// Bounds returns this range's bounds. These are inclusive bounds.
func (e ExprRange) Bounds() (start, end Expr) {
	return e.raw.start.With(e), e.raw.end.With(e)
}

// SetBounds set the expressions for this range's bounds.
//
// Clears the respective expressions when passed a nil expression.
func (e ExprRange) SetBounds(start, end Expr) {
	e.raw.start = toRawExpr(start)
	e.raw.end = toRawExpr(end)
}

// Keyword returns the "to" keyword for this range.
func (e ExprRange) Keyword() Token {
	return e.raw.to.With(e)
}

// Span implements [Spanner].
func (e ExprRange) Span() Span {
	lo, hi := e.Bounds()
	return JoinSpans(lo, e.Keyword(), hi)
}

func (e ExprRange) exprRaw() (exprKind, arena.Untyped) {
	return exprRange, e.ptr.Untyped()
}

// ExprArray represents an array of expressions between square brackets.
//
// ExprArray implements [Commas[Expr]].
type ExprArray struct {
	baseExpr
	withContext

	ptr arena.Pointer[rawExprArray]
	raw *rawExprArray
}

type rawExprArray struct {
	brackets rawToken
	args     []withComma[rawExpr]
}

var (
	_ Expr         = ExprArray{}
	_ Commas[Expr] = ExprArray{}
)

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e ExprArray) Brackets() Token {
	return e.raw.brackets.With(e)
}

// Len implements [Slice].
func (e ExprArray) Len() int {
	return len(e.raw.args)
}

// At implements [Slice].
func (e ExprArray) At(n int) Expr {
	return e.raw.args[n].Value.With(e)
}

// Iter implements [Slice].
func (e ExprArray) Iter(yield func(int, Expr) bool) {
	for i, arg := range e.raw.args {
		if !yield(i, arg.Value.With(e)) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprArray) Append(expr Expr) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter].
func (e ExprArray) Insert(n int, expr Expr) {
	e.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter].
func (e ExprArray) Delete(n int) {
	e.raw.args = slices.Delete(e.raw.args, n, n+1)
}

// Comma implements [Commas].
func (e ExprArray) Comma(n int) Token {
	return e.raw.args[n].Comma.With(e)
}

// AppendComma implements [Commas].
func (e ExprArray) AppendComma(expr Expr, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprArray) InsertComma(n int, expr Expr, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)

	e.raw.args = slices.Insert(e.raw.args, n, withComma[rawExpr]{toRawExpr(expr), comma.raw})
}

// AsArray implements [Expr].
func (e ExprArray) AsArray() Commas[Expr] {
	return e
}

// Span implements [Spanner].
func (e ExprArray) Span() Span {
	return e.Brackets().Span()
}

func (e ExprArray) exprRaw() (exprKind, arena.Untyped) {
	return exprArray, e.ptr.Untyped()
}

// ExprDict represents a an array of message fields between curly braces.
type ExprDict struct {
	baseExpr
	withContext

	ptr arena.Pointer[rawExprDict]
	raw *rawExprDict
}

type rawExprDict struct {
	braces rawToken
	fields []withComma[arena.Pointer[rawExprKV]]
}

var (
	_ Expr           = ExprDict{}
	_ Commas[ExprKV] = ExprDict{}
)

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
func (e ExprDict) At(n int) ExprKV {
	ptr := e.raw.fields[n].Value
	return ExprKV{
		baseExpr{},
		e.withContext,
		ptr,
		e.Context().exprs.kvs.Deref(ptr),
	}
}

// Iter implements [Slice].
func (e ExprDict) Iter(yield func(int, ExprKV) bool) {
	for i, f := range e.raw.fields {
		e := ExprKV{
			baseExpr{},
			e.withContext,
			f.Value,
			e.Context().exprs.kvs.Deref(f.Value),
		}
		if !yield(i, e) {
			break
		}
	}
}

// Append implements [Inserter].
func (e ExprDict) Append(expr ExprKV) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter].
func (e ExprDict) Insert(n int, expr ExprKV) {
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
func (e ExprDict) AppendComma(expr ExprKV, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (e ExprDict) InsertComma(n int, expr ExprKV, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)
	if expr.Nil() {
		panic("protocompile/ast: cannot append nil ExprField to ExprMessage")
	}

	e.raw.fields = slices.Insert(e.raw.fields, n, withComma[arena.Pointer[rawExprKV]]{expr.ptr, comma.raw})
}

// AsMessage implements [Expr].
func (e ExprDict) AsMessage() Commas[ExprKV] {
	return e
}

// Span implements [Spanner].
func (e ExprDict) Span() Span {
	return e.Braces().Span()
}

func (e ExprDict) exprRaw() (exprKind, arena.Untyped) {
	return exprDict, e.ptr.Untyped()
}

// ExprKV is a key-value pair within an [ExprDict].
//
// It implements [Expr], since it can appear inside of e.g. an array if the user incorrectly writes [foo: bar].
type ExprKV struct {
	baseExpr
	withContext

	ptr arena.Pointer[rawExprKV]
	raw *rawExprKV
}

type rawExprKV struct {
	key, value rawExpr
	colon      rawToken
}

// ExprKVArgs is arguments for [Context.NewExprKV].
type ExprKVArgs struct {
	Key   Expr
	Colon Token
	Value Expr
}

// Key returns the key for this field.
//
// May be nil if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprKV) Key() Expr {
	return e.raw.key.With(e)
}

// SetKey sets the key for this field.
//
// If passed nil, this clears the key.
func (e ExprKV) SetKey(expr Expr) {
	e.raw.key = toRawExpr(expr)
}

// Colon returns the colon between Key() and Value().
//
// May be nil: it is valid for a field name to be immediately followed by its value and be syntactically
// valid (unlike most "optional" punctuation, this is permitted by Protobuf, not just our permissive AST).
func (e ExprKV) Colon() Token {
	return e.raw.colon.With(e)
}

// Value returns the value for this field.
func (e ExprKV) Value() Expr {
	return e.raw.value.With(e)
}

// SetValue sets the value for this field.
//
// If passed nil, this clears the expression.
func (e ExprKV) SetValue(expr Expr) {
	e.raw.value = toRawExpr(expr)
}

// Span implements [Spanner].
func (e ExprKV) Span() Span {
	return JoinSpans(e.Key(), e.Colon(), e.Value())
}

func (e ExprKV) exprRaw() (exprKind, arena.Untyped) {
	return exprKV, e.ptr.Untyped()
}

const (
	exprLiteral exprKind = iota + 1
	exprPrefixed
	exprPath
	exprRange
	exprArray
	exprDict
	exprKV
	expr
)

type exprKind int8

// rawExpr is the raw representation of an expression.
//
// Similar to rawType (see type.go), this makes use of the fact that for rawPath,
// if the first element is negative, the other must be zero. See also rawType.With.
type rawExpr rawPath

func toRawExpr(e Expr) rawExpr {
	if e == nil {
		return rawExpr{}
	}
	if path, ok := e.(ExprPath); ok {
		return rawExpr(path.Path.raw)
	}

	k, p := e.exprRaw()
	return rawExpr{^rawToken(k), rawToken(p)}
}

// With extracts an expression out of a context at the given index to present to the user.
func (e rawExpr) With(c Contextual) Expr {
	if e[0] == 0 && e[1] == 0 {
		return nil
	}

	if e[0] < 0 && e[1] != 0 {
		c := c.Context()
		ptr := arena.Untyped(e[1])
		switch exprKind(^e[0]) {
		case exprLiteral:
			return ExprLiteral{Token: rawToken(ptr).With(c)}
		case exprPrefixed:
			ptr := arena.Pointer[rawExprPrefixed](ptr)
			return ExprPrefixed{withContext: withContext{c}, ptr: ptr, raw: c.exprs.prefixes.Deref(ptr)}
		case exprRange:
			ptr := arena.Pointer[rawExprRange](ptr)
			return ExprRange{withContext: withContext{c}, ptr: ptr, raw: c.exprs.ranges.Deref(ptr)}
		case exprArray:
			ptr := arena.Pointer[rawExprArray](ptr)
			return ExprArray{withContext: withContext{c}, ptr: ptr, raw: c.exprs.arrays.Deref(ptr)}
		case exprDict:
			ptr := arena.Pointer[rawExprDict](ptr)
			return ExprDict{withContext: withContext{c}, ptr: ptr, raw: c.exprs.dicts.Deref(ptr)}
		case exprKV:
			ptr := arena.Pointer[rawExprKV](ptr)
			return ExprKV{withContext: withContext{c}, ptr: ptr, raw: c.exprs.kvs.Deref(ptr)}
		default:
			return nil
		}
	}

	return ExprPath{Path: rawPath(e).With(c)}
}

// baseExpr implements most of the methods of expr, but returning default values.
// Intended for embedding.
type baseExpr struct{}

func (baseExpr) AsBool() (bool, bool)       { return false, false }
func (baseExpr) AsInt32() (int32, bool)     { return 0, false }
func (baseExpr) AsInt64() (int64, bool)     { return 0, false }
func (baseExpr) AsUInt32() (uint32, bool)   { return 0, false }
func (baseExpr) AsUInt64() (uint64, bool)   { return 0, false }
func (baseExpr) AsFloat32() (float32, bool) { return 0, false }
func (baseExpr) AsFloat64() (float64, bool) { return 0, false }
func (baseExpr) AsString() (string, bool)   { return "", false }
func (baseExpr) AsArray() Commas[Expr]      { return nil }
func (baseExpr) AsMessage() Commas[ExprKV]  { return nil }
