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
// implements [Comma[ExprAny]].
type DeclRange struct{ declImpl[rawDeclRange] }

type rawDeclRange struct {
	keyword rawToken
	args    []withComma[rawExpr]
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
	_ Commas[ExprAny] = DeclRange{}
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

// Len implements [Slice].
func (d DeclRange) Len() int {
	return len(d.raw.args)
}

// At implements [Slice].
func (d DeclRange) At(n int) ExprAny {
	return d.raw.args[n].Value.With(d)
}

// Iter implements [Slice].
func (d DeclRange) Iter(yield func(int, ExprAny) bool) {
	for i, arg := range d.raw.args {
		if !yield(i, arg.Value.With(d)) {
			break
		}
	}
}

// Append implements [Inserter].
func (d DeclRange) Append(expr ExprAny) {
	d.InsertComma(d.Len(), expr, Token{})
}

// Insert implements [Inserter].
func (d DeclRange) Insert(n int, expr ExprAny) {
	d.InsertComma(n, expr, Token{})
}

// Delete implements [Inserter].
func (d DeclRange) Delete(n int) {
	d.raw.args = slices.Delete(d.raw.args, n, n+1)
}

// Comma implements [Commas].
func (d DeclRange) Comma(n int) Token {
	return d.raw.args[n].Comma.With(d)
}

// AppendComma implements [Commas].
func (d DeclRange) AppendComma(expr ExprAny, comma Token) {
	d.InsertComma(d.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (d DeclRange) InsertComma(n int, expr ExprAny, comma Token) {
	d.Context().panicIfNotOurs(expr, comma)

	d.raw.args = slices.Insert(d.raw.args, n, withComma[rawExpr]{expr.raw, comma.raw})
}

// Options returns the compact options list for this range.
func (d DeclRange) Options() CompactOptions {
	return wrapOptions(d, d.raw.options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclRange) SetOptions(opts CompactOptions) {
	d.raw.options = d.ctx.options.Compress(opts.raw)
}

// Semicolon returns this range's ending semicolon.
//
// May be nil, if not present.
func (d DeclRange) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Span implements [Spanner].
func (d DeclRange) Span() Span {
	span := JoinSpans(d.Keyword(), d.Semicolon(), d.Options())
	for _, arg := range d.raw.args {
		span = JoinSpans(span, arg.Value.With(d), arg.Comma.With(d))
	}
	return span
}

func wrapDeclRange(c Contextual, ptr arena.Pointer[rawDeclRange]) DeclRange {
	return DeclRange{wrapDecl(c, ptr)}
}
