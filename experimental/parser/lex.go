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
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Lex performs lexical analysis on the file contained in ctx, and appends any
// diagnostics that results in to l.
func Lex(ctx token.Context, errs *report.Report) {
	l := &lexer{
		Context: ctx,
		Stream:  ctx.Stream(),
		Report:  errs,
	}

	defer l.HandleICE()

	// Check that the file isn't too big. We give up immediately if that's
	// the case.
	if len(l.Text()) > MaxFileSize {
		l.Error(ErrFileTooBig{Path: l.Path()})
		return
	}

	// Also check that the text of the file is actually UTF-8.
	// We go rune by rune to find the first invalid offset.
	for text := l.Text(); text != ""; {
		r := decodeRune(text)
		if r == -1 {
			l.Error(ErrNotUTF8{
				Path: l.Path(),
				At:   len(l.Text()) - len(text),
				Byte: text[0],
			})
			return
		}
		text = text[utf8.RuneLen(r):]
	}

	// This is the main loop of the lexer. Each iteration will examine the next
	// rune in the source file to determine what action to take.
	mp := l.mustProgress()
	for !l.Done() {
		mp.check()

		start := l.cursor
		r := l.Pop()

		switch {
		case unicode.In(r, unicode.Pattern_White_Space):
			// Whitepace. Consume as much whitespace as possible and mint a
			// whitespace token.
			l.TakeWhile(func(r rune) bool {
				return unicode.In(r, unicode.Pattern_White_Space)
			})
			l.Push(l.cursor-start, token.Space)

		case r == '/' && l.Peek() == '/':
			l.cursor++ // Skip the second /.

			// Single-line comment. Seek to the next '\n' or the EOF.
			var text string
			if comment, ok := l.SeekInclusive("\n"); ok {
				text = comment
			} else {
				text = l.SeekEOF()
			}
			l.Push(len("//")+len(text), token.Comment)
		case r == '/' && l.Peek() == '*':
			l.cursor++ // Skip the *.

			// Block comment. Seek to the next "*/". Protobuf comments
			// unfortunately do not nest, and allowing them to nest can't
			// be done in a backwards-compatible manner. We acknowledge that
			// this behavior is user-hostile.
			//
			// If we encounter no "*/", seek EOF and emit a diagnostic. Trying
			// to lex a partial comment is hopeless.

			var text string
			if comment, ok := l.SeekInclusive("*/"); ok {
				text = comment
			} else {
				// Create a span for the /*, that's what we're gonna highlight.
				l.Error(ErrUnmatched{Span: l.SpanFrom(l.cursor - 2)})
				text = l.SeekEOF()
			}
			l.Push(len("/*")+len(text), token.Comment)
		case r == '*' && l.Peek() == '/':
			l.cursor++ // Skip the /.

			// The user definitely thought nested comments were allowed. :/
			tok := l.Push(len("*/"), token.Unrecognized)
			l.Error(ErrUnmatched{Span: tok.Span()})

		case strings.ContainsRune(";,/:=-", r): // . is handled elsewhere.
			// Random punctuation that doesn't require special handling.
			l.Push(utf8.RuneLen(r), token.Punct)

		case strings.ContainsRune("()[]{}<>", r):
			tok := l.Push(utf8.RuneLen(r), token.Punct)
			l.braces = append(l.braces, tok.ID())

		case r == '"', r == '\'':
			l.cursor-- // Back up to behind the quote before resuming.
			lexString(l)

		case r == '.':
			// A . is normally a single token, unless followed by a digit, which
			// makes it into a digit.
			if r := l.Peek(); !unicode.IsDigit(r) {
				l.Push(1, token.Punct)
				continue
			}
			fallthrough
		case unicode.IsDigit(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			lexNumber(l)

		case r == '_' || xidStart(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			rawIdent := l.TakeWhile(xidContinue)

			// Eject any trailing unprintable characters.
			id := strings.TrimRightFunc(rawIdent, func(r rune) bool {
				return !unicode.IsPrint(r)
			})
			if id == "" {
				// This "identifier" appears to consist entirely of unprintable
				// characters (e.g. combining marks).
				tok := l.Push(len(rawIdent), token.Unrecognized)
				l.Error(ErrUnrecognized{Token: tok})
				continue
			}

			l.cursor -= len(rawIdent) - len(id)
			tok := l.Push(len(id), token.Ident)

			// Legalize non-ASCII runes.
			if !isASCIIIdent(tok.Text()) {
				l.Error(ErrNonASCIIIdent{Token: tok})
			}

		default:
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)

			unknown := l.TakeWhile(func(r rune) bool {
				return !strings.ContainsRune(";,/:=-.([{<>}])_\"'", r) &&
					!xidStart(r) &&
					!unicode.IsDigit(r) &&
					!unicode.In(r, unicode.Pattern_White_Space)
			})
			tok := l.Push(len(unknown), token.Unrecognized)
			l.Error(ErrUnrecognized{Token: tok})
		}
	}

	// Fuse brace pairs. We do this at the very end because it's easier to apply
	// lookahead heuristics here.
	fuseBraces(l)

	// Perform implicit string concatenation.
	fuseStrings(l)
}

// fuseBraces performs brace matching and token fusion, based on the contents of
// l.braces.
func fuseBraces(l *lexer) {
	var opens []token.ID
	for i := 0; i < len(l.braces); i++ {
		// At most four tokens are considered for fusion in one loop iteration,
		// named t0 through t3. The first token we extract is the third in this
		// sequence and thus is named t2.

		t2 := l.braces[i].In(l.Context)
		open, _ := bracePair(t2.Text())
		if t2.Text() == open {
			opens = append(opens, t2.ID())
			continue
		}

		// If no opens are present, this is an orphaned close brace.
		if len(opens) == 0 {
			l.Error(ErrUnmatched{Span: t2.Span()})
			continue
		}

		t1 := opens[len(opens)-1].In(l.Context)
		if t1.Text() == open {
			// Common case: the braces match.
			token.Fuse(t1, t2)
			opens = opens[:len(opens)-1]
			continue
		}

		// Check to see how similar this situation is to something like
		// the "irreducible" braces {[}]. This catches common cases of unpaired
		// braces.
		var t0, t3 token.Token
		if len(opens) > 1 {
			t0 = opens[len(opens)-2].In(l.Context)
		}
		// Don't seek for the next unpaired closer; that results in quadratic
		// behavior. Instead, we just look at i+1.
		if i+1 < len(l.braces) {
			t3 = l.braces[i+1].In(l.Context)
		}

		nextOpen, _ := bracePair(t3.Text())
		leftMatch := t0.Text() == open
		rightMatch := t3.Text() != nextOpen && t1.Text() == nextOpen

		switch {
		case leftMatch && rightMatch:
			l.Error(ErrUnmatched{
				Span:        t1.Span(),
				Mismatch:    t2.Span(),
				ShouldMatch: t3.Span(),
			})
			token.Fuse(t0, t2)
			// We do not fuse t1 to t3, since that would result in partially
			// overlapping nested token trees, which violates an invariant of
			// the token stream data structure.

			opens = opens[:len(opens)-2]
			i++
		case leftMatch:
			l.Error(ErrUnmatched{
				Span: t1.Span(),
			})
			token.Fuse(t0, t2) // t1 does not get fused in this case.
			opens = opens[:len(opens)-2]
		case rightMatch:
			l.Error(ErrUnmatched{
				Span:        t1.Span(),
				Mismatch:    t2.Span(),
				ShouldMatch: t3.Span(),
			})
			token.Fuse(t1, t3) // t2 does not get fused in this case.

			opens = opens[:len(opens)-1]
			i++
		default:
			l.Error(ErrUnmatched{
				Span: t2.Span(),
			})
			// No fusion happens here, we treat t2 as being orphaned.
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range opens {
		open := open.In(l.Context)
		l.Error(ErrUnmatched{Span: open.Span()})
	}

	// In backwards order, generate empty tokens to fuse with
	// the unclosed delimiters.
	for i := len(opens) - 1; i >= 0; i-- {
		empty := l.Push(0, token.Unrecognized)
		token.Fuse(opens[i].In(l.Context), empty)
	}
}

// fuseStrings fuses adjacent string literals into their concatenations. This
// implements implicit concatenation by juxtaposition.
func fuseStrings(l *lexer) {
	concat := func(start, end token.Token) {
		if start.Nil() || start == end {
			return
		}

		var buf strings.Builder
		for i := start.ID(); i <= end.ID(); i++ {
			tok := i.In(l.Context)
			if s, ok := tok.AsString(); ok {
				buf.WriteString(s)
				token.ClearValue(tok)
			}
		}

		token.SetValue(start, buf.String())
		token.Fuse(start, end)
	}

	var start, end token.Token
	l.All()(func(tok token.Token) bool {
		switch tok.Kind() {
		case token.Space, token.Comment:
			break

		case token.String:
			if start.Nil() {
				start = tok
			}
			end = tok

		default:
			concat(start, end)
			start = token.Nil
			end = token.Nil
		}

		return true
	})
	concat(start, end)
}

// bracePair returns the open/close brace pair this pair of braces is part of,
// or "", "" if it's not an open/close.
func bracePair(s string) (string, string) {
	switch s {
	case "(", ")":
		return "(", ")"
	case "[", "]":
		return "[", "]"
	case "{", "}":
		return "{", "}"
	case "<", ">":
		return "<", ">"
	case "/*", "*/":
		return "/*", "*/"
	default:
		return "", ""
	}
}

func isASCIIIdent(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}
