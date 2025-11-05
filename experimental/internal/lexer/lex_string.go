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
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// lexString lexes a string starting at the current cursor.
//
// The cursor position should be just before the string's first quote character.
func lexString(l *lexer, sigil string) {
	start := l.cursor
	l.cursor += len(sigil)

	// Check for a triple quote.
	quote := l.Rest()[:1]
	if len(l.Rest()) >= 3 && l.Rest()[1:2] == quote && l.Rest()[2:3] == quote {
		quote = l.Rest()[:3]
	}
	l.cursor += len(quote)

	var (
		buf        strings.Builder
		terminated bool
		escapes    []tokenmeta.Escape
	)
	for !l.Done() {
		if strings.HasPrefix(l.Rest(), quote) {
			l.cursor += len(quote)
			terminated = true
			break
		}

		cursor := l.cursor
		sc := lexStringContent(l)
		if !sc.escape.IsZero() {
			if escapes == nil {
				// If we saw our first escape, spill the string into the buffer
				// up to just before the escape.
				buf.WriteString(l.Text()[start+1 : cursor])
			}

			escape := tokenmeta.Escape{
				Start: uint32(sc.escape.Start),
				End:   uint32(sc.escape.End),
			}
			if sc.isRawByte {
				escape.Byte = byte(sc.rune)
			} else {
				escape.Rune = sc.rune
			}
			escapes = append(escapes, escape)
		}

		if escapes != nil {
			if sc.isRawByte {
				buf.WriteByte(byte(sc.rune))
			} else {
				buf.WriteRune(sc.rune)
			}
		}
	}

	tok := l.Push(l.cursor-start, token.String)
	if escapes != nil {
		meta := token.MutateMeta[tokenmeta.String](tok)
		meta.Text = buf.String()
		meta.Escapes = escapes
	}

	if sigil != "" {
		token.MutateMeta[tokenmeta.String](tok).Prefix = uint32(len(sigil))
	}

	if len(quote) > 1 {
		token.MutateMeta[tokenmeta.String](tok).Quote = uint32(len(quote))
	}

	if !terminated {
		var note report.DiagnosticOption
		if len(tok.Text()) == 1 {
			note = report.Notef("this string consists of a single orphaned quote")
		} else if strings.HasSuffix(tok.Text(), quote) && len(quote) == 1 {
			note = report.SuggestEdits(
				tok,
				"this string appears to end in an escaped quote",
				report.Edit{
					Start: tok.Span().Len() - 2, End: tok.Span().Len(),
					Replace: fmt.Sprintf(`\\%s%s`, quote, quote),
				},
			)
		}

		l.Errorf("unterminated string literal").Apply(
			report.Snippetf(tok, "expected to be terminated by `%s`", quote),
			note,
		)
	}
}

type stringContent struct {
	rune rune

	isRawByte bool
	escape    report.Span
}

