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
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// CompactOptions represents the collection of options attached to a [DeclAny],
// contained within square brackets.
//
// # Grammar
//
//	CompactOptions := `[` (option `,`?)? `]`
//	option         := Path [:=]? Expr?
type CompactOptions struct {
	withContext
	raw *rawCompactOptions
}

type rawCompactOptions struct {
	brackets token.ID
	options  slicesx.Inline[arena.Pointer[rawOption]]
	commas   slicesx.Inline[token.ID]
}

// Option is a key-value pair inside of a [CompactOptions] or a [DefOption].
type Option struct {
	withContext
	raw *rawOption
}

// OptionArgs is arguments for [Node.NewOption].
type OptionArgs struct {
	Path   Path
	Equals token.Token
	Value  ExprAny
}

// Path returns the path (i.e., the key) for this option.
func (o Option) Path() Path {
	if o.IsZero() {
		return Path{}
	}
	return o.raw.path.With(o.Context())
}

// Equals returns the equals sign for this option, if present.
func (o Option) Equals() token.Token {
	if o.IsZero() {
		return token.Zero
	}
	return o.raw.equals.In(o.Context())
}

// Value returns the expression for the value this option is set to, if present.
func (o Option) Value() ExprAny {
	if o.IsZero() {
		return ExprAny{}
	}
	return newExprAny(o.Context(), o.raw.value)
}

type rawOption struct {
	path   rawPath
	equals token.ID
	value  rawExpr
}

// Brackets returns the token tree corresponding to the whole [...].
func (o CompactOptions) Brackets() token.Token {
	if o.IsZero() {
		return token.Zero
	}

	return o.raw.brackets.In(o.Context())
}

// Entries returns the sequence of options in this CompactOptions.
func (o CompactOptions) Entries() Commas[Option] {
	var (
		opts *slicesx.Inline[arena.Pointer[rawOption]]
		toks *slicesx.Inline[token.ID]
	)
	if !o.IsZero() {
		opts = &o.raw.options
		toks = &o.raw.commas
	}

	return commas[Option, arena.Pointer[rawOption]]{
		ctx: o.Context(),
		InserterWrapper2: seq.WrapInserter2(
			opts, toks,
			func(r arena.Pointer[rawOption], _ token.ID) Option {
				return wrapOption(o.Context(), r)
			},
			func(v Option) (arena.Pointer[rawOption], token.ID) {
				o.Context().Nodes().panicIfNotOurs(v)

				ptr := v.Context().Nodes().options.Compress(v.raw)
				if ptr.Nil() {
					ptr = o.Context().Nodes().options.NewCompressed(*v.raw)
				}

				return ptr, 0
			},
		),
	}
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
		c.Nodes().compactOptions.Deref(ptr),
	}
}

func wrapOption(c Context, ptr arena.Pointer[rawOption]) Option {
	if ptr.Nil() {
		return Option{}
	}
	return Option{
		internal.NewWith(c),
		c.Nodes().options.Deref(ptr),
	}
}
