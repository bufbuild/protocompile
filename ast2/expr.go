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

package ast2

import (
	"math"
)

const (
	exprLiteral exprKind = iota + 1
	exprSigned
	exprPath
	exprRange
	exprArray
	exprMessage
	exprField
)

const (
	keyPath keyKind = iota + 1
	keyExtn
	keyAny
)

// Expr is an expression, primarily occurring on the right hand side of an =.
//
// Expr provides methods for interpreting it as various Go types, as a shorthand
// for introspecting the concrete type of the expression. These methods return
// (T, bool), returning false if the expression cannot be interpreted as that
// type. The [Commas]-returning methods return nil instead.
// TODO: Return a diagnostic instead.
//
// Implemented by [ExprLiteral], [ExprPath], [ExprSigned], [ExprArray], [ExprMessage], and [ExprField].
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
	AsMessage() Commas[ExprField]

	exprKind() exprKind
	rawExpr() rawExpr
}

func (ExprLiteral) exprKind() exprKind { return exprLiteral }
func (ExprPath) exprKind() exprKind    { return exprPath }
func (ExprSigned) exprKind() exprKind  { return exprSigned }
func (ExprRange) exprKind() exprKind   { return exprRange }
func (ExprArray) exprKind() exprKind   { return exprArray }
func (ExprMessage) exprKind() exprKind { return exprMessage }
func (ExprField) exprKind() exprKind   { return exprField }

// ExprLiteral is an expression corresponding to a string or number literal.
type ExprLiteral struct {
	baseExpr

	// The token backing this expression. Must be [TokenString] or [TokenNumber].
	Token
}

var _ Expr = ExprLiteral{}

// AsInt32 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsInt32() (int32, bool) {
	n, ok := e.Token.AsInt()
	return int32(n), ok && n <= uint64(math.MaxInt32)
}

// AsInt64 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsInt64() (int64, bool) {
	n, ok := e.Token.AsInt()
	return int64(n), ok && n <= uint64(math.MaxInt64)
}

// AsUInt32 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsUInt32() (uint32, bool) {
	n, ok := e.Token.AsInt()
	return uint32(n), ok && n <= uint64(math.MaxUint32)
}

// AsUInt64 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsUInt64() (uint64, bool) {
	return e.Token.AsInt()
}

// AsFloat32 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsFloat32() (float32, bool) {
	n, ok := e.Token.AsFloat()
	return float32(n), ok // Loss of precision is intentional.
}

// AsFloat64 implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsFloat64() (float64, bool) {
	return e.Token.AsFloat()
}

// AsString implements [Expr] for ExprLiteral.
func (e ExprLiteral) AsString() (string, bool) {
	return e.Token.AsString()
}

func (e ExprLiteral) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprLiteral), e.Token.id}
}

// ExprPath is a Protobuf path in expression position.
//
// Note: if this is BuiltinMax,
type ExprPath struct {
	baseExpr

	// The path backing this expression.
	Path
}

var _ Expr = ExprPath{}

// AsBool implements [Expr] for ExprPath.
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

// AsFloat32 implements [Expr] for ExprPath.
func (e ExprPath) AsFloat32() (float32, bool) {
	n, ok := e.AsFloat64()
	return float32(n), ok
}

// AsFloat64 implements [Expr] for ExprPath.
func (e ExprPath) AsFloat64() (float64, bool) {
	switch e.AsIdent().Text() {
	case "inf":
		return math.Inf(1), true
	case "nan":
		return math.NaN(), true
	default:
		return 0, false
	}
}

func (e ExprPath) rawExpr() rawExpr {
	return rawExpr(e.Path.raw)
}

// ExprSigned is an expression prefixed with a minus sign.
type ExprSigned struct {
	baseExpr
	withContext

	idx int
	raw *rawExprSigned
}

type rawExprSigned struct {
	sign rawToken
	expr rawExpr
}

var _ Expr = ExprSigned{}

// Sign returns the token representing this expression's sign.
func (e ExprSigned) Sign() Token {
	return e.raw.sign.With(e)
}

// Inner returns the expression the sign is applied to.
func (e ExprSigned) Inner() Expr {
	return e.raw.expr.With(e)
}

// Span implements [Spanner] for ExprSigned.
func (e ExprSigned) Span() Span {
	return JoinSpans(e.Sign(), e.Inner())
}

// AsInt32 implements [Expr] for ExprSigned.
func (e ExprSigned) AsInt32() (int32, bool) {
	n, ok := e.AsInt64()
	if !ok || n < int64(math.MinInt32) || n > int64(math.MaxInt32) {
		return 0, false
	}

	return int32(n), true
}

