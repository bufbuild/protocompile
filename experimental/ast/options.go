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

package ast

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// CompactOptions represents the collection of options attached to a [DeclAny],
// contained within square brackets.
//
// # Grammar
//
//	CompactOptions := `[` (option `,`?)? `]`
//	option         := Path [:=]? Expr?
type CompactOptions id.Node[CompactOptions, *File, *rawCompactOptions]

type rawCompactOptions struct {
	brackets token.ID
	options  []withComma[rawOption]
}

// Option is a key-value pair inside of a [CompactOptions] or a [DefOption].
type Option struct {
	Path   Path
	Equals token.Token
	Value  ExprAny
}

// Span implements [source.Spanner].
func (o Option) Span() source.Span {
	return source.Join(o.Path, o.Equals, o.Value)
}

type rawOption struct {
	path   PathID
	equals token.ID
	value  id.Dyn[ExprAny, ExprKind]
}

// Brackets returns the token tree corresponding to the whole [...].
func (o CompactOptions) Brackets() token.Token {
	if o.IsZero() {
		return token.Zero
	}

	return id.Wrap(o.Context().Stream(), o.Raw().brackets)
}

// Entries returns the sequence of options in this CompactOptions.
func (o CompactOptions) Entries() Commas[Option] {
	type slice = commas[Option, rawOption]
	if o.IsZero() {
		return slice{}
	}
	return slice{
		file: o.Context(),
		SliceInserter: seq.NewSliceInserter(
			&o.Raw().options,
			func(_ int, c withComma[rawOption]) Option {
				return c.Value.With(o.Context())
			},
			func(_ int, v Option) withComma[rawOption] {
				o.Context().Nodes().panicIfNotOurs(v.Path, v.Equals, v.Value)
				return withComma[rawOption]{Value: rawOption{
					path:   v.Path.ID(),
					equals: v.Equals.ID(),
					value:  v.Value.ID(),
				}}
			},
		),
	}
}

// Span implements [source.Spanner].
func (o CompactOptions) Span() source.Span {
	if o.IsZero() {
		return source.Span{}
	}

	return o.Brackets().Span()
}

func (o *rawOption) With(f *File) Option {
	if o == nil {
		return Option{}
	}
	return Option{
		Path:   o.path.In(f),
		Equals: id.Wrap(f.Stream(), o.equals),
		Value:  id.WrapDyn(f, o.value),
	}
}
