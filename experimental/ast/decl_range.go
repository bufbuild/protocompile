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
	"slices"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// DeclRange represents an extension or reserved range declaration. They are almost identical
// syntactically so they use the same AST node.
//
// In the Protocompile AST, ranges can contain arbitrary expressions. Thus, DeclRange
// implements [Comma[ExprAny]].
//
// # Grammar
//
//	DeclRange := (`extensions` | `reserved`) (Expr `,`)* Expr? CompactOptions? `;`?
type DeclRange struct{ declImpl[rawDeclRange] }

type rawDeclRange struct {
	keyword token.ID
	args    []withComma[rawExpr]
	options arena.Pointer[rawCompactOptions]
	semi    token.ID
}

// DeclRangeArgs is arguments for [Context.NewDeclRange].
type DeclRangeArgs struct {
	Keyword   token.Token
	Options   CompactOptions
	Semicolon token.Token
}

var (
	_ Commas[ExprAny] = DeclRange{}
)

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

// Len implements [Slice].
func (d DeclRange) Len() int {
	if d.IsZero() {
		return 0
	}

	return len(d.raw.args)
}

// At implements [Slice].
func (d DeclRange) At(n int) ExprAny {
	return newExprAny(d.Context(), d.raw.args[n].Value)
}

// Iter implements [Slice].
func (d DeclRange) Iter(yield func(int, ExprAny) bool) {
	if d.IsZero() {
		return
	}
	for i, arg := range d.raw.args {
		if !yield(i, newExprAny(d.Context(), arg.Value)) {
			break
		}
	}
}

// Append implements [Inserter].
func (d DeclRange) Append(expr ExprAny) {
	d.InsertComma(d.Len(), expr, token.Zero)
}

// Insert implements [Inserter].
func (d DeclRange) Insert(n int, expr ExprAny) {
	d.InsertComma(n, expr, token.Zero)
}

// Delete implements [Inserter].
func (d DeclRange) Delete(n int) {
	d.raw.args = slices.Delete(d.raw.args, n, n+1)
}

// Comma implements [Commas].
func (d DeclRange) Comma(n int) token.Token {
	return d.raw.args[n].Comma.In(d.Context())
}

// AppendComma implements [Commas].
func (d DeclRange) AppendComma(expr ExprAny, comma token.Token) {
	d.InsertComma(d.Len(), expr, comma)
}

// InsertComma implements [Commas].
func (d DeclRange) InsertComma(n int, expr ExprAny, comma token.Token) {
	d.Context().Nodes().panicIfNotOurs(expr, comma)

	d.raw.args = slices.Insert(d.raw.args, n, withComma[rawExpr]{expr.raw, comma.ID()})
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
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
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
	switch {
	case d.IsZero():
		return report.Span{}
	case d.Len() == 0:
		return report.Join(d.Keyword(), d.Semicolon(), d.Options())
	default:
		return report.Join(
			d.Keyword(), d.Semicolon(), d.Options(),
			d.At(0),
			d.At(d.Len()-1),
		)
	}
}

func wrapDeclRange(c Context, ptr arena.Pointer[rawDeclRange]) DeclRange {
	return DeclRange{wrapDecl(c, ptr)}
}
