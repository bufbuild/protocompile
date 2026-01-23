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

package descriptor

import (
	"slices"
	"strings"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// commentTracker is used to track and attribute comments in a token stream. All attributed
// comments are stored in [commentTracker].donated for easy look-up by [token.ID].
type commentTracker struct {
	currentCursor *token.Cursor
	donated       map[token.ID]comments
	tracked       []paragraph

	current                []token.Token
	prev                   token.ID // The last non-skippable token.
	firstCommentOnSameLine bool
}

// A paragraph is a group of comment and whitespace tokens that make up a single paragraph comment.
type paragraph []token.Token

// Comments are the leading, trailing, and detached comments associated with a token.
type comments struct {
	leading  []token.Token
	trailing []token.Token
	detached []paragraph
}

// attributeComments walks the given token stream and groups comment and space tokens
// into [paragraph]s and "donates" them to non-skippable tokens as leading, trailing, and
// detached comments.
func (ct *commentTracker) attributeComments(cursor *token.Cursor) {
	ct.currentCursor = cursor
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
			ct.currentCursor = cursor
		}
		t = cursor.NextSkippable()
	}
}

// For comment tokens, we need to determine whether to start a new comment or track it as
// part of an existing comment.
//
// For line comments, we track whether or not it is on the same line as the previous
// non-skippable token. A line comment is always in a [paragraph] starting with itself if
// there are no newlines between it and the previous non-skippable tokens.
//
// A block comment cannot be made into a paragraph with other tokens, so we need to close
// out the [paragraph] we are currently tracking and track it as its own paragraph.
//
// We always track the first comment token since the last non-skippable token.
func (ct *commentTracker) handleCommentToken(t token.Token) {
	prev := id.Wrap(ct.currentCursor.Context(), ct.prev)
	isLineComment := strings.HasPrefix(t.Text(), "//")

	if !isLineComment {
		// Block comments are their own paragraph, so we close the current paragraph and track
		// the current block comment as its own paragraph.
		ct.closeParagraph()
		ct.current = append(ct.current, t)
		ct.closeParagraph()
		return
	}

	if ct.current == nil {
		ct.current = append(ct.current, t)

		if !prev.IsZero() && ct.newLinesBetween(prev, t, 1) == 0 {
			// This first comment is always in a paragraph by itself if there are no newlines
			// between it and the previous non-skippable token.
			ct.closeParagraph()
			ct.firstCommentOnSameLine = true
		}
		return
	}

	// Track the current comment token.
	ct.current = append(ct.current, t)
}

// For space tokens, we need to determine whether this space token is part of a comment or
// if it requires us to break the current paragraph.
//
// We first check if there are any tokens already being tracked as part of the paragraph.
// If not, then we do not start paragraphs with spaces, and the token is dropped.
//
// If a newline token is preceded by another token that ends with a newline, then we break
// the current paragraph and start a new one. Otherwise, we attach it to the current paragraph.
//
// We throw away all other space tokens.
func (ct *commentTracker) handleSpaceToken(t token.Token) {
	if strings.HasSuffix(t.Text(), "\n") && len(ct.current) > 0 {
		if strings.HasSuffix(ct.current[len(ct.current)-1].Text(), "\n") {
			ct.closeParagraph()
		} else {
			ct.current = append(ct.current, t)
		}
	}
}

