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
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

// lexString lexes a string starting at the current cursor.
//
// The cursor position should be just before the string's first quote character.
func lexString(l *lexer, sigil string) {
	start := l.cursor
	l.cursor += len(sigil)

	// Check for a triple quote.
	quote := l.rest()[:1]
	if len(l.rest()) >= 3 && l.rest()[1:2] == quote && l.rest()[2:3] == quote {
		quote = l.rest()[:3]
	}
	l.cursor += len(quote)

	var (
		buf        strings.Builder
		terminated bool
		escapes    []tokenmeta.Escape
	)
	for !l.done() {
		if strings.HasPrefix(l.rest(), quote) {
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

	tok := l.push(l.cursor-start, token.String)
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
	escape    source.Span
}

// lexStringContent lexes a single logical rune's worth of content for a quoted
// string.
func lexStringContent(l *lexer) (sc stringContent) {
	start := l.cursor
	r := l.pop()

	switch {
	case r == 0:
		esc := l.spanFrom(l.cursor - utf8.RuneLen(r))
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
		nl := l.spanFrom(l.cursor - utf8.RuneLen(r))
		l.Errorf("unescaped newlines are not permitted in string literals").Apply(
			report.Snippet(nl),
			report.Helpf("consider splitting this into adjacent string literals; Protobuf will automatically concatenate them"),
		)
	case unicodex.NonPrint(r):
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

		esc := l.spanFrom(l.cursor - utf8.RuneLen(r))
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

	r = l.pop()
escSwitch:
	switch r {
	// These are all the simple escapes.
	case 'n':
		sc.rune = '\n'
		sc.escape = l.spanFrom(start)
		return sc
	case 'r':
		sc.rune = '\r'
		sc.escape = l.spanFrom(start)
		return sc
	case 't':
		sc.rune = '\t'
		sc.escape = l.spanFrom(start)
		return sc
	case '\\', '\'', '"':
		sc.escape = l.spanFrom(start)
		sc.rune = r
		return sc

	case 'a':
		if !l.EscapeExtended {
			break
		}
		sc.rune = '\a' // U+0007
		sc.escape = l.spanFrom(start)
		return sc
	case 'b':
		if !l.EscapeExtended {
			break
		}
		sc.rune = '\b' // U+0008
		sc.escape = l.spanFrom(start)
		return sc
	case 'f':
		if !l.EscapeExtended {
			break
		}
		sc.rune = '\f' // U+000C
		sc.escape = l.spanFrom(start)
		return sc
	case 'v':
		if !l.EscapeExtended {
			break
		}
		sc.rune = '\v' // U+000B
		sc.escape = l.spanFrom(start)
		return sc

	case '?':
		if !l.EscapeAsk {
			break
		}
		sc.rune = '?'
		sc.escape = l.spanFrom(start)
		return sc

	// Octal escape. Need to eat the next two runes if they're octal.
	case '0', '1', '2', '3', '4', '5', '6', '7':
		if !l.EscapeOctal {
			if r == '0' {
				sc.rune = 0
				sc.escape = l.spanFrom(start)
				return sc
			}

			break
		}

		sc.isRawByte = true
		sc.rune = r - '0'
		for i := 0; i < 2 && !l.done(); i++ {
			// Check before consuming the rune. If we see e.g.
			// an 8, we don't want to consume it.
			r = l.peek()
			if r < '0' || r > '7' {
				break
			}
			_ = l.pop()

			sc.rune *= 8
			sc.rune += r - '0'
		}
		sc.escape = l.spanFrom(start)
		return sc

	// Hex escapes. And yes, the 'X' is no mistake: Protobuf is one of the
	// only language that supports \XNN as an alias for \xNN, something not
	// even C offers! https://en.cppreference.com/w/c/language/escape
	case 'x', 'X', 'u', 'U':
		var digits, consumed int
		switch r {
		case 'X':
			if !l.EscapeUppercaseX {
				break escSwitch
			}
			fallthrough
		case 'x':
			digits = 2
			sc.isRawByte = true

		case 'u':
			if !l.EscapeOldStyleUnicode {
				break escSwitch
			}
			digits = 4
		case 'U':
			if !l.EscapeOldStyleUnicode {
				break escSwitch
			}
			digits = 8
		}

		for i := 0; i < digits && !l.done(); i++ {
			digit, ok := unicodex.Digit(l.peek(), 16)
			if !ok {
				break
			}

			sc.rune *= 16
			sc.rune += rune(digit)

			l.pop()
			consumed++
		}

		sc.escape = l.spanFrom(start)
		if consumed == 0 {
			l.Error(errtoken.InvalidEscape{Span: sc.escape})
		} else if !l.EscapePartialX || !sc.isRawByte {
			// \u and \U must have exact numbers of digits.
			if consumed != digits || !utf8.ValidRune(sc.rune) {
				l.Error(errtoken.InvalidEscape{Span: sc.escape})
			}
		}
		return sc
	}

	sc.escape = l.spanFrom(start)
	l.Error(errtoken.InvalidEscape{Span: sc.escape})
	return sc
}
