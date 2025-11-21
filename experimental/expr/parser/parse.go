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

package parser

import (
	"github.com/bufbuild/protocompile/experimental/expr"
	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/just"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Parse greedily parses an expression at c, writing diagnostics to r as it
// goes, and returns it.
//
// context and c must share the same token.Stream. This function will not
// set up ICE catching; the caller is expected to do that.
func Parse(context *expr.Context, c *token.Cursor, r *report.Report) expr.Expr {
	p := &parser{
		Nodes:  context.Nodes(),
		Report: r,
	}
	return p.expr(c)
}

// parser is the actual parser struct. Most of the action takes place in
// [parser.expr].
type parser struct {
	*expr.Nodes
	*report.Report
}

// expr parses an expression at c.
func (p *parser) expr(c *token.Cursor) expr.Expr {
	if c.Done() {
		return p.eof(c)
	}

}

// terminal parses an expression that is grammatically equivalent to an
// identifier: replacing it with an arbitrary identifier will not change the
// resulting AST beyond replacing this expression.
func (p *parser) terminal(c *token.Cursor) expr.Expr {
	if c.Done() {
		return p.eof(c)
	}

	next := c.Next()
	switch next.Keyword() {
	case keyword.If:
		args := expr.IfArgs{
			If: next,
		}

		// Try parsing an expression as the condition.
		cond := p.expr(c)

	case keyword.For:
	case keyword.Switch:
	}

	switch next.Kind() {
	case token.Number, token.String:
	}
}

// block parses a block expression, i.e., braces containing a sequence of
// expressions.
func (p *parser) block(c *token.Cursor) expr.Block {

}

// eof "parses" an eof, generating a diagnostic and returning an error expression.
func (p *parser) eof(c *token.Cursor) expr.Expr {
	err := errtoken.UnexpectedEOF(c, taxa.Expr.In())
	err.Want = taxa.Expr.AsSet()
	p.Error(err)
	return p.NewError(err.What.Span()).AsAny()
}

// keyword attempts to unconditionally parse a keyword.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a
// zero token instead. Returns a diagnostic on failure, which can be fed into
// a report.
func (p *parser) tryKeyword(
	c *token.Cursor, want keyword.Keyword,
	where taxa.Place, justify just.Kind,
) (token.Token, report.Diagnose) {
	start := c.PeekSkippable().Span()
	start = start.File.Span(start.Start, start.Start)

	next := c.Peek()
	if next.Keyword() == want {
		return c.Next(), nil
	}

	wanted := taxa.Noun(want).AsSet()
	err := errtoken.Unexpected{
		What:  next,
		Where: where,
		Want:  wanted,
	}
	if next.IsZero() {
		end, span := c.SeekToEnd()
		err.What = span
		err.Got = taxa.EOF

		if _, c, _ := end.Keyword().Brackets(); c != keyword.Unknown {
			// Special case for closing braces.
			err.Got = "`" + c.String() + "`"
		} else if !end.IsZero() {
			err.Got = taxa.Classify(end)
		}
	}

	if justify != just.None {
		err.Stream = p.Context().Stream()
		err.Insert = want.String()
		err.InsertAt = err.What.Span().Start
		err.InsertJustify = justify
	}

	return token.Zero, err
}
