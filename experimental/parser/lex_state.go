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
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// lexer is a Protobuf lexer.
type lexer struct {
	token.Context
	*token.Stream // Embedded so we don't have to call Stream() everywhere.
	*report.Report

	cursor, count int

	braces []token.ID

	prev token.ID // The last non-skippable token.

	firstCommentSincePrev  token.ID
	firstCommentOnSameLine bool
	parStart, parEnd       token.ID
}

func (l *lexer) Cursor() int {
	return l.cursor
}

// Done returns whether or not we're done lexing runes.
func (l *lexer) Done() bool {
	return l.Rest() == ""
}

// Rest returns unlexed text.
func (l *lexer) Rest() string {
	return l.Text()[l.cursor:]
}

// Peek peeks the next character.
//
// Returns -1 if l.Done().
func (l *lexer) Peek() rune {
	return decodeRune(l.Rest())
}

// Pop consumes the next character.
//
// Returns -1 if l.Done().
func (l *lexer) Pop() rune {
	r := l.Peek()
	if r != -1 {
		l.cursor += utf8.RuneLen(r)
		return r
	}
	return -1
}

// TakeWhile consumes the characters while they match the given function.
// Returns consumed characters.
func (l *lexer) TakeWhile(f func(rune) bool) string {
	start := l.cursor
	for !l.Done() {
		r := l.Peek()
		if r == -1 || !f(r) {
			break
		}
		_ = l.Pop()
	}
	return l.Text()[start:l.cursor]
}

// SeekInclusive seek until the given needle is found; returns the prefix inclusive that
// needle, and updates the cursor to point after it.
func (l *lexer) SeekInclusive(needle string) (string, bool) {
	if idx := strings.Index(l.Rest(), needle); idx != -1 {
		prefix := l.Rest()[:idx+len(needle)]
		l.cursor += idx + len(needle)
		return prefix, true
	}
	return "", false
}

// SeekEOF seeks the cursor to the end of the file and returns the remaining text.
func (l *lexer) SeekEOF() string {
	rest := l.Rest()
	l.cursor += len(rest)
	return rest
}

func (l *lexer) SpanFrom(start int) report.Span {
	return l.Span(start, l.cursor)
}

