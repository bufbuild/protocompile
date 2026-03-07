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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/just"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// lexer is a Protobuf parser.
type parser struct {
	*ast.Nodes
	*report.Report

	syntaxNode       ast.DeclSyntax
	importOptionNode ast.DeclImport
	syntax           syntax.Syntax
	parseComplete    bool
}

// classified is a spanner that has been classified by taxa.
type classified struct {
	source.Spanner
	what taxa.Noun
}

type punctParser struct {
	*parser
	c      *token.Cursor
	want   keyword.Keyword
	where  taxa.Place
	insert just.Kind
}

// parse attempts to unconditionally parse some punctuation.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a nil
// token instead. Returns a diagnostic on failure.
func (p punctParser) parse() (token.Token, report.Diagnose) {
	start := p.c.PeekSkippable().Span()
	start = start.File.Span(start.Start, start.Start)

	next := p.c.Peek()
	if next.Keyword() == p.want {
		return p.c.Next(), nil
	}

	wanted := taxa.Noun(p.want).AsSet()
	err := errtoken.Unexpected{
		What:  next,
		Where: p.where,
		Want:  wanted,
	}
	if next.IsZero() {
		end, span := p.c.SeekToEnd()
		err.What = span
		err.Got = taxa.EOF

		if _, c, _ := end.Keyword().Brackets(); c != keyword.Unknown {
			// Special case for closing braces.
			err.Got = "`" + c.String() + "`"
		} else if !end.IsZero() {
			err.Got = taxa.Classify(end)
		}
	}

	if p.insert != 0 {
		err.Stream = p.File().Stream()
		err.Insert = p.want.String()
		err.InsertAt = err.What.Span().Start
		err.InsertJustify = p.insert
	}

	return token.Zero, err
}

// parseEquals parses an equals sign.
//
// This is a shorthand for a very common version of punctParser.
func parseEquals(p *parser, c *token.Cursor, in taxa.Noun) (token.Token, report.Diagnose) {
	return punctParser{
		parser: p, c: c,
		want:   keyword.Assign,
		where:  in.In(),
		insert: just.Between,
	}.parse()
}

// parseSemi parses a semicolon.
//
// This is a shorthand for a very common version of punctParser.
func parseSemi(p *parser, c *token.Cursor, after taxa.Noun) (token.Token, report.Diagnose) {
	return punctParser{
		parser: p, c: c,
		want:   keyword.Semi,
		where:  after.After(),
		insert: just.Left,
	}.parse()
}
