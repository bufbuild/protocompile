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

	syntaxNode    ast.DeclSyntax
	syntax        syntax.Syntax
	parseComplete bool
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
	insert int // One of the justify* values.
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

	wanted := taxa.NewSet(taxa.Keyword(p.want))
	err := errUnexpected{
		what:  next,
		where: p.where,
		want:  wanted,
	}
	if next.IsZero() {
		end, span := p.c.SeekToEnd()
		err.what = span
		err.got = taxa.EOF

		if _, c, _ := end.Keyword().Brackets(); c != keyword.Unknown {
			// Special case for closing braces.
			err.got = "`" + c.String() + "`"
		} else if !end.IsZero() {
			err.got = taxa.Classify(end)
		}
	}

	if p.insert != 0 {
		err.stream = p.File().Stream()
		err.insert = p.want.String()
		err.insertAt = err.what.Span().Start
		err.insertJustify = p.insert
	}

	return token.Zero, err
}

// parseEquals parses an equals sign.
//
// This is a shorthand for a very common version of punctParser.
func parseEquals(p *parser, c *token.Cursor, in taxa.Noun) (token.Token, report.Diagnose) {
	return punctParser{
		parser: p, c: c,
		want:   keyword.Eq,
		where:  in.In(),
		insert: justifyBetween,
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
		insert: justifyLeft,
	}.parse()
}
