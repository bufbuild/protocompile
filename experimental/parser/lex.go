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
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// maxFileSize is the maximum file size Protocompile supports.
const maxFileSize int = math.MaxInt32 // 2GB

// lex performs lexical analysis on the file contained in ctx, and appends any
// diagnostics that results in to l.
//
// lex will freeze the stream in ctx when it is done.
//
// You should almost never need to call this function; [Parse] calls it directly.
// It is exported so that it is straight forward to build other parsers on top
// of the Protobuf lexer.
func lex(ctx token.Context, errs *report.Report) {
	l := &lexer{
		Context: ctx,
		Stream:  ctx.Stream(),
		Report:  errs,
	}

	defer l.CatchICE(false, func(d *report.Diagnostic) {
		d.Apply(
			report.Snippetf(l.Span(l.cursor, l.cursor), "cursor is here"),
			report.Notef("cursor: %d, count: %d", l.cursor, l.count),
		)
	})

	if !lexPrelude(l) {
		return
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
				l.Error(errUnmatched{Span: l.SpanFrom(l.cursor - 2)})
				text = l.SeekEOF()
			}
			l.Push(len("/*")+len(text), token.Comment)
		case r == '*' && l.Peek() == '/':
			l.cursor++ // Skip the /.

			// The user definitely thought nested comments were allowed. :/
			tok := l.Push(len("*/"), token.Unrecognized)
			l.Error(errUnmatched{Span: tok.Span()})

		case r == '&' && l.Peek() == '&':
			l.Push(2, token.Punct)
		case r == '|' && l.Peek() == '|':
			l.Push(2, token.Punct)

		case strings.ContainsRune("=!<>", r):
			if l.Peek() == '=' {
				l.Pop()
				l.Push(2, token.Punct)
			} else {
				l.Push(1, token.Punct)
			}

		case strings.ContainsRune(";,:+-*/%?", r): // . is handled elsewhere.
			// Random punctuation that doesn't require special handling.
			l.Push(utf8.RuneLen(r), token.Punct)

		case strings.ContainsRune("()[]{}", r):
			tok := l.Push(utf8.RuneLen(r), token.Punct)
			l.braces = append(l.braces, tok.ID())

		case r == '"', r == '\'':
			l.cursor-- // Back up to behind the quote before resuming.
			lexString(l, "")

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
				l.Errorf("unrecognized token").Apply(
					report.Snippet(tok),
					report.Debugf("%v, %v, %q", tok.ID(), tok.Span(), tok.Text()),
				)
				continue
			}

			// Figure out if we should be doing a raw string instead.
			next := l.Peek()
			if len(rawIdent) <= 2 && isASCIIIdent(rawIdent) && (next == '"' || next == '\'') {
				l.cursor -= len(rawIdent)
				lexString(l, rawIdent)
				continue
			}

			l.cursor -= len(rawIdent) - len(id)

			tok := l.Push(len(id), token.Ident)

			// Legalize non-ASCII runes.
			if !isASCIIIdent(tok.Text()) {
				l.Errorf("non-ASCII identifiers are not allowed").Apply(
					report.Snippet(tok),
				)
			}

		default:
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)

			unknown := l.TakeWhile(func(r rune) bool {
				return !strings.ContainsRune(";,:=+-*/%!?<>([{}])_\"'", r) &&
					!xidStart(r) &&
					!unicode.IsDigit(r) &&
					!unicode.In(r, unicode.Pattern_White_Space)
			})
			tok := l.Push(len(unknown), token.Unrecognized)
			l.Errorf("unrecognized token").Apply(
				report.Snippet(tok),
				report.Debugf("%v, %v, %q", tok.ID(), tok.Span(), tok.Text()),
			)
		}
	}

	// Fuse brace pairs. We do this at the very end because it's easier to apply
	// lookahead heuristics here.
	fuseBraces(l)

	// Perform implicit string concatenation.
	fuseStrings(l)
}

