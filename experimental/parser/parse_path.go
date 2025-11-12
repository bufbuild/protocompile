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
	"fmt"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/astx"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// parsePath parses the longest path at cursor. Returns a nil path if
// the next token is neither an identifier, a dot, or a ().
//
// If an invalid token occurs after a dot, returns the longest path up until that dot.
// The cursor is then placed after the dot.
//
// This function assumes that we have decided to definitely parse a path, and
// will emit diagnostics to that effect. As such, the current token position on cursor
// should not be nil.
func parsePath(p *parser, c *token.Cursor) ast.Path {
	start := c.Peek()
	if !canStartPath(start) {
		p.Error(errUnexpected{what: start, want: startsPath})
		return ast.Path{}
	}

	var prevSeparator token.Token
	if slicesx.Among(start.Keyword(), keyword.Dot, keyword.Slash) {
		prevSeparator = c.Next()
	}

	var done bool
	end := start
	for !done && !c.Done() {
		next := c.Peek()
		first := start == next

		switch {
		case slicesx.Among(next.Keyword(), keyword.Dot, keyword.Slash):
			if !prevSeparator.IsZero() {
				// This is a double dot, so something like foo..bar, ..foo, or
				// foo.. We diagnose it and move on -- Path.Components is robust
				// against double dots.

				// We consume additional separators here so that we can diagnose
				// them all in one shot.
				for {
					prevSeparator = c.Next()
					next := c.Peek()
					if !slicesx.Among(next.Text(), ".", "/") {
						break
					}
				}

				tokens := source.Join(next, prevSeparator)
				p.Error(errUnexpected{
					what:  tokens,
					where: taxa.Classify(next).After(),
					want:  taxa.NewSet(taxa.Ident, taxa.Parens),
					got:   "tokens",
				})
			} else {
				prevSeparator = c.Next()
			}

		case next.Kind() == token.Ident:
			if !first && prevSeparator.IsZero() {
				// This means we found something like `foo bar`, which means we
				// should stop consuming components.
				done = true
				continue
			}

			end = next
			prevSeparator = token.Zero
			c.Next()

		case next.Keyword() == keyword.Parens:
			if !first && prevSeparator.IsZero() {
				// This means we found something like `foo(bar)`, which means we
				// should stop consuming components.
				done = true
				continue
			}

			// Recurse into this token and check it, too, contains a path. We throw
			// the result away once we're done, because we don't need to store it;
			// a Path simply stores its start and end tokens and knows how to
			// recurse into extensions. We also need to check there are no
			// extraneous tokens.
			contents := next.Children()
			parsePath(p, contents)
			if tok := contents.Peek(); !tok.IsZero() {
				p.Error(errUnexpected{
					what:  start,
					where: taxa.ExtensionName.After(),
				})
			}

			end = next
			prevSeparator = token.Zero
			c.Next()

		default:
			if prevSeparator.IsZero() {
				// This means we found something like `foo =`, which means we
				// should stop consuming components.
				done = true
				continue
			}

			// This means we found something like foo.1 or bar."xyz" or bar.[...].
			// TODO: Do smarter recovery here. Generally speaking it's likely we should *not*
			// consume this token.
			p.Error(errUnexpected{
				what:  next,
				where: taxa.QualifiedName.After(),
				want:  taxa.NewSet(taxa.Ident, taxa.Parens),
			}).Apply(report.SuggestEdits(
				prevSeparator,
				fmt.Sprintf("delete the extra `%s`", prevSeparator.Text()),
				report.Edit{Start: 0, End: 1},
			))

			end = prevSeparator // Include the trailing separator.
			done = true
		}
	}

	// NOTE: We do not need to legalize against a single-dot path; that
	// is already done for us by the if nextDot checks.

	return astx.NewPath(p.File(), start, end)
}
