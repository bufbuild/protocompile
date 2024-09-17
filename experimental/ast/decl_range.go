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

	"github.com/bufbuild/protocompile/internal/arena"
)

// DeclRange represents an extension or reserved range declaration. They are almost identical
// syntactically so they use the same AST node.
//
// In the Protocompile AST, ranges can contain arbitrary expressions. Thus, DeclRange
// implements [Comma[Expr]].
type DeclRange struct {
	withContext

	ptr arena.Untyped
	raw *rawDeclRange
}

type rawDeclRange struct {
	keyword rawToken
	args    []struct {
		expr  rawExpr
		comma rawToken
	}
	options arena.Pointer[rawCompactOptions]
	semi    rawToken
}

// DeclRangeArgs is arguments for [Context.NewDeclRange].
type DeclRangeArgs struct {
	Keyword   Token
	Options   CompactOptions
	Semicolon Token
}

var (
	_ Decl         = DeclRange{}
	_ Commas[Expr] = DeclRange{}
)

// Keyword returns the keyword for this range.
func (d DeclRange) Keyword() Token {
	return d.raw.keyword.With(d)
}

// IsExtensions checks whether this is an extension range.
func (d DeclRange) IsExtensions() bool {
	return d.Keyword().Text() == "extensions"
}

// IsReserved checks whether this is a reserved range.
func (d DeclRange) IsReserved() bool {
	return d.Keyword().Text() == "reserved"
}

// Len implements [Slice] for Extensions.
func (d DeclRange) Len() int {
	return len(d.raw.args)
}

// At implements [Slice] for Range.
func (d DeclRange) At(n int) Expr {
	return d.raw.args[n].expr.With(d)
}

// Iter implements [Slice] for Range.
func (d DeclRange) Iter(yield func(int, Expr) bool) {
	for i, arg := range d.raw.args {
		if !yield(i, arg.expr.With(d)) {
			break
		}
	}
}

// Append implements [Inserter] for Range.
func (d DeclRange) Append(expr Expr) {
	d.InsertComma(d.Len(), expr, Token{})
}

// Insert implements [Inserter] for Range.
func (d DeclRange) Insert(n int, expr Expr) {
	d.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter] for Range.
func (d DeclRange) Delete(n int) {
	d.raw.args = slices.Delete(d.raw.args, n, n+1)
}

// Comma implements [Commas] for Range.
func (d DeclRange) Comma(n int) Token {
	return d.raw.args[n].comma.With(d)
}

// AppendComma implements [Commas] for Range.
func (d DeclRange) AppendComma(expr Expr, comma Token) {
	d.InsertComma(d.Len(), expr, comma)
}

// InsertComma implements [Commas] for Range.
func (d DeclRange) InsertComma(n int, expr Expr, comma Token) {
	d.Context().panicIfNotOurs(expr, comma)

	d.raw.args = slices.Insert(d.raw.args, n, struct {
		expr  rawExpr
		comma rawToken
	}{toRawExpr(expr), comma.raw})
}

// Options returns the compact options list for this range.
func (d DeclRange) Options() CompactOptions {
	return newOptions(d.raw.options, d)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclRange) SetOptions(opts CompactOptions) {
	d.raw.options = opts.ptr
}

// Semicolon returns this range's ending semicolon.
//
// May be nil, if not present.
func (d DeclRange) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Span implements [Spanner] for Range.
func (d DeclRange) Span() Span {
	span := JoinSpans(d.Keyword(), d.Semicolon(), d.Options())
	for _, arg := range d.raw.args {
		span = JoinSpans(span, arg.expr.With(d), arg.comma.With(d))
	}
	return span
}

func (DeclRange) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclRange{withContext{ctx}, ptr, ctx.decls.ranges.At(ptr)}
}

func (d DeclRange) declIndex() arena.Untyped {
	return d.ptr
}