// For non-skippable tokens, we first break off the current paragraph. We then determine
// where to donate currently tracked comments and reset currently tracked comments.
//
// Comments are either donated as leading or detached leading comments on the current token
// or as trailing comments on the last seen non-skippable token.
func (ct *commentTracker) handleNonSkippableToken(t token.Token) {
	ct.closeParagraph()
	prev := id.Wrap(ct.currentCursor.Context(), ct.prev)

	if len(ct.tracked) > 0 {
		var donate bool
		switch {
		case prev.IsZero():
			donate = false
		case ct.firstCommentOnSameLine:
			donate = true
		case ct.newLinesBetween(prev, ct.tracked[0][0], 2) < 2:
			// We check the remaining three criteria for donation if there are more than 2
			// newlines between the previous non-skippable token and the beginning of the first
			// currently tracked paragraph. These are:
			//
			// 1. Is there more than one comment? If not, donate.
			// 2. Is the current token one of the closers, ), ], or } (but not >). If yes, we
			//    donate the currently tracked paragraphs because a body is closed.
			// 3. Is there more than one newline between the current token and the end of the
			//    first tracked paragraph? If yes, donate.
			switch {
			case len(ct.tracked) > 1 && ct.tracked[1] != nil:
				donate = true
			case slices.Contains([]string{
				keyword.LParen.String(),
				keyword.LBracket.String(),
				keyword.LBrace.String(),
			}, t.Text()):
				donate = true
			case ct.newLinesBetween(ct.tracked[0][len(ct.tracked[0])-1], t, 2) > 1:
				donate = true
			}
		}

		if donate {
			ct.setTrailing(ct.tracked[0], prev)
			ct.tracked = ct.tracked[1:]
		}

		if len(ct.tracked) > 0 {
			// The leading comment must have precisely one new line between it and the current token.
			if last := ct.tracked[len(ct.tracked)-1]; ct.newLinesBetween(last[len(last)-1], t, 2) == 1 {
				ct.setLeading(last, t)
				ct.tracked = ct.tracked[:len(ct.tracked)-1]
			}
		}

		// Check the remaining tracked comments to see if they are detached comments.
		// Detached comments must be separated from other non-space tokens by at least 2
		// newlines (unless they are at the top of the file), e.g.
		//
		// // This is a detached comment at the top of the file.
		//
		//  edition = "2023";
		//
		// message Foo {}
		// // This is neither a detached nor trailing comment, since it is not separated from
		// // the closing brace above by an empty line.
		//
		// // This IS a detached comment for Bar.
		//
		// // A leading comment for Bar.
		// message Bar {}
		for i, remaining := range ct.tracked {
			prev := remaining[0].Prev()
			for prev.Kind() == token.Space {
				prev = prev.Prev()
			}
			next := remaining[len(remaining)-1].Next()
			for next.Kind() == token.Space {
				next = next.Next()
			}
			if prev.IsZero() || ct.newLinesBetween(prev, remaining[0], 2) == 2 {
				if !next.IsZero() && ct.newLinesBetween(remaining[len(remaining)-1], next, 2) == 2 {
					ct.setDetached(ct.tracked[i:], t)
					break
				}
			}
		}
		// Reset tracked comment information
		ct.firstCommentOnSameLine = false
		ct.tracked = nil
	}
	ct.prev = t.ID()
}

func (ct *commentTracker) closeParagraph() {
	// If the current paragraph only contains whitespace tokens, then we throw it away.
	var containsComment bool
	for _, t := range ct.current {
		if t.Kind() == token.Comment {
			containsComment = true
			break
		}
	}
	if containsComment {
		ct.tracked = append(ct.tracked, ct.current)
	}
	ct.current = nil
}

// newLinesBetween counts the number of \n characters between the end of [token.Token] a
// and the start of b, up to the limit.
//
// The final rune of a is included in this count, since comments may end in a \n rune.
func (ct *commentTracker) newLinesBetween(a, b token.Token, limit int) int {
	end := a.LeafSpan().End
	if end != 0 {
		// Account for the final rune of a
		end--
	}

	start := b.LeafSpan().Start
	between := ct.currentCursor.Context().Text()[end:start]

	var total int
	for total < limit {
		var found bool
		_, between, found = strings.Cut(between, "\n")
		if !found {
			break
		}

		total++
	}
	return total
}

func (ct *commentTracker) setLeading(leading paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.leading = leading
	})
}

func (ct *commentTracker) setTrailing(trailing paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.trailing = trailing
	})
}

func (ct *commentTracker) setDetached(detached []paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.detached = detached
	})
}

func (ct *commentTracker) mutateComment(t token.Token, cb func(*comments)) {
	if ct.donated == nil {
		ct.donated = make(map[token.ID]comments)
	}

	raw := ct.donated[t.ID()]
	cb(&raw)
	ct.donated[t.ID()] = raw
}