// lexStringContent lexes a single logical rune's worth of content for a quoted
// string.
func lexStringContent(l *lexer) (sc stringContent) {
	start := l.cursor
	r := l.Pop()

	switch {
	case r == 0:
		esc := l.SpanFrom(l.cursor - utf8.RuneLen(r))
		l.Errorf("unescaped NUL bytes are not permitted in string literals").Apply(
			report.Snippet(esc),
			report.SuggestEdits(esc, "replace it with `\\0` or `\\x00`", report.Edit{
				Start:   0,
				End:     1,
				Replace: "\\0",
			}),
		)
	case r == '\n':
		// TODO: This diagnostic is simply user-hostile. We should remove it.
		// Not having this is valuable for strings that contain e.g. CEL
		// expressions, and there is no technical reason that Protobuf forbids
		// it. (A historical note: C forbids this because Ken's original
		// C preprocessor, written in PDP11 assembly, was incapable of dealing
		// with multi-line tokens because Ken didn't originally bother.
		// Many programming languages have since thoughtlessly copied this
		// choice, including Protobuf, whose lexical morphology is almost
		// exactly C's).
		nl := l.SpanFrom(l.cursor - utf8.RuneLen(r))
		l.Errorf("unescaped newlines are not permitted in string literals").Apply(
			report.Snippet(nl),
			report.Helpf("consider splitting this into adjacent string literals; Protobuf will automatically concatenate them"),
		)
	case report.NonPrint(r):
		// Warn if the user has a non-printable character in their string that isn't
		// ASCII whitespace.
		var escape string
		switch {
		case r < 0x80:
			escape = fmt.Sprintf(`\x%02x`, r)
		case r < 0x10000:
			escape = fmt.Sprintf(`\u%04x`, r)
		default:
			escape = fmt.Sprintf(`\U%08x`, r)
		}

		esc := l.SpanFrom(l.cursor - utf8.RuneLen(r))
		l.Warnf("non-printable character in string literal").Apply(
			report.Snippet(esc),
			report.SuggestEdits(esc, "consider escaping it", report.Edit{
				Start:   0,
				End:     len(esc.Text()),
				Replace: escape,
			}),
		)
	}

	if r != '\\' {
		// We intentionally do not legalize against literal \0 and \n. The above warning
		// covers \0 and legalizing against \n is user-hostile. This is valuable for
		// e.g. strings that contain CEL code.
		//
		// In other words, this limitation helps no one, so we ignore it.
		return stringContent{rune: r}
	}

	r = l.Pop()
	switch r {
	// These are all the simple escapes.
	case 'a':
		sc.rune = '\a' // U+0007
		sc.escape = l.SpanFrom(start)
	case 'b':
		sc.rune = '\b' // U+0008
		sc.escape = l.SpanFrom(start)
	case 'f':
		sc.rune = '\f' // U+000C
		sc.escape = l.SpanFrom(start)
	case 'n':
		sc.rune = '\n'
		sc.escape = l.SpanFrom(start)
	case 'r':
		sc.rune = '\r'
		sc.escape = l.SpanFrom(start)
	case 't':
		sc.rune = '\t'
		sc.escape = l.SpanFrom(start)
	case 'v':
		sc.rune = '\v' // U+000B
		sc.escape = l.SpanFrom(start)
	case '\\', '\'', '"', '?':
		sc.escape = l.SpanFrom(start)
		sc.rune = r

	// Octal escape. Need to eat the next two runes if they're octal.
	case '0', '1', '2', '3', '4', '5', '6', '7':
		sc.isRawByte = true
		sc.rune = r - '0'
		for i := 0; i < 2 && !l.Done(); i++ {
			// Check before consuming the rune. If we see e.g.
			// an 8, we don't want to consume it.
			r = l.Peek()
			if r < '0' || r > '7' {
				break
			}
			_ = l.Pop()

			sc.rune *= 8
			sc.rune += r - '0'
		}
		sc.escape = l.SpanFrom(start)

	// Hex escapes. And yes, the 'X' is no mistake: Protobuf is one of the
	// only language that supports \XNN as an alias for \xNN, something not
	// even C offers! https://en.cppreference.com/w/c/language/escape
	case 'x', 'X', 'u', 'U':
		var digits, consumed int
		switch r {
		case 'x', 'X':
			digits = 2
			sc.isRawByte = true
		case 'u':
			digits = 4
		case 'U':
			digits = 8
		}

		for i := 0; i < digits && !l.Done(); i++ {
			digit := parseDigit(l.Peek())
			if digit >= 16 {
				break
			}

			sc.rune *= 16
			sc.rune += rune(digit)

			l.Pop()
			consumed++
		}

		sc.escape = l.SpanFrom(start)
		if consumed == 0 {
			l.Error(errInvalidEscape{Span: sc.escape})
		} else if !sc.isRawByte {
			// \u and \U must have exact numbers of digits.
			if consumed != digits || !utf8.ValidRune(sc.rune) {
				l.Error(errInvalidEscape{Span: sc.escape})
			}
		}

	default:
		sc.escape = l.SpanFrom(start)
		l.Error(errInvalidEscape{Span: sc.escape})
	}

	return sc
}

// errInvalidEscape diagnoses an invalid escape sequence within a string
// literal.
type errInvalidEscape struct {
	Span report.Span // The span of the offending escape within a literal.
}

// Diagnose implements [report.Diagnose].
func (e errInvalidEscape) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("invalid escape sequence"))

	text := e.Span.Text()

	if len(text) < 2 {
		d.Apply(report.Snippet(e.Span))
	}

	switch c := text[1]; c {
	case 'x', 'X':
		if len(text) < 3 {
			d.Apply(report.Snippetf(e.Span, "`\\%c` must be followed by at least one hex digit", c))
			return
		}
		return
	case 'u', 'U':
		expected := 4
		if c == 'U' {
			expected = 8
		}

		if len(text[2:]) != expected {
			d.Apply(report.Snippetf(e.Span, "`\\%c` must be followed by exactly %d hex digits", c, expected))
			return
		}

		value, _ := strconv.ParseUint(text[2:], 16, 32)
		if !utf8.ValidRune(rune(value)) {
			d.Apply(report.Snippetf(e.Span, "must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF"))
			return
		}
		return
	}

	d.Apply(report.Snippet(e.Span))
}
