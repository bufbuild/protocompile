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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// DeclRange represents an extension or reserved range declaration. They are almost identical
// syntactically so they use the same AST node.
//
// # Grammar
//
//	DeclRange := (`extensions` | `reserved`) (Expr `,`)* Expr? CompactOptions? `;`?
type DeclRange struct{ declImpl[rawDeclRange] }

type rawDeclRange struct {
	keyword token.ID
	args    slicesx.Inline[rawExpr]
	commas  slicesx.Inline[token.ID]
	options arena.Pointer[rawCompactOptions]
	semi    token.ID
}

// DeclRangeArgs is arguments for [Context.NewDeclRange].
type DeclRangeArgs struct {
	Keyword   token.Token
	Options   CompactOptions
	Semicolon token.Token
}

// Keyword returns the keyword for this range.
func (d DeclRange) Keyword() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.keyword.In(d.Context())
}

// IsExtensions checks whether this is an extension range.
func (d DeclRange) IsExtensions() bool {
	return d.Keyword().Text() == "extensions"
}

// IsReserved checks whether this is a reserved range.
func (d DeclRange) IsReserved() bool {
	return d.Keyword().Text() == "reserved"
}

// Ranges returns the sequence of expressions denoting the ranges in this
// range declaration.
func (d DeclRange) Ranges() Commas[ExprAny] {
	var (
		args *slicesx.Inline[rawExpr]
		toks *slicesx.Inline[token.ID]
	)
	if !d.IsZero() {
		args = &d.raw.args
		toks = &d.raw.commas
	}

	// A single return here promotes devirtualization of both the interface
	// and the funcvals within.
	return commas[ExprAny, rawExpr]{
		ctx: d.Context(),
		InserterWrapper2: seq.WrapInserter2(
			args, toks,
			func(e rawExpr, _ token.ID) ExprAny {
				return newExprAny(d.Context(), e)
			},
			func(e ExprAny) (rawExpr, token.ID) {
				d.Context().Nodes().panicIfNotOurs(e)
				return e.raw, 0
			},
		),
	}
}

// Options returns the compact options list for this range.
func (d DeclRange) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclRange) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().compactOptions.Compress(opts.raw)
}

// Semicolon returns this range's ending semicolon.
//
// May be nil, if not present.
func (d DeclRange) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.semi.In(d.Context())
}

// Span implements [report.Spanner].
func (d DeclRange) Span() report.Span {
	r := d.Ranges()
	switch {
	case d.IsZero():
		return report.Span{}
	case r.Len() == 0:
		return report.Join(d.Keyword(), d.Semicolon(), d.Options())
	default:
		return report.Join(
			d.Keyword(), d.Semicolon(), d.Options(),
			r.At(0),
			r.At(r.Len()-1),
		)
	}
}

func wrapDeclRange(c Context, ptr arena.Pointer[rawDeclRange]) DeclRange {
	return DeclRange{wrapDecl(c, ptr)}
}
