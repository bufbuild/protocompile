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

package fdp

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// commentTracker is used to track and attribute comments in a token stream. All attributed
// comments are stored in [commentTracker].attributed for easy look-up by [token.ID].
type commentTracker struct {
	cursor     *token.Cursor
	attributed map[token.ID]*comments // [token.ID] and its attributed comments.
	tracked    []paragraph

	current []token.Token
	prev    token.ID // The last non-skippable token.
	// The first line of the current comment tokens is on the same line as the last non-skippable token.
	firstCommentOnSameLine bool
}

// A paragraph is a group of comment and whitespace tokens that make up a single paragraph comment.
type paragraph []token.Token

// stringify returns the paragraph is a single string. It also trims off the leading "//"
// for line comments, and enclosing "/* */" for block comments.
func (p paragraph) stringify() string {
	var str strings.Builder
	for _, t := range p {
		text := t.Text()
		if t.Kind() == token.Comment {
			switch {
			case strings.HasPrefix(text, "//"):
				// For line comments, the leading "//" needs to be trimmed off.
				fmt.Fprint(&str, strings.TrimPrefix(text, "//"))
			case strings.HasPrefix(text, "/*"):
				// For block comments, we iterate through each line and trim the leading "/*",
				// "*", and "*/".
				for _, line := range strings.SplitAfter(text, "\n") {
					switch {
					case strings.HasPrefix(line, "/*"):
						fmt.Fprint(&str, strings.TrimPrefix(line, "/*"))
					case strings.HasSuffix(line, "*/"):
						fmt.Fprint(&str, strings.TrimSuffix(line, "*/"))
					case strings.HasPrefix(strings.TrimSpace(line), "*"):
						// We check the line with all spaces trimmed because of leading whitespace.
						fmt.Fprint(&str, strings.TrimPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "*"))
					}
				}
			}
		} else {
			fmt.Fprint(&str, text)
		}
	}
	return str.String()
}

// Comments are the leading, trailing, and detached comments associated with a token.
type comments struct {
	leading  paragraph
	trailing paragraph
	detached []paragraph
}

// leadingComment returns the leading comment string.
func (c comments) leadingComment() string {
	return c.leading.stringify()
}

// trailingComment returns the trailing comment string.
func (c comments) trailingComment() string {
	return c.trailing.stringify()
}

// detachedComments returns a slice of detached comment strings.
func (c comments) detachedComments() []string {
	detached := make([]string, len(c.detached))
	for i, paragraph := range c.detached {
		detached[i] = paragraph.stringify()
	}
	return detached
}

// attributeComments walks the given token stream and groups comment and space tokens
// into [paragraph]s and attributes them to non-skippable tokens as leading, trailing, and
// detached comments.
func (ct *commentTracker) attributeComments(cursor *token.Cursor) {
	ct.cursor = cursor
	t := cursor.NextSkippable()
	for !t.IsZero() {
		switch t.Kind() {
		case token.Comment:
			ct.handleCommentToken(t)
		case token.Space:
			ct.handleSpaceToken(t)
		default:
			ct.handleNonSkippableToken(t)
		}
		if !t.IsLeaf() {
			ct.attributeComments(t.Children())
			_, end := t.StartEnd()
			ct.handleNonSkippableToken(end)
			ct.cursor = cursor
		}
		t = cursor.NextSkippable()
	}
}

// handleCommentToken looks at the current comment [token.Token] and determines whether to
// start tracking a new comment paragraph or track it as part of an existing paragraph.
//
// For line comments, if it is on the same line as the previous non-skippable token, it is
// always considered its own paragraph.
//
// A block comment cannot be made into a paragraph with other tokens, so the currently
// tracked paragraph is closed out, and the block comment is also closed out as its own
// paragraph.
//
// The first comment token since the last non-skippable token is always tracked.
func (ct *commentTracker) handleCommentToken(t token.Token) {
	prev := id.Wrap(ct.cursor.Context(), ct.prev)
	isLineComment := strings.HasPrefix(t.Text(), "//")

	if !isLineComment {
		// Block comments are their own paragraph, close the current paragraph and track the
		// current block comment as its own paragraph.
		ct.closeParagraph()
		ct.current = append(ct.current, t)
		ct.closeParagraph()
		return
	}

	ct.current = append(ct.current, t)
	// If this is not the first comment in the current paragraph, move on.
	if len(ct.current) > 1 {
		return
	}

	if !prev.IsZero() && ct.cursor.NewLinesBetween(prev, t, 1) == 0 {
		// This first comment is always in a paragraph by itself if there are no newlines
		// between it and the previous non-skippable token.
		ct.closeParagraph()
		ct.firstCommentOnSameLine = true
	}
}

// handleSpaceToken looks at the current space [token.Token] and determines whether this
// space token is part of the current comment paragraph or if the current paragraph needs
// to be closed.
//
// If there are no currently tracked paragraphs, then the space token is thrown away,
// paragraphs are not started with space tokens.
//
// If the current space token is a newline, and is preceded by another token that ends with
// a newline, then the current paragraph is closed, and the current newline token is dropped.
// Otherwise, the newline token is attached to the current paragraph.
//
// All other space tokens are thrown away.
func (ct *commentTracker) handleSpaceToken(t token.Token) {
	if !strings.HasSuffix(t.Text(), "\n") || len(ct.current) == 0 {
		return
	}

	if strings.HasSuffix(ct.current[len(ct.current)-1].Text(), "\n") {
		ct.closeParagraph()
	} else {
		ct.current = append(ct.current, t)
	}
}

