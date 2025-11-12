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
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// lexer is a Protobuf lexer.
type lexer struct {
	*token.Stream
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
	r, ok := stringsx.Rune(l.Rest(), 0)
	if !ok {
		return -1
	}
	return r
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

func (l *lexer) SpanFrom(start int) source.Span {
	return l.Span(start, l.cursor)
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
