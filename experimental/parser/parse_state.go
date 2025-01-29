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

	parseComplete bool

	syntax     ast.DeclSyntax
	cachedMode taxa.Noun
}

// Mode returns whether or not the parser believes it is in editions
// mode. This function must not be called until AST construction is complete
// and legalization begins.
//
// This function will return the same answer every time it is called. This is to
// avoid diagnostics depending on where the editions keyword appears. For
// example, consider:
//
//	message Foo {
//	  reserved foo;
//	}
//
//	edition = "2023";
//
//	message Bar {
//	  reserved "foo";
//	}
//
// If we only referenced p.syntax, we get into a situation where we diagnose
// *both* reserved ranges, rather than just the one in Foo, which is potentially
// confusing, and suggests that the order of declarations in Protobuf is
// semantically meaningful.
func (p *parser) Mode() taxa.Noun {
	if !p.parseComplete {
		panic("called parser.Mode() outside of the legalizer; this is a bug")
	}

	if p.cachedMode == taxa.Unknown {
		p.cachedMode = taxa.SyntaxMode
		if !p.syntax.IsZero() && p.syntax.IsEdition() {
			p.cachedMode = taxa.EditionMode
		}
	}

	return p.cachedMode
}

// classified is a spanner that has been classified by taxa.
type classified struct {
	report.Spanner
	what taxa.Noun
}

// parsePunct attempts to unconditionally parse some punctuation.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a nil
// token instead. Returns a diagnostic on failure.
func (p *parser) Punct(c *token.Cursor, want string, where taxa.Place) (token.Token, report.Diagnose) {
	next := c.Peek()
	if next.Text() == want {
		return c.Next(), nil
	}

	wanted := taxa.NewSet(taxa.Punct(want, false))
	if next.IsZero() {
		tok, span := c.SeekToEnd()
		err := errUnexpected{
			what:  span,
			where: where,
			want:  wanted,
			got:   taxa.EOF,
		}
		if !tok.IsZero() {
			err.got = taxa.Classify(tok)
		}

		return token.Zero, err
	}

	return token.Zero, errUnexpected{
		what:  next,
		where: where,
		want:  wanted,
	}
}
