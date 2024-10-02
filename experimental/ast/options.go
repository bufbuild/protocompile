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
	"golang.org/x/exp/slices"
)

// Options represents the collection of options attached to a field-like declaration,
// contained within square brackets.
//
// Options implements [Commas] over its options.
type Options struct {
	withContext

	ptr arena.Pointer[optionsImpl]
	raw *optionsImpl
}

type optionsImpl struct {
	brackets rawToken
	options  []struct {
		option rawOption
		comma  rawToken
	}
}

var _ Commas[Option] = Options{}

type Option struct {
	Path   Path
	Equals Token
	Value  Expr
}

type rawOption struct {
	path   rawPath
	equals rawToken
	value  rawExpr
}

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
	o.raw.options = slices.Delete(o.raw.options, n, n+1)
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
	o.Context().panicIfNotOurs(option.Path, option.Equals, option.Value, comma)

	o.raw.options = slices.Insert(o.raw.options, n, struct {
		option rawOption
		comma  rawToken
	}{
		rawOption{
			path:   option.Path.raw,
			equals: option.Equals.raw,
			value:  toRawExpr(option.Value),
		},
		comma.raw,
	})
}

// Span implements [Spanner] for Options.
func (o Options) Span() Span {
	return JoinSpans(o.Brackets())
}

func (o Options) rawOptions() rawOptions {
	if o.Nil() {
		return 0
	}

	return rawOptions(o.ptr + 1)
}

type rawOptions arena.Pointer[optionsImpl]

func (o rawOptions) With(c Contextual) Options {
	if o == 0 {
		return Options{}
	}
	return Options{
		withContext{c.Context()},
		arena.Pointer[optionsImpl](o),
		c.Context().options.At(arena.Untyped(o)),
	}
}

func (o *rawOption) With(c Contextual) Option {
	if o == nil {
		return Option{}
	}
	return Option{
		Path:   o.path.With(c),
		Equals: o.equals.With(c),
		Value:  o.value.With(c),
	}
}