// AsInt64 implements [Expr] for ExprSigned.
func (e ExprSigned) AsInt64() (int64, bool) {
	n, ok := e.Inner().AsInt64()
	if ok && n != -n {
		// If n == -n, that means n == MinInt32.
		return -n, ok
	}

	// Need to handle the funny case where someone wrote -9223372036854775808, since
	// 9223372036854775808 is not representable as an int64.
	u, ok := e.Inner().AsUInt64()
	if ok && u == uint64(math.MaxInt64) {
		return math.MinInt64, true
	}

	return 0, false
}

// AsUInt32 implements [Expr] for ExprSigned.
func (e ExprSigned) AsUInt32() (uint32, bool) {
	// NOTE: - is not treated as two's complement here; we only allow -0
	n, ok := e.Inner().AsUInt32()
	return 0, ok && n == 0
}

// AsUInt64 implements [Expr] for ExprSigned.
func (e ExprSigned) AsUInt64() (uint64, bool) {
	// NOTE: - is not treated as two's complement here; we only allow -0
	n, ok := e.Inner().AsUInt64()
	return 0, ok && n == 0
}

// AsFloat32 implements [Expr] for ExprSigned.
func (e ExprSigned) AsFloat32() (float32, bool) {
	n, ok := e.Inner().AsFloat32()
	return -n, ok
}

// AsFloat64 implements [Expr] for ExprSigned.
func (e ExprSigned) AsFloat64() (float64, bool) {
	n, ok := e.Inner().AsFloat64()
	return -n, ok
}

func (e ExprSigned) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprSigned), rawToken(e.idx)}
}

// ExprRange represents a range of values, such as 1 to 4 or 5 to max.
//
// Note that max is not special syntax; it will appear as an [ExprPath] with the name "max".
type ExprRange struct {
	baseExpr
	withContext

	idx int
	raw *rawExprRange
}

var _ Expr = ExprRange{}

// Bounds returns this range's bounds. These are inclusive bounds.
func (e ExprRange) Bounds() (Expr, Expr) {
	return e.raw.lo.With(e), e.raw.hi.With(e)
}

// Keyword returns the "to" keyword for this range.
func (e ExprRange) Keyword() Token {
	return e.raw.to.With(e)
}

// Span implements [Spanner] for ExprRange.
func (e ExprRange) Span() Span {
	lo, hi := e.Bounds()
	return JoinSpans(lo, e.Keyword(), hi)
}

func (e ExprRange) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprRange), rawToken(e.idx)}
}

type rawExprRange struct {
	lo, hi rawExpr
	to     rawToken
}

// ExprArray represents an array of expressions between square brackets.
//
// ExprArray implements [Commas[Expr]].
type ExprArray struct {
	baseExpr
	withContext

	idx int
	raw *rawExprArray
}

type rawExprArray struct {
	brackets rawToken
	args     []struct {
		expr  rawExpr
		comma rawToken
	}
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

// Len implements [Slice] for ExprArray.
func (e ExprArray) Len() int {
	return len(e.raw.args)
}

// At implements [Slice] for ExprArray.
func (e ExprArray) At(n int) Expr {
	return e.raw.args[n].expr.With(e)
}

// Iter implements [Slice] for ExprArray.
func (e ExprArray) Iter(yield func(int, Expr) bool) {
	for i, arg := range e.raw.args {
		if !yield(i, arg.expr.With(e)) {
			break

		}
	}
}

// Append implements [Inserter] for ExprArray.
func (e ExprArray) Append(expr Expr) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter] for ExprArray.
func (e ExprArray) Insert(n int, expr Expr) {
	e.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter] for ExprArray.
func (e ExprArray) Delete(n int) {
	deleteSlice(&e.raw.args, n)
}

// Comma implements [Commas] for ExprArray.
func (e ExprArray) Comma(n int) Token {
	return e.raw.args[n].comma.With(e)
}

// AppendComma implements [Commas] for TypeGeneric.
func (e ExprArray) AppendComma(expr Expr, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas] for TypeGeneric.
func (e ExprArray) InsertComma(n int, expr Expr, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)

	insertSlice(&e.raw.args, n, struct {
		expr  rawExpr
		comma rawToken
	}{expr.rawExpr(), comma.id})
}

// AsArray implements [Expr] for ExprArray.
func (e ExprArray) AsArray() Commas[Expr] {
	return e
}

// Span implements [Spanner] for ExprArray.
func (e ExprArray) Span() Span {
	return e.Brackets().Span()
}

func (e ExprArray) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprArray), rawToken(e.idx)}
}

// ExprMessage represents a an array of message fields between curly braces.
type ExprMessage struct {
	baseExpr
	withContext

	idx int
	raw *rawExprMessage
}

type rawExprMessage struct {
	braces rawToken
	fields []struct {
		idx   uint32
		comma rawToken
	}
}

