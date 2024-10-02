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

// Field is a field definition.
//
// We interpret field broadly: enum values are simply fields without a declared
// type. Group declarations are also fields, which will have the special
// [BuiltinGroup] type, and whose inline body is [Field.InlineMessage].
type Field struct {
	withContext

	idx int
	raw *rawField
}

type rawField struct {
	type_   rawType // Not present for enum fields.
	name    rawPath
	equals  rawToken
	tag     rawExpr
	semi    rawToken   // Missing for groups.
	body    decl[Body] // Present only for groups.
	options *rawOptions
}

var _ Decl = Field{}

// Type returns the type name for this field.
//
// If nil (i.e., there isn't a type name) this is intended to be an enum value.
func (f Field) Type() Type {
	return f.raw.type_.With(f)
}

// IsMessageField returns whether this is a message's field. This type of node
// can occur inside of many other nodes, such as [Extend] and [Oneof].
func (f Field) IsMessageField() bool {
	return f.Type() != nil
}

// IsEnumValue returns whether this is an enum's value.
func (f Field) IsEnumValue() bool {
	return f.Type() == nil
}

// Name returns this field's name.
//
// For permissiveness, this is a path, not a single identifier. You will almost
// always want to do Name().AsIdent(). May be nil in rare cases, such as for
// map<int, int> = 1;.
func (f Field) Name() Path {
	return f.raw.name.With(f)
}

// Equals returns the equals sign after the name.
//
// May be nil, if the user left the tag off.
func (f Field) Equals() Token {
	return f.raw.equals.With(f)
}

// Tag returns the field tag.
//
// This can be any expression, since users may write something like string x = -1;
//
// May be nil, if the user left the tag off.
func (f Field) Tag() Expr {
	return f.raw.tag.With(f)
}

// Options returns this field's options list.
//
// Returns nil if this field does not have one.
func (f Field) Options() Options {
	return f.raw.options.With(f)
}

// Semicolon returns this field's ending semicolon.
//
// May be nil, if not present.
func (f Field) Semicolon() Token {
	return f.raw.semi.With(f)
}

// Body returns this field's "inline message" body.
//
// May be nil, if not present.
func (f Field) InlineMessage() Body {
	return f.raw.body.With(f)
}

// Span implements [Spanner] for Service.
func (f Field) Span() Span {
	return JoinSpans(f.Type(), f.Name(), f.Equals(), f.Tag(), f.Semicolon())
}

func (Field) with(ctx *Context, idx int) Decl {
	return Field{withContext{ctx}, idx, ctx.fields.At(idx)}
}

func (f Field) declIndex() int {
	return f.idx
}

// Method is a method signature in a service.
type Method struct {
	withContext

	idx int
	raw *rawMethod
}

var _ Decl = Method{}

// Keyword returns the "rpc" keyword for this method.
func (m Method) Keyword() Token {
	return m.raw.rpc.With(m)
}

// Name returns this method's name.
//
// For permissiveness, this is a path, not a single identifier. You will almost
// always want to do Name().AsIdent(). May be nil in rare cases, such as for
// rpc(Foo) returns (Bar);
func (m Method) Name() Path {
	return m.raw.name.With(m)
}

// Signature returns the signature for this method.
func (m Method) Signature() (in, out MethodTypes) {
	return MethodTypes{m.withContext, &m.raw.inputs},
		MethodTypes{m.withContext, &m.raw.outputs}
}

// ReturnsKeyword returns the "returns" keyword for this method.
//
// May be nil if the user forgot it.
func (m Method) ReturnsKeyword() Token {
	return m.raw.returns.With(m)
}

// Body returns this method's "body".
//
// This is not a "body" as in a traditional language; it is a message body that
// contains the method's options.
//
// May be nil, if not present.
func (m Method) Body() Body {
	return m.raw.body.With(m)
}

// Span implements [Spanner] for Method.
func (m Method) Span() Span {
	in, out := m.Signature()
	return JoinSpans(m.Keyword(), m.Name(), in, m.ReturnsKeyword(), out, m.Body())
}

func (Method) with(ctx *Context, idx int) Decl {
	return Method{withContext{ctx}, idx, ctx.methods.At(idx)}
}

func (m Method) declIndex() int {
	return m.idx
}

type rawMethod struct {
	rpc, returns    rawToken
	name            rawPath
	inputs, outputs rawMethodTypes
	body            decl[Body] // Options live here.
}

// MethodTypes is a [Commas[Type]] over the arguments (or return values) of a method.
//
// Protobuf methods can't have more than one input/output, of course, but we parse arbitrarily many.
//
// Note that the "stream" keyword prefix on an input/output is represented as a [TypeModified]
// with [ModifierStream] set.
type MethodTypes struct {
	withContext

	raw *rawMethodTypes
}

var _ Commas[Type] = MethodTypes{}

type rawMethodTypes struct {
	parens rawToken
	args   []struct {
		ty    rawType
		comma rawToken
	}
}