// lexPrelude performs various file-prelude checks, such as size and encoding
// verification. Returns whether lexing should proceed.
func lexPrelude(l *lexer) bool {
	if l.Text() == "" {
		return true
	}

	// Check that the file isn't too big. We give up immediately if that's
	// the case.
	if len(l.Text()) > maxFileSize {
		l.Errorf("files larger than 2GB (%d bytes) are not supported", maxFileSize).Apply(
			report.InFile(l.Path()),
		)
		return false
	}

	// Heuristically check for a UTF-16-encoded file. There are two good
	// heuristics:
	// 1. Presence of a UTF-16 BOM, which is either FE FF or FF FE, depending on
	//    endianness.
	// 2. Exactly one of the first two bytes is a NUL. Valid Protobuf cannot
	//    contain a NUL in the first two bytes, so this is probably a UTF-16-encoded
	//    ASCII rune.
	bom16 := strings.HasPrefix(l.Text(), "\xfe\xff") || strings.HasPrefix(l.Text(), "\xff\xfe")
	ascii16 := len(l.Text()) >= 2 && (l.Text()[0] == 0 || l.Text()[1] == 0)
	if bom16 || ascii16 {
		l.Errorf("input appears to be encoded with UTF-16").Apply(
			report.InFile(l.Path()),
			report.Notef("Protobuf files must be UTF-8 encoded"),
		)
		return false
	}

	// Check that the text of the file is actually UTF-8.
	var idx int
	var count int
	for i, r := range stringsx.Runes(l.Text()) {
		if r != -1 {
			continue
		}
		if count == 0 {
			idx = i
		}
		count++
	}
	frac := float64(count) / float64(len(l.Text()))
	switch {
	case frac == 0:
		break
	case frac < 0.2:
		// This diagnostic is for cases where this file appears to be corrupt.
		// We pick 20% non-UTF-8 as the threshold to show this error.
		l.Errorf("input appears to be encoded with UTF-8, but found invalid byte").Apply(
			report.Snippet(l.Span(idx, idx+1)),
			report.Notef("non-UTF-8 byte occurs at offset %d (%#x)", idx, idx),
			report.Notef("Protobuf files must be UTF-8 encoded"),
		)
		return false
	default:
		l.Errorf("input appears to be a binary file").Apply(
			report.InFile(l.Path()),
			report.Notef("non-UTF-8 byte occurs at offset %d (%#x)", idx, idx),
			report.Notef("Protobuf files must be UTF-8 encoded"),
		)
		return false
	}

	if l.Peek() == '\uFEFF' {
		l.Pop() // Peel off a leading UTF-8 BOM.
		l.Push(3, token.Unrecognized)
	}

	return true
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
			l.Error(errUnmatched{Span: t2.Span()})
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
			l.Error(errUnmatched{
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
			l.Error(errUnmatched{
				Span: t1.Span(),
			})
			token.Fuse(t0, t2) // t1 does not get fused in this case.
			opens = opens[:len(opens)-2]
		case rightMatch:
			l.Error(errUnmatched{
				Span:        t1.Span(),
				Mismatch:    t2.Span(),
				ShouldMatch: t3.Span(),
			})
			token.Fuse(t1, t3) // t2 does not get fused in this case.

			opens = opens[:len(opens)-1]
			i++
		default:
			l.Error(errUnmatched{
				Span: t2.Span(),
			})
			// No fusion happens here, we treat t2 as being orphaned.
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range opens {
		open := open.In(l.Context)
		l.Error(errUnmatched{Span: open.Span()})
	}

	// In backwards order, generate empty tokens to fuse with
	// the unclosed delimiters.
	for _, open := range slices.Backward(opens) {
		empty := l.Push(0, token.Unrecognized)
		token.Fuse(open.In(l.Context), empty)
	}
}

// fuseStrings fuses adjacent string literals into their concatenations. This
// implements implicit concatenation by juxtaposition.
func fuseStrings(l *lexer) {
	concat := func(start, end token.Token) {
		if start.IsZero() || start == end {
			return
		}

		var buf strings.Builder
		for i := start.ID(); i <= end.ID(); i++ {
			tok := i.In(l.Context)
			if s := tok.AsString(); !s.IsZero() {
				buf.WriteString(s.Text())
				token.ClearMeta[tokenmeta.String](tok)
			}
		}

		meta := token.MutateMeta[tokenmeta.String](start)
		meta.Text = buf.String()
		meta.Concatenated = true

		token.Fuse(start, end)
	}

	var start, end token.Token
	for tok := range l.All() {
		switch tok.Kind() {
		case token.Space, token.Comment:

		case token.String:
			if start.IsZero() {
				start = tok
			} else {
				overall := start.AsString().Prefix()
				prefix := tok.AsString().Prefix()

				if !prefix.IsZero() && overall.Text() != prefix.Text() {
					l.Errorf("implicitly-concatenated string has incompatible prefix").Apply(
						report.Snippet(prefix),
						report.Snippetf(overall, "must match this prefix"),
					)
				}
			}
			end = tok

		default:
			concat(start, end)
			start = token.Zero
			end = token.Zero
		}
	}

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
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		case r == '_':
		default:
			return false
		}
	}
	return len(s) > 0
}
