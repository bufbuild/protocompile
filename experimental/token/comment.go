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

package token

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/internal/iter"
	"github.com/bufbuild/protocompile/internal/iters"
)

// Comments provides access to manipulating a [Token]'s attached comments.
//
// Comments are represented as tokens. When many line comments appear in
// together, they will be grouped into a paragraph. These comment paragraphs
// are represented as token trees. To obtain the contents of a comment,
// consistent with how Protobuf requires them to be formatted for SourceCodeInfo,
// use [Token.CommentLines].
//
// These functions are placed a separate struct to avoid cluttering [Token]'s
// method set.
type Comments struct {
	Token Token
}

type rawComments struct {
	// start and end are the range of all detached comments.
	start, end ID

	leading, trailing ID
}

// Detached returns an iterator over this token's detached comments.
func (c Comments) Detached() iter.Seq[Token] {
	s := c.Token.Context().Stream()
	raw := s.comments[c.Token.ID()]

	return func(yield func(Token) bool) {
		if raw.start.Nil() {
			return
		}

		c := NewCursor(
			raw.start.In(c.Token.Context()),
			raw.end.In(c.Token.Context()),
		)

		for { // Can't use Done(), that will skip comments.
			tok := c.PopSkippable()
			if tok.Nil() || (tok.Kind() == Comment && !yield(tok)) {
				break
			}
		}
	}
}

// SetDetachedRange sets the range containing this token's detached comments.
//
// Panics of t, start, or end is synthetic, if start and end are not both nil or
// non-nil, or the underlying stream has been frozen.
func (c Comments) SetDetachedRange(start, end Token) {
	if start.IsSynthetic() || end.IsSynthetic() {
		panic("protocompile/token: cannot use synthetic tokens in Comments.SetDetachedRange")
	}
	if start.Nil() != end.Nil() {
		panic("protocompile/token: both start/end passed to SetDetachedRange must be nil or non-nil")
	}

	fmt.Println("detached", start, end, c)

	c.mutate(func(raw *rawComments) {
		raw.start = start.ID()
		raw.end = end.ID()
	})
}

// Leading returns this token's leading comment, if it has one.
func (c Comments) Leading() Token {
	s := c.Token.Context().Stream()

	return s.comments[c.Token.ID()].leading.In(c.Token.Context())
}

// SetLeading sets the leading comment for this token.
//
// Panics of t is synthetic or the underlying stream has been frozen.
func (c Comments) SetLeading(comment Token) {
	fmt.Println("leading", comment, c)

	c.mutate(func(raw *rawComments) {
		raw.leading = comment.ID()
	})
}

// Trailing returns this token's trailing comment, if it has one.
func (c Comments) Trailing() Token {
	s := c.Token.Context().Stream()
	return s.comments[c.Token.ID()].trailing.In(c.Token.Context())
}

// SetTrailing sets the leading comment for this token.
//
// Panics of t is synthetic or the underlying stream has been frozen.
func (c Comments) SetTrailing(comment Token) {
	fmt.Println("trailing", comment, c)

	c.mutate(func(raw *rawComments) {
		raw.trailing = comment.ID()
	})
}

// mutate performs a mutation on the comments struct for this token.
func (c Comments) mutate(cb func(*rawComments)) {
	s := c.Token.Context().Stream()
	if c.Token.IsSynthetic() {
		panic("protocompile/token: modifying comments on a synthetic token is not yet implemented")
	}

	if !c.Token.IsSynthetic() {
		s.mustNotBeFrozen()
	}

	if s.comments == nil {
		s.comments = make(map[ID]rawComments)
	}

	// My kingdom for maps.Upsert.
	raw := s.comments[c.Token.id]
	cb(&raw)
	s.comments[c.Token.id] = raw
}

type commentFormatter struct {
	lines  []string
	margin string
	quirks bool
}

func (c *commentFormatter) appendComment(text string) {
	if text, ok := strings.CutPrefix(text, "//"); ok {
		// Pull off a trailing newline. A comment just before EOF will not
		// have one.
		text = strings.TrimSuffix(text, "\n")

		// If not in quirks mode, we want to pull a uniform amount of comment
		// margin off of each line in this paragraph. We detect this to be the
		// whitespace prefix of the first line.
		if !c.quirks {
			if c.margin == "" {
				c.margin, text = trimSpace(text)
			} else {
				text = strings.TrimPrefix(text, c.margin)
			}
		}

		c.lines = append(c.lines, text)
		return
	}

	// This is a block comment. We need to remove the /**/, and for each
	// line beyond the first, we must also remove leading whitespace and a
	// *.
	text = strings.TrimPrefix(strings.TrimSuffix(text, "*/"), "/*")

	// First, append all of the lines in the comment without modification.
	start := len(c.lines)
	c.lines = iters.AppendSeq(c.lines, iters.SplitString(text, "\n"))
	lines := c.lines[start:]

	// When in quirks mode, all we need to do is strip whitespace and the
	// asetrisk.
	if c.quirks {
		for i, line := range lines[1:] {
			_, line = trimSpace(line)
			line = strings.TrimPrefix(line, "*")

			lines[i+1] = line
		}
		return
	}

	// Otherwise, we only want to remove the same amount of space for each
	// line, *and* we want to only remove asterisks if every line other than
	// the first has them.
	var margin string
	haveStars := true
	for i, line := range lines[1:] {
		if margin == "" {
			margin, line = trimSpace(line)
		} else {
			line = strings.TrimPrefix(line, margin)
		}

		if !strings.HasPrefix(line, "*") {
			haveStars = false
		}

		lines[i+1] = line
	}

	// Now we can remove the asterisks. Note that we remove an asterisk
	// from *all* lines, because many comment styles have a leading /**.
	//
	// TODO: for single-line block comments, we may want to handle Doxygen's
	// /*< xyz */ comments in the future.
	if haveStars {
		var margin string
		for i, line := range lines {
			line = strings.TrimPrefix(line, "*")

			// *Also* remove margin after the asterisk!
			if margin == "" {
				margin, line = trimSpace(line)
			} else {
				line = strings.TrimPrefix(line, margin)
			}

			lines[i] = line
		}
	}
}

func trimSpace(s string) (space, rest string) {
	suffix := strings.TrimLeftFunc(s, func(r rune) bool {
		return unicode.Is(unicode.Pattern_White_Space, r)
	})
	return s[:len(s)-len(suffix)], suffix
}
