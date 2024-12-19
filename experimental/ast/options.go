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
	"slices"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// CompactOptions represents the collection of options attached to a field-like declaration,
// contained within square brackets.
//
// CompactOptions implements [Commas] over its options.
type CompactOptions struct {
	withContext
	raw *rawCompactOptions
}

type rawCompactOptions struct {
	brackets token.ID
	options  []withComma[rawOption]
}

var _ Commas[Option] = CompactOptions{}

// Option is a key-value pair inside of a [CompactOptions] or a [DefOption].
type Option struct {
	Path   Path
	Equals token.Token
	Value  ExprAny
}

type rawOption struct {
	path   rawPath
	equals token.ID
	value  rawExpr
}

// Brackets returns the token tree corresponding to the whole [...].
func (o CompactOptions) Brackets() token.Token {
	return o.raw.brackets.In(o.Context())
}

// Len implements [Slice].
func (o CompactOptions) Len() int {
	return len(o.raw.options)
}

// At implements [Slice].
func (o CompactOptions) At(n int) Option {
	return o.raw.options[n].Value.With(o.Context())
}

// Iter implements [Slice].
func (o CompactOptions) Iter(yield func(int, Option) bool) {
	for i, arg := range o.raw.options {
		if !yield(i, arg.Value.With(o.Context())) {
			break
		}
	}
}

// Append implements [Inserter].
func (o CompactOptions) Append(option Option) {
	o.InsertComma(o.Len(), option, token.Zero)
}

// Insert implements [Inserter].
func (o CompactOptions) Insert(n int, option Option) {
	o.InsertComma(n, option, token.Zero)
}

// Delete implements [Inserter].
func (o CompactOptions) Delete(n int) {
	o.raw.options = slices.Delete(o.raw.options, n, n+1)
}

// Comma implements [Commas].
func (o CompactOptions) Comma(n int) token.Token {
	return o.raw.options[n].Comma.In(o.Context())
}

// AppendComma implements [Commas].
func (o CompactOptions) AppendComma(option Option, comma token.Token) {
	o.InsertComma(o.Len(), option, comma)
}

// InsertComma implements [Commas].
func (o CompactOptions) InsertComma(n int, option Option, comma token.Token) {
	o.Context().Nodes().panicIfNotOurs(option.Path, option.Equals, option.Value, comma)

	o.raw.options = slices.Insert(o.raw.options, n, withComma[rawOption]{
		rawOption{
			path:   option.Path.raw,
			equals: option.Equals.ID(),
			value:  option.Value.raw,
		},
		comma.ID(),
	})
}

// Span implements [report.Spanner].
func (o CompactOptions) Span() report.Span {
	if o.IsZero() {
		return report.Span{}
	}

	return o.Brackets().Span()
}

func wrapOptions(c Context, ptr arena.Pointer[rawCompactOptions]) CompactOptions {
	if ptr.Nil() {
		return CompactOptions{}
	}
	return CompactOptions{
		internal.NewWith(c),
		c.Nodes().options.Deref(ptr),
	}
}

func (o *rawOption) With(c Context) Option {
	if o == nil {
		return Option{}
	}
	return Option{
		Path:   o.path.With(c),
		Equals: o.equals.In(c),
		Value:  newExprAny(c, o.value),
	}
}
