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
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// DeclRange represents an extension or reserved range declaration. They are almost identical
// syntactically so they use the same AST node.
//
// # Grammar
//
//	DeclRange := (`extensions` | `reserved`) (Expr `,`)* Expr? CompactOptions? `;`?
type DeclRange id.Node[DeclRange, *File, *rawDeclRange]

type rawDeclRange struct {
	keyword token.ID
	args    []withComma[id.Dyn[ExprAny, ExprKind]]
	options id.ID[CompactOptions]
	semi    token.ID
}

// DeclRangeArgs is arguments for [Context.NewDeclRange].
type DeclRangeArgs struct {
	Keyword   token.Token
	Options   CompactOptions
	Semicolon token.Token
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclRange) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindRange, id.ID[DeclAny](d.ID())))
}

// Keyword returns the keyword for this range.
func (d DeclRange) Keyword() keyword.Keyword {
	return d.KeywordToken().Keyword()
}

// KeywordToken returns the keyword token for this range.
func (d DeclRange) KeywordToken() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().keyword)
}

// IsExtensions checks whether this is an extension range.
func (d DeclRange) IsExtensions() bool {
	return d.Keyword() == keyword.Extensions
}

// IsReserved checks whether this is a reserved range.
func (d DeclRange) IsReserved() bool {
	return d.Keyword() == keyword.Reserved
}

// Ranges returns the sequence of expressions denoting the ranges in this
// range declaration.
func (d DeclRange) Ranges() Commas[ExprAny] {
	type slice = commas[ExprAny, id.Dyn[ExprAny, ExprKind]]
	if d.IsZero() {
		return slice{}
	}
	return slice{
		file: d.Context(),
		SliceInserter: seq.NewSliceInserter(
			&d.Raw().args,
			func(_ int, c withComma[id.Dyn[ExprAny, ExprKind]]) ExprAny {
				return id.WrapDyn(d.Context(), c.Value)
			},
			func(_ int, e ExprAny) withComma[id.Dyn[ExprAny, ExprKind]] {
				d.Context().Nodes().panicIfNotOurs(e)
				return withComma[id.Dyn[ExprAny, ExprKind]]{Value: e.ID()}
			},
		),
	}
}

// Options returns the compact options list for this range.
func (d DeclRange) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return id.Wrap(d.Context(), d.Raw().options)
}

// SetOptions sets the compact options list for this definition.
//
// Setting it to a nil Options clears it.
func (d DeclRange) SetOptions(opts CompactOptions) {
	d.Raw().options = opts.ID()
}

// Semicolon returns this range's ending semicolon.
//
// May be nil, if not present.
func (d DeclRange) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// Span implements [source.Spanner].
func (d DeclRange) Span() source.Span {
	r := d.Ranges()
	switch {
	case d.IsZero():
		return source.Span{}
	case r.Len() == 0:
		return source.Join(d.KeywordToken(), d.Semicolon(), d.Options())
	default:
		return source.Join(
			d.KeywordToken(), d.Semicolon(), d.Options(),
			r.At(0),
			r.At(r.Len()-1),
		)
	}
}
