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

package lexer

import (
	"math"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// MaxFileSize is the maximum file size the lexer supports.
const MaxFileSize int = math.MaxInt32 // 2GB

// OnKeyword is an action to take in response to a [Lexer] encountering a
// [keyword.Keyword].
type OnKeyword int

const (
	// If the keyword is punctuation, reject it; if it's a reserved word, treat
	// it as an identifier.
	DiscardKeyword OnKeyword = iota

	// Accept the keyword as a keyword token.
	KeepKeyword

	// Accept the keyword, and treat it as an open brace. It must be one of
	// the open brace keywords.
	BracketKeyword

	// Treat the keyword as starting a line comment through to the next newline.
	LineComment

	// Treat the keyword as starting a block comment. It must be one of the
	// open brace keywords.
	BlockComment
)

// Lexer is the general-purpose lexer exposed by this file.
type Lexer struct {
	// How to handle a known keyword when encountered by the lexer.
	OnKeyword func(keyword.Keyword) OnKeyword

	// Used for validating prefixes and suffixes of strings and numbers.
	IsAffix func(affix string, kind token.Kind, suffix bool) bool

	// If true, a dot immediately followed by a digit is taken to begin a
	// digit.
	NumberCanStartWithDot bool

	// If true, decimal numbers starting with 0 are treated as octal instead.
	OldStyleOctal bool

	// If true, diagnostics are emitted for non-ASCII identifiers.
	RequireASCIIIdent bool

	EscapeExtended        bool // Escapes \a, \b, \f, and \v.
	EscapeAsk             bool // Escape \?.
	EscapeOctal           bool // Octal escapes other than \0
	EscapePartialX        bool // Partial \xN escapes.
	EscapeUppercaseX      bool // The unusual \XNN escape.
	EscapeOldStyleUnicode bool // Old-style Unicode escapes \uXXXX and \UXXXXXXXX.
}

// Lex runs lexical analysis on file and returns a new token stream as a result.
func (l *Lexer) Lex(file *source.File, r *report.Report) *token.Stream {
	stream := &token.Stream{File: file}
	loop(&lexer{Lexer: l, Stream: stream, Report: r})
	return stream
}

// lexer is the actual lexer book-keeping used in this package.
type lexer struct {
	*Lexer
	*token.Stream
	*report.Report

	cursor, count int
	braces        []token.ID
	scratch       []byte
	scratchFloat  *big.Float

	// Used for determining longest runs of unrecognized tokens.
	badBytes int
}

// push pushes a new token onto the stream the lexer is building.
func (l *lexer) push(length int, kind token.Kind) token.Token {
	if l.badBytes > 0 {
		l.count++
		tok := l.Stream.Push(l.badBytes, token.Unrecognized)
		l.badBytes = 0

		l.Errorf("unrecognized token").Apply(
			report.Snippet(tok),
		)
	}

	l.count++
	return l.Stream.Push(length, kind)
}

// rest returns the remaining unlexed text.
func (l *lexer) rest() string {
	return l.Text()[l.cursor:]
}

// done returns whether or not we're done lexing runes.
func (l *lexer) done() bool {
	return l.rest() == ""
}

// peek peeks the next character.
//
// Returns -1 if l.done().
func (l *lexer) peek() rune {
	r, ok := stringsx.Rune(l.rest(), 0)
	if !ok {
		return -1
	}
	return r
}

// pop consumes the next character.
//
// Returns -1 if l.done().
func (l *lexer) pop() rune {
	r := l.peek()
	if r != -1 {
		l.cursor += utf8.RuneLen(r)
		return r
	}
	return -1
}

// takeWhile consumes the characters while they match the given function.
// Returns consumed characters.
func (l *lexer) takeWhile(f func(rune) bool) string {
	start := l.cursor
	for !l.done() {
		r := l.peek()
		if r == -1 || !f(r) {
			break
		}
		_ = l.pop()
	}
	return l.Text()[start:l.cursor]
}

// seekInclusive seek until the given needle is found; returns the prefix inclusive that
// needle, and updates the cursor to point after it.
func (l *lexer) seekInclusive(needle string) (string, bool) {
	if idx := strings.Index(l.rest(), needle); idx != -1 {
		prefix := l.rest()[:idx+len(needle)]
		l.cursor += idx + len(needle)
		return prefix, true
	}
	return "", false
}

// seekEOF seeks the cursor to the end of the file and returns the remaining text.
func (l *lexer) seekEOF() string {
	rest := l.rest()
	l.cursor += len(rest)
	return rest
}

func (l *lexer) spanFrom(start int) source.Span {
	return l.Span(start, l.cursor)
}

// mustProgress returns a progress checker for this lexer.
func (l *lexer) mustProgress() mustProgress {
	return mustProgress{l, -1}
}
