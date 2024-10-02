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

// Option represents an option on some declaration.
type Option struct {
	withContext

	idx int
	raw *rawOption
}

type rawOption struct {
	keyword rawToken
	path    rawPath
	equals  rawToken
	value   rawExpr
	semi    rawToken
}

var _ Decl = Option{}

// Keyword returns the keyword for this option.
//
// May be nil, such as for an Option inside of an Options.
func (o Option) Keyword() Token {
	return o.raw.keyword.With(o)
}

// Path returns the key for this option.
//
// This is the only Path in the whole ast package which can contain extension
// components in a valid AST.
func (o Option) Path() Path {
	return o.raw.path.With(o)
}

// Equals returns the equals sign after the path.
//
// May be nil, if the user wrote something like option foo bar;.
func (o Option) Equals() Token {
	return o.raw.equals.With(o)
}

// Value returns the expression that provides this option's value.
//
// May be nil, if the user wrote something like option foo;.
func (o Option) Value() Expr {
	return o.raw.value.With(o)
}

// Semicolon returns this pragma's ending semicolon.
//
// May be nil, such as for an Option inside of an Options.
func (o Option) Semicolon() Token {
	return o.raw.semi.With(o)
}

// Span implements [Spanner] for Option.
func (o Option) Span() Span {
	return JoinSpans(o.Keyword(), o.Path(), o.Equals(), o.Value(), o.Semicolon())
}

func (Option) with(ctx *Context, idx int) Decl {
	return Option{withContext{ctx}, idx, ctx.options.At(idx)}
}

func (o Option) declIndex() int {
	return o.idx
}

// Options represents the collection of options attached to a field-like declaration,
// contained within square brackets.
//
// Options implements [Commas[Option]].
type Options struct {
	withContext

	raw *rawOptions
}

type rawOptions struct {
	brackets rawToken
	options  []struct {
		option decl[Option]
		comma  rawToken
	}
}

var _ Commas[Option] = Options{}

// Brackets returns the token tree corresponding to the whole [...].
func (o Options) Brackets() Token {
	return o.raw.brackets.With(o)
}

// Len implements [Slice] for Options.
func (o Options) Len() int {
	return len(o.raw.options)
}

// At implements [Slice] for Options.
func (o Options) At(n int) Option {
	return o.raw.options[n].option.With(o)
}

// Iter implements [Slice] for Options.
func (o Options) Iter(yield func(int, Option) bool) {
	for i, arg := range o.raw.options {
		if !yield(i, arg.option.With(o)) {
			break
		}
	}
}

// Append implements [Inserter] for Options.
func (o Options) Append(option Option) {
	o.InsertComma(o.Len(), option, Token{})
}

// Insert implements [Inserter] for Options.
func (o Options) Insert(n int, option Option) {
	o.InsertComma(n, option, Token{})
}

// Delete implements [Inserter] for Options.
func (o Options) Delete(n int) {
	deleteSlice(&o.raw.options, n)
}

// Comma implements [Commas] for Options.
func (o Options) Comma(n int) Token {
	return o.raw.options[n].comma.With(o)
}

// AppendComma implements [Commas] for Options.
func (o Options) AppendComma(option Option, comma Token) {
	o.InsertComma(o.Len(), option, comma)
}

// InsertComma implements [Commas] for Options.
func (o Options) InsertComma(n int, option Option, comma Token) {
	o.Context().panicIfNotOurs(option, comma)
	if option.Nil() {
		panic("protocompile/ast: cannot append nil Option to Options")
	}

	insertSlice(&o.raw.options, n, struct {
		option decl[Option]
		comma  rawToken
	}{declFor(option), comma.id})
}

// Span implements [Spanner] for Options.
func (o Options) Span() Span {
	return JoinSpans(o.Brackets())
}

func (o *rawOptions) With(c Contextual) Options {
	if o == nil {
		return Options{}
	}
	return Options{
		withContext{c.Context()},
		o,
	}
}