var (
	_ Expr              = ExprMessage{}
	_ Commas[ExprField] = ExprMessage{}
)

// Braces returns the token tree corresponding to the whole {...}.
//
// May be missing for a synthetic expression.
func (e ExprMessage) Braces() Token {
	return e.raw.braces.With(e)
}

// Len implements [Slice] for ExprMessage.
func (e ExprMessage) Len() int {
	return len(e.raw.fields)
}

// At implements [Slice] for ExprMessage.
func (e ExprMessage) At(n int) ExprField {
	idx := int(e.raw.fields[n].idx)
	return ExprField{
		baseExpr{},
		e.withContext,
		idx,
		e.Context().fieldExprs.At(idx),
	}
}

// Iter implements [Slice] for ExprMessage.
func (e ExprMessage) Iter(yield func(int, ExprField) bool) {
	for i, f := range e.raw.fields {
		e := ExprField{
			baseExpr{},
			e.withContext,
			int(f.idx),
			e.Context().fieldExprs.At(int(f.idx)),
		}
		if !yield(i, e) {
			break
		}
	}
}

// Append implements [Inserter] for ExprMessage.
func (e ExprMessage) Append(expr ExprField) {
	e.InsertComma(e.Len(), expr, Token{})
}

// Insert implements [Inserter] for ExprMessage.
func (e ExprMessage) Insert(n int, expr ExprField) {
	e.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter] for ExprMessage.
func (e ExprMessage) Delete(n int) {
	deleteSlice(&e.raw.fields, n)
}

// Comma implements [Commas] for ExprMessage.
func (e ExprMessage) Comma(n int) Token {
	return e.raw.fields[n].comma.With(e)
}

// AppendComma implements [Commas] for TypeGeneric.
func (e ExprMessage) AppendComma(expr ExprField, comma Token) {
	e.InsertComma(e.Len(), expr, comma)
}

// InsertComma implements [Commas] for TypeGeneric.
func (e ExprMessage) InsertComma(n int, expr ExprField, comma Token) {
	e.Context().panicIfNotOurs(expr, comma)
	if expr.Nil() {
		panic("protocompile/ast: cannot append nil ExprField to ExprMessage")
	}

	insertSlice(&e.raw.fields, n, struct {
		idx   uint32
		comma rawToken
	}{uint32(expr.idx), comma.id})
}

// AsMessage implements [Expr] for ExprMessage.
func (e ExprMessage) AsMessage() Commas[ExprField] {
	return e
}

// Span implements [Spanner] for ExprMessage.
func (e ExprMessage) Span() Span {
	return e.Braces().Span()
}

func (e ExprMessage) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprMessage), rawToken(e.idx)}
}

// ExprField is a field within an [ExprMessage].
//
// It implements [Expr], since it can appear inside of e.g. an array if the user incorrectly writes [foo: bar].
type ExprField struct {
	baseExpr
	withContext

	idx int
	raw *rawExprField
}

type rawExprField struct {
	key   rawKey
	colon rawToken
	value rawExpr
}

// Key returns the key for this field.
//
// May be nil if the parser encounters a message expression with a missing field, e.g. {foo, bar: baz}.
func (e ExprField) Key() Key {
	return e.raw.key.With(e)
}

// Colon returns the colon between Key() and Value().
//
// May be nil: it is valid for a field name to be immediately followed by its value and be syntactically
// valid (unlike most "optional" punctuation, this is required by Protobuf, not just our permissive AST).
func (e ExprField) Colon() Token {
	return e.raw.colon.With(e)
}

// Value returns the value for this field.
func (e ExprField) Value() Expr {
	return e.raw.value.With(e)
}

// Span implements [Spanner] for ExprField.
func (e ExprField) Span() Span {
	return JoinSpans(e.Key(), e.Colon(), e.Value())
}

func (e ExprField) rawExpr() rawExpr {
	return rawExpr{^rawToken(exprField), rawToken(e.idx)}
}

// Key is a [ExprField]'s field name.
//
// This is almost always a single identifier, but it may be a path, a bracketed path, or a
// hostname-scoped path!
//
// Implemented by [KeyPath], [KeyExtension], and [KeyAny].
type Key interface {
	Spanner

	keyKind() keyKind
}

func (KeyPath) keyKind() keyKind      { return keyPath }
func (KeyExtension) keyKind() keyKind { return keyExtn }
func (KeyAny) keyKind() keyKind       { return keyExtn }

// KeyPath a simple key.
//
// Virtually all KeyPaths will be single identifiers: multi-element key paths are generally
// not allowed, but are included for permittivity.
type KeyPath struct {
	// The path backing this key.
	Path
}

var _ Key = KeyPath{}

// KeyExtension is a key for an extension field, consisting of that extension's path in
// square brackets.
type KeyExtension struct {
	withContext

	raw *rawKeyExtension
}