func (l *lexer) Push(length int, kind token.Kind) token.Token {
	l.count++
	prev := l.prev.In(l.Context)
	tok := l.Stream.Push(length, kind)
	// NOTE: tok will have the Stream rather than l.Context as its context,
	// which will cause issues when we call NewCursor below.
	tok = tok.ID().In(l.Context)

	// NOTE: For the purposes of attributing comments, we need to know what line
	// certain offsets are at. Although we could track this as we advance cursor,
	// we instead use other methods to determine if two tokens are on the same
	// line. This is for a couple of reasons.
	//
	// 1. Getting a line number from the line index is O(log n), but we can
	//    instead use strings.Index and friends in some places without going
	//    quadratic.
	//
	// 2. Having to examine every character directly locks us out of using e.g.
	//    strings.Index for certain operations, which is much more efficient
	//    than the naive for loop.

	switch {
	case tok.Kind() == token.Comment:
		isLineComment := strings.HasPrefix(tok.Text(), "//")

		if l.firstCommentSincePrev.Nil() {
			l.firstCommentSincePrev = tok.ID()

			if !prev.Nil() && l.newLinesBetween(prev, tok, 1) == 0 {
				// The first comment is always in a paragraph by itself if there
				// is no newline between it and the comment start.
				l.firstCommentOnSameLine = true
				break
			}
		}

		if !isLineComment {
			// Block comments cannot be made into paragraphs, so we must
			// interrupt the current paragraph.
			l.fuseParagraph()
			break
		}

		// Start building up a line comment paragraph if there isn't one
		// currently.
		if l.parStart.Nil() {
			l.parStart = tok.ID()
		}
		l.parEnd = tok.ID()

	case tok.Kind() == token.Space:
		// Note that line comments contain their newlines, except for a line
		// comment at the end of the file. Thus, seeing a single new line
		// means that if we are interrupting a line comment paragraph, and thus
		// we must fuse the current paragraph.
		if strings.Contains(tok.Text(), "\n") {
			l.fuseParagraph()
		}

	default:
		l.fuseParagraph()
		//nolint:dupword // False positive due to comments describing an algorithm.
		if !l.firstCommentSincePrev.Nil() {
			fmt.Println(l.firstCommentSincePrev.In(l.Context), tok)
			comments := token.NewCursor(l.firstCommentSincePrev.In(l.Context), tok)
			var first, second, penultimate, last token.Token
			for { // Don't use l.Done() here, that tosses comment tokens.
				next := comments.PopSkippable()
				if next.Nil() {
					break
				} else if next.Kind() == token.Comment {
					switch {
					case first.Nil():
						first = next
					case second.Nil():
						second = next
					}
					penultimate = last
					last = next
				}
			}
			fmt.Println(first, second, penultimate, last)

			// Determine if we need to donate first to the previous comment.
			var donate bool
			switch {
			case prev.Nil():
				donate = false
			case l.firstCommentOnSameLine:
				donate = true
			case l.newLinesBetween(prev, first, 2) < 2:
				// Now we need to check the remaining three criteria for
				// donate. These are:
				//
				// 1. Is there more than one comment.
				// 2. Is the token one of the closers ), ], or } (but not
				//    >).
				// 3. The line of the current token minus the end line of
				//    the first comment is greater than one.
				switch {
				case !second.Nil():
					donate = true
				case slices.Contains([]string{")", "]", "}"}, tok.Text()):
					donate = true
				case l.newLinesBetween(first, tok, 2) > 1:
					donate = true
				}
			}

			if donate {
				prev.Comments().SetTrailing(first)
				first = second
			}

			// The leading comment must have precisely one newline between
			// it and the new token.
			if !first.Nil() && !last.Nil() && l.newLinesBetween(last, tok, 2) == 1 {
				tok.Comments().SetLeading(last)
				last = penultimate
			}

			// Check if we have any detached comments left. This is the case
			// when first and last are both non-nil and <=. If we donated the
			// only comment, second will have been nil, so first is now nil.
			//
			// If we attached the only remaining comment after donating a
			// comment, we would have had the following value evolution for
			// first, second, penultimate and last:
			//
			//   before donate: a, b, a, b
			//   after donate: b, b, a, b
			//   after attach: b, b, a, a
			//
			// Thus, when we check b < a, we find that we have nothing left to
			// attach.
			if !first.Nil() && !last.Nil() && first.ID() <= last.ID() {
				tok.Comments().SetDetachedRange(first, last)
			}

			l.firstCommentSincePrev = 0
			l.firstCommentOnSameLine = false
		}

		l.prev = tok.ID()
	}
	return tok
}

func (l *lexer) fuseParagraph() {
	if !l.parStart.Nil() && l.parEnd != l.parStart {
		token.Fuse(
			l.parStart.In(l.Context),
			l.parEnd.In(l.Context),
		)
	}
	l.parStart = 0
}

// newLinesBetween counts the number of \n characters between the end of a
// and the start of b, up to max.
//
// The final rune of a is included in this count, since comments may end in a
// \n rune.
//
//nolint:revive,predeclared // Complains about redefining max.
func (l *lexer) newLinesBetween(a, b token.Token, max int) int {
	end := a.Span().End
	if end != 0 {
		// Account for the final rune of a.
		end--
	}

	start := b.Span().Start
	between := l.Text()[end:start]

	var total int
	for total < max {
		var found bool
		_, between, found = strings.Cut(between, "\n")
		if !found {
			break
		}

		total++
	}
	return total
}

// mustProgress returns a progress checker for this lexer.
func (l *lexer) mustProgress() mustProgress {
	return mustProgress{l, -1}
}

// mustProgress is a helper for ensuring that the lexer makes progress
// in each loop iteration. This is intended for turning infinite loops into
// panics.
type mustProgress struct {
	l    *lexer
	prev int
}

// check panics if the lexer has not produced new tokens since it was last
// called.
func (mp *mustProgress) check() {
	if mp.prev == mp.l.count {
		// NOTE: no need to annotate this panic; it will get wrapped in the
		// call to HandleICE for us.
		panic("lexer failed to make progress; this is a bug in protocompile")
	}
	mp.prev = mp.l.count
}

// decodeRune is a wrapper around utf8.DecodeRuneInString that makes it easier
// to check for failure. Instead of returning RuneError (which is a valid rune!),
// it returns -1.
//
// The success conditions for DecodeRune are kind of subtle; this makes
// sure we get the logic right every time. It is somewhat annoying that
// Go did not chose to make this easier to inspect.
func decodeRune(s string) rune {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && n < 2 {
		return -1
	}
	return r
}
