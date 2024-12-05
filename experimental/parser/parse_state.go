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

// parsePunct attempts to unconditionally parse some punctuation.
//
// If the wrong token is encountered, it DOES NOT consume the token, returning a nil
// token instead. Returns a diagnostic on failure.
func (p *parser) Punct(c *token.Cursor, want string, where taxa.Place) (token.Token, report.Diagnose) {
	next := c.Peek()
	if next.Text() == want {
		return c.Pop(), nil
	}

	wanted := taxa.NewSet(taxa.Punct(want, false))
	if next.Nil() {
		tok, span := c.JustAfter()
		err := errUnexpected{
			what:  span,
			where: where,
			want:  wanted,
			got:   taxa.EOF,
		}
		if !tok.Nil() {
			err.got = taxa.Classify(tok)
		}

		return token.Nil, err
	}

	return token.Nil, errUnexpected{
		what:  next,
		where: where,
		want:  wanted,
	}
}

// commas returns an iterator over a comma-delimited list of things out of cursor.
// This automatically handles various corner-cases around commas that occur throughout the
// grammar.
//
// This will completely drain cursor, unless the parse function returns false, which signals
// that the end of the list has been reached.
func commas[T any](
	_ *parser,
	c *token.Cursor,
	commasRequired bool,
	parse func(*token.Cursor) (T, bool),
) func(func(T, token.Token) bool) {
	return func(yield func(T, token.Token) bool) {
		for !c.Done() {
			result, ok := parse(c)

			// Check for a trailing comma.
			var comma token.Token
			if next := c.Peek(); next.Text() == "," {
				c.Pop()
				comma = next
			}

			if !ok || !yield(result, comma) {
				break
			}

			if commasRequired && comma.Nil() {
				break
			}
		}
	}
}

// ensureProgress is used to make sure that the parser makes progress on each
// loop iteration. See mustProgress in lex_state.go for the lexer equivalent.
func ensureProgress(c *token.Cursor, m *token.CursorMark) {
	next := c.Mark()
	if *m == next {
		panic("protocompile/parser: parser failed to make progress; this is a bug in protocompile")
	}
	*m = next
}
