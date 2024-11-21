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
}

func (l *lexer) Push(length int, kind token.Kind) token.Token {
	l.count++
	return l.Stream.Push(length, kind)
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

func (l *lexer) Span(start, end int) report.Span {
	return report.Span{
		IndexedFile: l.IndexedFile,
		Start:       start,
		End:         end,
	}
}

func (l *lexer) SpanFrom(start int) report.Span {
	return l.Span(start, l.cursor)
}

// HandleICE is a defer-able method that will handle an "internal compiler error"
// during lexing. This annotates a panic with the lexer state for ease of
// debugging.
func (l *lexer) HandleICE() {
	if panicked := recover(); panicked != nil {
		panic(fmt.Sprintf("protocompile/parse: panic while lexing {cursor: %d, count: %d}: %v", l.cursor, l.count, panicked))
	}
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
