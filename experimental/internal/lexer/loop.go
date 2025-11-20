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
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

// loop is the main loop of the lexer.
func loop(l *lexer) {
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
	for !l.done() {
		mp.check()
		start := l.cursor

		if unicode.In(l.peek(), unicode.Pattern_White_Space) {
			// Whitespace. Consume as much whitespace as possible and mint a
			// whitespace token.
			l.takeWhile(func(r rune) bool {
				return unicode.In(r, unicode.Pattern_White_Space)
			})
			l.push(l.cursor-start, token.Space)
			continue
		}

		// Find the next valid keyword.
		var what OnKeyword
		var kw keyword.Keyword
		for k := range keyword.Prefixes(l.rest()) {
			n := l.OnKeyword(k)
			if n != DiscardKeyword {
				kw = k
				what = n
			}
		}

		switch what {
		case KeepKeyword, BracketKeyword:
			word := kw.String()
			if l.NumberCanStartWithDot && kw == keyword.Dot {
				next, _ := stringsx.Rune(l.rest(), len(word))
				if unicode.IsDigit(next) {
					break
				}
			}

			kind := token.Punct
			if kw.IsReservedWord() {
				kind = token.Ident
				// If this is a reserved word, the rune after it must not be
				// an XID continue.
				next, _ := stringsx.Rune(l.rest(), len(word))
				if unicodex.IsXIDContinue(next) {
					break
				}
			}

			l.cursor += len(word)
			tok := l.push(len(word), kind)

			if what == BracketKeyword {
				l.braces = append(l.braces, tok.ID())
			}
			continue

		case LineComment:
			word := kw.String()
			l.cursor += len(word)

			var text string
			if comment, ok := l.seekInclusive("\n"); ok {
				text = comment
			} else {
				text = l.seekEOF()
			}
			l.push(len(word)+len(text), token.Comment)
			continue

		case BlockComment:
			word := kw.String()
			l.cursor += len(word)

			// Block comment. Seek to the next "*/". Protobuf comments
			// unfortunately do not nest, and allowing them to nest can't
			// be done in a backwards-compatible manner. We acknowledge that
			// this behavior is user-hostile.
			//
			// If we encounter no "*/", seek EOF and emit a diagnostic. Trying
			// to lex a partial comment is hopeless.
			_, end, fused := kw.Brackets()
			if kw == end {
				// The user definitely thought nested comments were allowed. :/
				tok := l.push(len(end.String()), token.Unrecognized)
				l.Error(errtoken.Unmatched{Span: tok.Span(), Keyword: kw}).Apply(
					report.Notef("nested `%s` comments are not supported", fused),
				)
				continue
			}

			var text string
			if comment, ok := l.seekInclusive(end.String()); ok {
				text = comment
			} else {
				// Create a span for the /*, that's what we're gonna highlight.
				l.Error(errtoken.Unmatched{
					Span:    l.spanFrom(l.cursor - len(word)),
					Keyword: kw,
				})
				text = l.seekEOF()
			}
			l.push(len(word)+len(text), token.Comment)

			continue
		}

		r := l.pop()

		switch {
		case r == '"', r == '\'':
			l.cursor-- // Back up to behind the quote before resuming.
			lexString(l, "")

		case l.NumberCanStartWithDot && r == '.', unicode.IsDigit(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			lexNumber(l)

		case unicodex.IsXIDStart(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			rawIdent := l.takeWhile(unicodex.IsXIDContinue)

			// Eject any trailing unprintable characters.
			id := strings.TrimRightFunc(rawIdent, func(r rune) bool {
				return !unicode.IsPrint(r)
			})
			if id == "" {
				// This "identifier" appears to consist entirely of unprintable
				// characters (e.g. combining marks).
				tok := l.push(len(rawIdent), token.Unrecognized)
				l.Errorf("unrecognized token").Apply(
					report.Snippet(tok),
					report.Debugf("%v, %v, %q", tok.ID(), tok.Span(), tok.Text()),
				)
				continue
			}

			// Figure out if we should be doing a prefixed string instead.
			next := l.peek()
			if next == '"' || next == '\'' &&
				// Check to see if we like this prefix.
				l.IsAffix != nil && l.IsAffix(rawIdent, token.String, false) {
				l.cursor -= len(rawIdent)
				lexString(l, rawIdent)
				continue
			}

			l.cursor -= len(rawIdent) - len(id)

			tok := l.push(len(id), token.Ident)

			// Legalize non-ASCII runes.
			if l.RequireASCIIIdent && !unicodex.IsASCIIIdent(tok.Text()) {
				l.Errorf("non-ASCII identifiers are not allowed").Apply(
					report.Snippet(tok),
				)
			}

		default:
			l.badBytes += utf8.RuneLen(r)
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
	if len(l.Text()) > MaxFileSize {
		l.Errorf("files larger than 2GB (%d bytes) are not supported", MaxFileSize).Apply(
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

	if l.peek() == '\uFEFF' {
		l.pop() // Peel off a leading UTF-8 BOM.
		l.push(3, token.Unrecognized)
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

		t2 := id.Wrap(l.Stream, l.braces[i])
		open, _, _ := t2.Keyword().Brackets()
		if t2.Keyword() == open {
			opens = append(opens, t2.ID())
			continue
		}

		// If no opens are present, this is an orphaned close brace.
		if len(opens) == 0 {
			l.Error(errtoken.Unmatched{Span: t2.Span(), Keyword: t2.Keyword()})
			continue
		}

		t1 := id.Wrap(l.Stream, opens[len(opens)-1])
		if t1.Keyword() == open {
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
			t0 = id.Wrap(l.Stream, opens[len(opens)-2])
		}
		// Don't seek for the next unpaired closer; that results in quadratic
		// behavior. Instead, we just look at i+1.
		if i+1 < len(l.braces) {
			t3 = id.Wrap(l.Stream, l.braces[i+1])
		}

		nextOpen, _, _ := t3.Keyword().Brackets()
		leftMatch := t0.Keyword() == open
		rightMatch := t3.Keyword() != nextOpen && t1.Keyword() == nextOpen

		switch {
		case leftMatch && rightMatch:
			l.Error(errtoken.Unmatched{
				Span:        t1.Span(),
				Keyword:     t1.Keyword(),
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
			l.Error(errtoken.Unmatched{
				Span:    t1.Span(),
				Keyword: t1.Keyword(),
			})
			token.Fuse(t0, t2) // t1 does not get fused in this case.
			opens = opens[:len(opens)-2]
		case rightMatch:
			l.Error(errtoken.Unmatched{
				Span:        t1.Span(),
				Keyword:     t1.Keyword(),
				Mismatch:    t2.Span(),
				ShouldMatch: t3.Span(),
			})
			token.Fuse(t1, t3) // t2 does not get fused in this case.

			opens = opens[:len(opens)-1]
			i++
		default:
			l.Error(errtoken.Unmatched{
				Span:    t2.Span(),
				Keyword: t2.Keyword(),
			})
			// No fusion happens here, we treat t2 as being orphaned.
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range opens {
		open := id.Wrap(l.Stream, open)
		l.Error(errtoken.Unmatched{Span: open.Span(), Keyword: open.Keyword()})
	}

	// In backwards order, generate empty tokens to fuse with
	// the unclosed delimiters.
	for _, open := range slices.Backward(opens) {
		empty := l.push(0, token.Unrecognized)
		token.Fuse(id.Wrap(l.Stream, open), empty)
	}
}

// fuseStrings fuses adjacent string literals into their concatenations. This
// implements implicit concatenation by juxtaposition.
func fuseStrings(l *lexer) {
	concat := func(start, end token.Token) {
		if start.IsZero() || start == end {
			return
		}

		var escapes []tokenmeta.Escape
		var buf strings.Builder
		for i := start.ID(); i <= end.ID(); i++ {
			tok := id.Wrap(l.Stream, i)
			if s := tok.AsString(); !s.IsZero() {
				buf.WriteString(s.Text())
				if i > start.ID() {
					token.ClearMeta[tokenmeta.String](tok)
				}
			}
		}

		meta := token.MutateMeta[tokenmeta.String](start)
		meta.Text = buf.String()
		meta.Concatenated = true
		meta.Escapes = escapes

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