// handleNonSkippableToken looks at the current non-skippable [token.Token], closes out the
// currently tracked paragraph, and determines attributions for the tracked comment paragraphs.
//
// Comments are either attributed as leading or detached leading comments on the current
// token or as trailing comments on the last seen non-skippable token.
func (ct *commentTracker) handleNonSkippableToken(t token.Token) {
	ct.closeParagraph()
	prev := id.Wrap(ct.cursor.Context(), ct.prev)

	// Set new non-skippable token
	ct.prev = t.ID()

	if len(ct.tracked) == 0 {
		return
	}

	var donate bool // Donate the first tracked paragraph as a trailing comment to prev
	switch {
	case prev.IsZero():
		donate = false
	case ct.firstCommentOnSameLine:
		donate = true
		// Check if there are more than 2 newlines between the previous non-skippable token
		// and the first line of the first tracked paragraph.
	case ct.cursor.NewLinesBetween(prev, ct.tracked[0][0], 2) < 2:
		// If yes, check the remaining criteria for donation:
		//
		// 1. Is there more than one comment? If not, donate.
		// 2. Is the current token one of the closers, ), ], or } (but not >). If yes, donate
		//    the currently tracked paragraphs because a body is closed.
		// 3. Is there more than one newline between the current token and the end of the
		//    first tracked paragraph? If yes, donate.
		switch {
		case len(ct.tracked) > 1 && ct.tracked[1] != nil:
			donate = true
		case slicesx.Among(
			t.Text(),
			keyword.LParen.String(),
			keyword.LBracket.String(),
			keyword.LBrace.String(),
		):
			donate = true
		case ct.cursor.NewLinesBetween(ct.tracked[0][len(ct.tracked[0])-1], t, 2) > 1:
			donate = true
		}
	}

	if donate {
		ct.setTrailing(ct.tracked[0], prev)
		ct.tracked = ct.tracked[1:]
	}

	if len(ct.tracked) > 0 {
		// The leading comment must have precisely one new line between it and the current token.
		if last := ct.tracked[len(ct.tracked)-1]; ct.cursor.NewLinesBetween(last[len(last)-1], t, 2) == 1 {
			ct.setLeading(last, t)
			ct.tracked = ct.tracked[:len(ct.tracked)-1]
		}
	}

	// Check the remaining tracked comments to see if they are detached comments.
	// Detached comments must be separated from other non-space tokens by at least 2
	// newlines (unless they are at the top of the file), e.g. a file with contents:
	//
	// 	// This is a detached comment at the top of the file.
	//
	// 	 edition = "2023";
	//
	// 	message Foo {}
	// 	// This is neither a detached nor trailing comment, since it is not separated from
	// 	// the closing brace above by an empty line.
	//
	// 	// This IS a detached comment for Bar.
	//
	// 	// A leading comment for Bar.
	// 	message Bar {}
	//
	for i, remaining := range ct.tracked {
		prev := remaining[0].Prev()
		for prev.Kind() == token.Space {
			prev = prev.Prev()
		}
		next := remaining[len(remaining)-1].Next()
		for next.Kind() == token.Space {
			next = next.Next()
		}
		if !prev.IsZero() && ct.cursor.NewLinesBetween(prev, remaining[0], 2) < 2 {
			continue
		}
		if !next.IsZero() && ct.cursor.NewLinesBetween(remaining[len(remaining)-1], next, 2) == 2 {
			ct.setDetached(ct.tracked[i:], t)
			break
		}
	}
	// Reset tracked comment information
	ct.firstCommentOnSameLine = false
	ct.tracked = nil
}

// closeParagraph takes the currently tracked paragraph, closes it, and tracks it.
func (ct *commentTracker) closeParagraph() {
	// If the current paragraph only contains whitespace tokens, then throw it away.
	if slices.ContainsFunc(ct.current, func(t token.Token) bool {
		return t.Kind() == token.Comment
	}) {
		ct.tracked = append(ct.tracked, ct.current)
	}
	ct.current = nil
}

// setLeading sets the given paragraph as the leading comment on the given token.
func (ct *commentTracker) setLeading(leading paragraph, t token.Token) {
	ct.mutateComment(t, func(c *comments) {
		c.leading = leading
	})
}

// setTrailing sets the given paragraph as the trailing comment on the given token.
func (ct *commentTracker) setTrailing(trailing paragraph, t token.Token) {
	ct.mutateComment(t, func(c *comments) {
		c.trailing = trailing
	})
}

// setDetached sets the given slice of paragraphs as the detached comments on the given token.
func (ct *commentTracker) setDetached(detached []paragraph, t token.Token) {
	ct.mutateComment(t, func(c *comments) {
		c.detached = detached
	})
}

// mutateComment mutates the attributed comments on the given token.
func (ct *commentTracker) mutateComment(t token.Token, mutate func(*comments)) {
	if ct.attributed == nil {
		ct.attributed = make(map[token.ID]*comments)
	}

	if ct.attributed[t.ID()] == nil {
		ct.attributed[t.ID()] = &comments{}
	}
	mutate(ct.attributed[t.ID()])
}
