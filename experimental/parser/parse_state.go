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
	"github.com/bufbuild/protocompile/experimental/token/keyword"
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
func (p *parser) Punct(c *token.Cursor, want keyword.Keyword, where taxa.Place) (token.Token, report.Diagnose) {
	next := c.Peek()
	if next.Keyword() == want {
		return c.Next(), nil
	}

	wanted := taxa.NewSet(taxa.Keyword(want))
	if next.IsZero() {
		end, span := c.SeekToEnd()
		err := errUnexpected{
			what:  span,
			where: where,
			want:  wanted,
			got:   taxa.EOF,
		}
		if _, close, ok := end.Keyword().OpenClose(); ok {
			// Special case for closing braces.
			err.got = "`" + close + "`"
		} else if !end.IsZero() {
			err.got = taxa.Classify(end)
		}

		return token.Zero, err
	}

	return token.Zero, errUnexpected{
		what:  next,
		where: where,
		want:  wanted,
	}
}