var _ Key = KeyExtension{}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e KeyExtension) Brackets() Token {
	return e.raw.brackets.With(e)
}

// Path returns the extension's path.
func (e KeyExtension) Path() Path {
	return e.raw.path.With(e)
}

// Span implements [Spanner] for KeyExtension.
func (e KeyExtension) Span() Span {
	return e.Brackets().Span()
}

type rawKeyExtension struct {
	brackets rawToken
	path     rawPath
}

// KeyExtension is a key for an Any field, consisting of the Any's host-qualified name in square
// brackets (e.g. [example.com/my.cool.Proto]).
type KeyAny struct {
	withContext

	raw *rawKeyAny
}

var _ Key = KeyAny{}

// Brackets returns the token tree corresponding to the whole [...].
//
// May be missing for a synthetic expression.
func (e KeyAny) Brackets() Token {
	return e.raw.brackets.With(e)
}

// Host returns the path for the host the type is scoped under.
func (e KeyAny) Host() Path {
	return e.raw.host.With(e)
}

// Slash returns the / between Hos() and Type().
func (e KeyAny) Slash() Token {
	return e.raw.slash.With(e)
}

// Type returns the path for the Any'd type.
func (e KeyAny) Type() Path {
	return e.raw.path.With(e)
}

// Span implements [Spanner] for KeyAny.
func (e KeyAny) Span() Span {
	return e.Brackets().Span()
}

type rawKeyAny struct {
	brackets, slash rawToken
	host, path      rawPath
}

type keyKind int8
type exprKind int8

// rawExpr is the raw representation of an expression.
//
// Similar to rawType (see type.go), this makes use of the fact that for rawPath,
// if the first element is negative, the other must be zero. See also rawType.With.
type rawExpr rawPath

// With extracts an expression out of a context at the given index to present to the user.
func (e rawExpr) With(c Contextual) Expr {
	if e[0] == 0 && e[1] == 0 {
		return nil
	}

	if e[0] < 0 && e[1] != 0 {
		c := c.Context()
		idx := int(e[1]) // NOTE: no -1 here, nil is represented by 0, 0 above.
		switch exprKind(^e[0]) {
		case exprLiteral:
			return ExprLiteral{Token: rawToken(idx).With(c)}
		case exprSigned:
			return ExprSigned{withContext: withContext{c}, raw: c.signedExprs.At(idx)}
		case exprRange:
			return ExprRange{withContext: withContext{c}, raw: c.rangeExprs.At(idx)}
		case exprArray:
			return ExprArray{withContext: withContext{c}, raw: c.arrayExprs.At(idx)}
		case exprMessage:
			return ExprMessage{withContext: withContext{c}, raw: c.messageExprs.At(idx)}
		case exprField:
			return ExprField{withContext: withContext{c}, raw: c.fieldExprs.At(idx)}
		default:
			return nil
		}
	}

	return ExprPath{Path: rawPath(e).With(c)}
}

// rawKey is the raw representation of a key.
//
// Similar to rawType (see type.go), this makes use of the fact that for rawPath,
// if the first element is negative, the other must be zero. See also rawType.With.
type rawKey rawPath

// With extracts a key out of a context at the given index to present to the user.
func (e rawKey) With(c Contextual) Key {
	if e[0] == 0 && e[1] == 0 {
		return nil
	}

	if e[0] < 0 && e[1] != 0 {
		c := c.Context()
		idx := int(e[1]) // NOTE: no -1 here, nil is represented by 0, 0 above.
		switch keyKind(^e[0]) {
		case keyExtn:
			return KeyExtension{withContext{c}, c.extnKeys.At(idx)}
		case keyAny:
			return KeyAny{withContext{c}, c.anyKeys.At(idx)}
		default:
			return nil
		}
	}

	return KeyPath{Path: rawPath(e).With(c)}
}

// baseExpr implements most of the methods of expr, but returning default values.
// Intended for embedding.
type baseExpr struct{}

func (baseExpr) AsBool() (bool, bool)         { return false, false }
func (baseExpr) AsInt32() (int32, bool)       { return 0, false }
func (baseExpr) AsInt64() (int64, bool)       { return 0, false }
func (baseExpr) AsUInt32() (uint32, bool)     { return 0, false }
func (baseExpr) AsUInt64() (uint64, bool)     { return 0, false }
func (baseExpr) AsFloat32() (float32, bool)   { return 0, false }
func (baseExpr) AsFloat64() (float64, bool)   { return 0, false }
func (baseExpr) AsString() (string, bool)     { return "", false }
func (baseExpr) AsArray() Commas[Expr]        { return nil }
func (baseExpr) AsMessage() Commas[ExprField] { return nil }
