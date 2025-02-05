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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// lexer is a Protobuf parser.
type parser struct {
	ast.Context
	*ast.Nodes
	*report.Report
}

type punctParser struct {
	*parser
	c      *token.Cursor
	want   string
	where  taxa.Place
	insert report.Justify
}

// parse attempts to unconditionally parse some punctuation.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a nil
// token instead. Returns a diagnostic on failure.
func (p punctParser) parse() (token.Token, report.Diagnose) {
	start := p.c.PeekSkippable().Span()
	start = start.File.Span(start.Start, start.Start)

	next := p.c.Peek()
	if next.Text() == p.want {
		return p.c.Next(), nil
	}

	err := errUnexpected{
		where: p.where,
		want:  taxa.NewSet(taxa.Punct(p.want, false)),
	}
	if next.IsZero() {
		tok, span := p.c.SeekToEnd()
		err.what = span
		err.got = taxa.EOF
		if !tok.IsZero() {
			err.got = taxa.Classify(tok)
		}

		return token.Zero, err
	} else {
		err.what = next
	}

	if p.insert != 0 {
		err.insert = p.want
		err.insertAt = err.what.Span().Start
		err.insertJustify = p.insert
	}

	return token.Zero, err
}
