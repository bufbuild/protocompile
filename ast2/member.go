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

	raw *rawField
}

type rawField struct {
	type_  rawType // Not present for enum fields.
	name   rawPath
	equals rawToken
	tag    rawToken
	semi   rawToken      // Missing for groups.
	body   rawDecl[Body] // Present only for groups.
}

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

// Tag returns the field tag. This can be any token, not just a number.
//
// May be nil, if the user left the tag off.
func (f Field) Tag() Token {
	return f.raw.tag.With(f)
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
	return Field{withContext{ctx}, ctx.fields.At(idx)}
}

// Method is a method signature in a service.
type Method struct {
	withContext

	raw *rawMethod
}

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
func (m Method) Signature() (in, out MethodArgs) {
	return MethodArgs{m.withContext, &m.raw.inputs},
		MethodArgs{m.withContext, &m.raw.outputs}
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
	return Method{withContext{ctx}, ctx.methods.At(idx)}
}

type rawMethod struct {
	rpc, returns    rawToken
	name            rawPath
	inputs, outputs rawMethodArgs
	body            rawDecl[Body] // Options live here.
}

// MethodArgs is a [Commas[MethodArg]] over the arguments (or return values)
// of a method.
type MethodArgs struct {
	withContext

	raw *rawMethodArgs
}

type rawMethodArgs struct {
	parens rawToken
	args   []struct {
		ty               rawType
		streaming, comma rawToken
	}
}

// Parens returns the token tree for the parentheses wrapping the MethodArgs.
//
// May be nil, if the user forgot to include parentheses.
func (ma MethodArgs) Parens() Token {
	return ma.raw.parens.With(ma)
}

// Len implements [Slice] for MethodArgs.
func (ma MethodArgs) Len() int {
	return len(ma.raw.args)
}

// At implements [Slice] for MethodArgs.
func (ma MethodArgs) At(n int) MethodArg {
	arg := ma.raw.args[n]
	return MethodArg{arg.ty.With(ma), arg.streaming.With(ma)}
}

// Comma implements [Commas] for MethodArgs.
func (ma MethodArgs) Comma(n int) Token {
	return ma.raw.args[n].comma.With(ma)
}

// At implements [Iter] for MethodArgs.
func (ma MethodArgs) Iter(yield func(int, MethodArg) bool) {
	for i, arg := range ma.raw.args {
		arg := MethodArg{arg.ty.With(ma), arg.streaming.With(ma)}
		if !yield(i, arg) {
			break
		}
	}
}

// Span implements [Spanner] for MethodArgs.
func (ma MethodArgs) Span() Span {
	if !ma.Parens().Nil() {
		return ma.Parens().Span()
	}

	var span Span
	for _, arg := range ma.raw.args {
		span = JoinSpans(span, arg.streaming.With(ma), arg.ty.With(ma), arg.comma.With(ma))
	}
	return span
}

// MethodArg is an argument (or return value) to a [Method].
type MethodArg struct {
	Type

	// The "stream" keyword token before this type, if any.
	Stream Token
}

// TODO: reserved, extensions