// Parens returns the token tree for the parentheses wrapping the MethodTypes.
//
// May be nil, if the user forgot to include parentheses.
func (m MethodTypes) Parens() Token {
	return m.raw.parens.With(m)
}

// Len implements [Slice] for MethodTypes.
func (m MethodTypes) Len() int {
	return len(m.raw.args)
}

// At implements [Slice] for MethodTypes.
func (m MethodTypes) At(n int) Type {
	return m.raw.args[n].ty.With(m)
}

// At implements [Iter] for MethodTypes.
func (m MethodTypes) Iter(yield func(int, Type) bool) {
	for i, arg := range m.raw.args {
		if !yield(i, arg.ty.With(m)) {
			break
		}
	}
}

// Append implements [Inserter] for TypeGeneric.
func (m MethodTypes) Append(ty Type) {
	m.InsertComma(m.Len(), ty, Token{})
}

// Insert implements [Inserter] for TypeGeneric.
func (m MethodTypes) Insert(n int, ty Type) {
	m.InsertComma(n, ty, Token{})
}

// Delete implements [Inserter] for TypeGeneric.
func (m MethodTypes) Delete(n int) {
	deleteSlice(&m.raw.args, n)
}

// Comma implements [Commas] for MethodTypes.
func (m MethodTypes) Comma(n int) Token {
	return m.raw.args[n].comma.With(m)
}

// AppendComma implements [Commas] for MethodTypes.
func (m MethodTypes) AppendComma(ty Type, comma Token) {
	m.InsertComma(m.Len(), ty, comma)
}

// InsertComma implements [Commas] for MethodTypes.
func (m MethodTypes) InsertComma(n int, ty Type, comma Token) {
	m.Context().panicIfNotOurs(ty, comma)

	insertSlice(&m.raw.args, n, struct {
		ty    rawType
		comma rawToken
	}{ty.rawType(), comma.id})
}

// Span implements [Spanner] for MethodTypes.
func (m MethodTypes) Span() Span {
	if !m.Parens().Nil() {
		return m.Parens().Span()
	}

	var span Span
	for _, arg := range m.raw.args {
		span = JoinSpans(span, arg.ty.With(m), arg.comma.With(m))
	}
	return span
}

// Range represents an extension or reserved range declaration. They are almost identical
// syntactically so they use the same AST node.
//
// In the Protocompile AST, ranges can contain arbitrary expressions. Thus, Range
// implements [Comma[Expr]].
type Range struct {
	withContext

	idx int
	raw *rawRange
}

type rawRange struct {
	keyword rawToken
	args    []struct {
		expr  rawExpr
		comma rawToken
	}
	options *rawOptions
	semi    rawToken
}

var (
	_ Decl         = Range{}
	_ Commas[Expr] = Range{}
)

// Keyword returns the "extensions" keyword for this extensions range.
func (r Range) Keyword() Token {
	return r.raw.keyword.With(r)
}

// Len implements [Slice] for Extensions.
func (r Range) Len() int {
	return len(r.raw.args)
}

// At implements [Slice] for Range.
func (r Range) At(n int) Expr {
	return r.raw.args[n].expr.With(r)
}

// Iter implements [Slice] for Range.
func (r Range) Iter(yield func(int, Expr) bool) {
	for i, arg := range r.raw.args {
		if !yield(i, arg.expr.With(r)) {
			break

		}
	}
}

// Append implements [Inserter] for Range.
func (r Range) Append(expr Expr) {
	r.InsertComma(r.Len(), expr, Token{})
}

// Insert implements [Inserter] for Range.
func (r Range) Insert(n int, expr Expr) {
	r.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter] for Range.
func (r Range) Delete(n int) {
	deleteSlice(&r.raw.args, n)
}

// Comma implements [Commas] for Range.
func (r Range) Comma(n int) Token {
	return r.raw.args[n].comma.With(r)
}

// AppendComma implements [Commas] for Range.
func (r Range) AppendComma(expr Expr, comma Token) {
	r.InsertComma(r.Len(), expr, comma)
}

// InsertComma implements [Commas] for Range.
func (r Range) InsertComma(n int, expr Expr, comma Token) {
	r.Context().panicIfNotOurs(expr, comma)

	insertSlice(&r.raw.args, n, struct {
		expr  rawExpr
		comma rawToken
	}{expr.rawExpr(), comma.id})
}

// Options returns this range's options list.
//
// Returns nil if this field does not have one.
func (r Range) Options() Options {
	return r.raw.options.With(r)
}

// Semicolon returns this range's ending semicolon.
//
// May be nil, if not present.
func (r Range) Semicolon() Token {
	return r.raw.semi.With(r)
}

// Span implements [Spanner] for Range.
func (r Range) Span() Span {
	span := JoinSpans(r.Keyword(), r.Semicolon(), r.Options())
	for _, arg := range r.raw.args {
		span = JoinSpans(span, arg.expr.With(r), arg.comma.With(r))
	}
	return span
}

func (Range) with(ctx *Context, idx int) Decl {
	return Range{withContext{ctx}, idx, ctx.ranges.At(idx)}
}

func (r Range) declIndex() int {
	return r.idx
}
