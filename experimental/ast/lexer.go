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

package ast

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
)

// lexer is a Protobuf lexer.
type lexer struct {
	*Context
	cursor    int
	openStack []Token
}

// Lex performs lexical analysis, and places any diagnostics in report.
func (l *lexer) Lex(errs *report.Report) {
	// Check that the file isn't too big. We give up immediately if that's
	// the case.
	if len(l.Text()) > MaxFileSize {
		errs.Error(ErrFileTooBig{Path: l.Path()})
		return
	}

	// Also check that the text of the file is actually UTF-8.
	// We go rune by rune to find the first invalid offset.
	for text := l.file.File().Text; text != ""; {
		r := decodeRune(text)
		if r == -1 {
			errs.Error(ErrNotUTF8{
				Path: l.Path(),
				At:   len(l.Text()) - len(text),
				Byte: text[0],
			})
			return
		}
		text = text[utf8.RuneLen(r):]
	}

	var tokens int
	for !l.Done() {
		start := l.cursor
		r := l.Pop()

		prevTokens := tokens
		if prevTokens > 0 && prevTokens == len(l.stream) {
			panic(fmt.Sprintf("protocompile/ast: lexer failed to make progress at offset %d; this is a bug in protocompile", l.cursor))
		}
		tokens = len(l.stream)

		switch {
		case unicode.IsSpace(r):
			// Whitepace. Consume as much whitespace as possible and mint a
			// whitespace token.
			l.TakeWhile(unicode.IsSpace)
			l.PushToken(l.cursor-start, TokenSpace)

		case r == '/' && l.Peek() == '/':
			l.cursor++ // Skip the second /.

			// Single-line comment. Seek to the next '\n' or the EOF.
			var text string
			if comment, ok := l.SeekInclusive("\n"); ok {
				text = comment
			} else {
				text = l.SeekEOF()
			}
			l.PushToken(len("//")+len(text), TokenComment)
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
			if comment, ok := l.SeekInclusive("\n"); ok {
				text = comment
			} else {
				// Create a span for the /*, that's what we're gonna highlight.
				errs.Error(ErrUnterminated{Span: l.NewSpan(l.cursor-2, l.cursor)})
				text = l.SeekEOF()
			}
			l.PushToken(len("/*")+len(text), TokenComment)
		case r == '*' && l.Peek() == '/':
			// The user definitely thought nested comments were allowed. :/
			tok := l.PushToken(len("*/"), TokenUnrecognized)
			errs.Error(ErrUnterminated{Span: tok.Span()})

		case strings.ContainsRune(";,/:=-", r): // . is handled elsewhere.
			// Random punctuation that doesn't require special handling.
			l.PushToken(utf8.RuneLen(r), TokenPunct)

		case strings.ContainsRune("([{<", r): // Push the opener, close it later.
			token := l.PushToken(utf8.RuneLen(r), TokenPunct)
			l.openStack = append(l.openStack, token)
		case strings.ContainsRune(")]}>", r):
			token := l.PushToken(utf8.RuneLen(r), TokenPunct)
			if len(l.openStack) == 0 {
				errs.Error(ErrUnterminated{Span: token.Span()})
			} else {
				end := len(l.openStack) - 1
				var expected string
				switch l.openStack[end].Text() {
				case "(":
					expected = ")"
				case "[":
					expected = "]"
				case "{":
					expected = "}"
				case "<":
					expected = ">"
				}
				if token.Text() != expected {
					errs.Error(ErrUnterminated{Span: l.openStack[end].Span(), Mismatch: token.Span()})
				}

				l.FuseTokens(l.openStack[end], token)
				l.openStack = l.openStack[:end]
			}

		case r == '"', r == '\'':
			l.cursor-- // Back up to behind the quote before resuming.
			l.LexString(errs)

		case r == '.':
			// A . is normally a single token, unless followed by a digit, which makes it
			// into a digit.
			if r := l.Peek(); !unicode.IsDigit(r) {
				l.PushToken(1, TokenPunct)
				continue
			}
			fallthrough
		case unicode.IsDigit(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			l.LexNumber(errs)

		case r == '_' || unicode.IsLetter(r): // Consume fairly-open-ended identifiers, legalize to ASCII later.
			l.TakeWhile(func(r rune) bool {
				return r == '_' || unicode.IsDigit(r) || unicode.IsLetter(r)
			})
			token := l.PushToken(l.cursor-start, TokenIdent)

			// Legalize non-ASCII runes.
			if !isASCIIIdent(token.Text()) {
				errs.Error(ErrNonASCIIIdent{Token: token})
			}

		default: // Consume as much stuff we don't understand as possible, diagnose it.
			l.TakeWhile(func(r rune) bool {
				return !strings.ContainsRune(";,/:=-.([{<>}])_\"'", r) &&
					unicode.IsDigit(r) && unicode.IsLetter(r)
			})
			token := l.PushToken(l.cursor-start, TokenUnrecognized)
			errs.Error(ErrUnrecognized{Token: token})
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range l.openStack {
		errs.Error(ErrUnterminated{Span: open.Span()})
	}
	// In backwards order, generate empty tokens to fuse with
	// the unclosed delimiters.
	for i := len(l.openStack) - 1; i >= 0; i-- {
		empty := l.PushToken(0, TokenUnrecognized)
		l.FuseTokens(l.openStack[i], empty)
	}

	// Perform implicit string concatenation.
	catStrings := func(start, end Token) {
		var buf strings.Builder
		for i := start.raw; i <= end.raw; i++ {
			tok := i.With(l.Context)
			if s, ok := tok.AsString(); ok {
				buf.WriteString(s)
				delete(l.literals, tok.raw)
			}
		}
		l.literals[start.raw] = buf.String()
		l.FuseTokens(start, end)
	}
	var start, end Token
	for i := range l.stream {
		tok := rawToken(i + 1).With(l.Context)
		switch tok.Kind() {
		case TokenSpace, TokenComment:
			continue
		case TokenString:
			if start.Nil() {
				start = tok
			} else {
				end = tok
			}
		default:
			if !start.Nil() && !end.Nil() {
				catStrings(start, end)
			}
			start = Token{}
			end = Token{}
		}
	}
	if !start.Nil() && !end.Nil() {
		catStrings(start, end)
	}
}

// LexString lexes a number starting at the current cursor.
func (l *lexer) LexNumber(errs *report.Report) Token {
	start := l.cursor
	// Accept all digits, legalize later.
	// Consume the largest prefix that satisfies the rules at
	// https://protobuf.com/docs/language-spec#numeric-literals
more:
	r := l.Peek()
	if r == 'e' || r == 'E' {
		_ = l.Pop()
		r = l.Peek()
		if r == '+' || r == '-' {
			_ = l.Pop()
		}

		goto more
	}
	if r == '.' || unicode.IsDigit(r) || unicode.IsLetter(r) ||
		// We consume _ because 0_n is not valid in any context, so we
		// can offer _ digit separators as an extension.
		r == '_' {
		_ = l.Pop()
		goto more
	}

	// Create the token, even if this is an invalid number. This will help
	// the parser pick up bad numbers into number literals.
	digits := l.Text()[start:l.cursor]
	token := l.PushToken(len(digits), TokenNumber)

	// Delete all _s from digits and normalize to lowercase.
	digits = strings.ToLower(strings.ReplaceAll(digits, "_", ""))

	// Now, let's see if this is actually a valid number that needs a diagnostic.
	// First, try to reify it as a uint64.
	switch {
	case strings.HasPrefix(digits, "0x"):
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0x"), 16, 64)
		if err == nil {
			l.literals[token.raw] = value
			return token
		}

		// Emit a diagnostic. Which diagnostic we emit depends on the error Go
		// gave us. NB: all ParseUint errors are *strconv.NumError, as promised
		// by the documentation.
		if err.(*strconv.NumError).Err == strconv.ErrRange {
			errs.Error(ErrIntegerOverflow{Token: token})
		} else {
			errs.Error(ErrInvalidNumber{Token: token})

		}
	case strings.HasPrefix(digits, "0") && strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '7' }) == -1:
		// Annoyingly, 0777 is octal, but 0888 is not, so we have to handle this case specially.
		fallthrough
	case strings.HasPrefix(digits, "0o"): // Rust/Python-style octal ints are an extension.
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0o"), 8, 64)
		if err == nil {
			l.literals[token.raw] = value
			return token
		}

		if err.(*strconv.NumError).Err == strconv.ErrRange {
			errs.Error(ErrIntegerOverflow{Token: token})
		} else {
			errs.Error(ErrInvalidNumber{Token: token})

		}
	case strings.HasPrefix(digits, "0b"): // Binary ints are an extension.
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0b"), 2, 64)
		if err == nil {
			l.literals[token.raw] = value
			return token
		}

		if err.(*strconv.NumError).Err == strconv.ErrRange {
			errs.Error(ErrIntegerOverflow{Token: token})
		} else {
			errs.Error(ErrInvalidNumber{Token: token})

		}
	}

	// This is either a float or a decimal number. Try parsing as decimal first, otherwise
	// try parsing as float.
	_, err := strconv.ParseUint(digits, 10, 64)
	if err == nil {
		// This is the most common result. It gets computed ON DEMAND by Token.AsNumber.
		// DO NOT place it into l.literals.
		return token
	}

	// Check if it's out of range, because if that's the only problem, it's
	// guaranteed to parse as a possibly-infinite float.
	if err.(*strconv.NumError).Err == strconv.ErrRange {
		// We want this to overflow to Infinity as needed, which ParseFloat
		// will do for us. Otherwise it will ties-to-even as expected. Currently,
		// the spec does not say "ties-to-even", but it says "nearest value",
		// which everyone says when they mean "ties-to-even" whether they know it
		// or not.
		//
		// ParseFloat itself says it "returns the nearest floating-point number
		// rounded using IEEE754 unbiased rounding", which is just a weird way to
		// say "ties-to-even".
		value, _ := strconv.ParseFloat(digits, 64)
		l.literals[token.raw] = value
		return token
	}

	// If it was not well-formed, it might be a float.
	value, err := strconv.ParseFloat(digits, 64)

	// This time, the syntax might be invalid. If it is, we decide whether
	// this is a bad float or a bad integer based on whether it contains
	// periods.
	if err.(*strconv.NumError).Err == strconv.ErrSyntax {
		errs.Error(ErrInvalidNumber{Token: token})
	}

	// If the error is ErrRange we don't care, it will clamp to Infinity in
	// the way we want, as noted by the comment above.
	l.literals[token.raw] = value
	return token
}

// LexString lexes a string starting at the current cursor.
//
// The cursor position should be just before the string's first quote character.
func (l *lexer) LexString(errs *report.Report) Token {
	start := l.cursor
	q := l.Pop()

	// Seek to the end of the string, unescaping as we go. We do not
	// materialize an unescaped string if this string does not require escaping.
	var buf strings.Builder
	var haveEsc bool
escapeLoop:
	for !l.Done() {
		r := l.Pop()
		if r == q {
			break
		}

		// Warn if the user has a non-printable character in their string that isn't
		// ASCII whitespace.
		if !unicode.IsGraphic(r) && !strings.ContainsRune(" \n\t\r", r) {
			errs.Warnf("non-printable character in string literal").With(
				report.Snippetf(l.NewSpan(l.cursor-utf8.RuneLen(r), l.cursor), "this is the rune U+%04x", r),
			)
		}

		if r != '\\' {
			// We intentionally do not legalize against literal \0 and \n. The above warning
			// covers \0 and legalizing against \n is user-hostile. This is valuable for
			// e.g. strings that contain CEL code.
			//
			// In other words, this limitation helps no one, so we ignore it.
			if haveEsc {
				buf.WriteRune(r)
			}
			continue
		}

		if !haveEsc {
			buf.WriteString(l.Text()[start+1 : l.cursor-1])
			haveEsc = true
		}

		r = l.Pop()
		switch r {
		// These are all the simple escapes.
		case 'a':
			buf.WriteByte('\a') // U+0007
		case 'b':
			buf.WriteByte('\b') // U+0008
		case 'f':
			buf.WriteByte('\f') // U+000C
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 'v':
			buf.WriteByte('\v') // U+000B
		case '\\', '\'', '"', '?':
			buf.WriteRune(r)

		// Octal escape. Need to eat the next twos rune if they're octal.
		case '0', '1', '2', '3', '4', '5', '6', '7':
			value := byte(r) - '0'
			for i := 0; i < 2; i++ {
				if l.Done() {
					break escapeLoop
				}
				r = l.Peek()

				// Check before consuming the rune. If we see e.g.
				// an 8, we don't want to consume it.
				if r < '0' || r > '7' {
					break
				}
				_ = l.Pop()

				value *= 8
				value |= byte(r) - '0'
			}
			buf.WriteByte(value)

		// Hex escapes.
		case 'x', 'u', 'U':
			var value uint32
			var digits, consumed int
			switch r {
			case 'x':
				digits = 2
			case 'u':
				digits = 4
			case 'U':
				digits = 8
			}

		digits:
			for i := 0; i < digits; i++ {
				if l.Done() {
					break escapeLoop
				}
				r = l.Peek()

				value *= 16
				switch {
				case r >= '0' && r <= '9':
					value |= uint32(r) - '0'
				case r >= 'a' && r <= 'f':
					value |= uint32(r) - 'a' + 10
				case r >= 'A' && r <= 'F':
					value |= uint32(r) - 'A' + 10
				default:
					break digits
				}
				_ = l.Pop()

				consumed++
			}

			escape := l.NewSpan(start, l.cursor)
			if consumed == 0 {
				errs.Error(ErrInvalidEscape{Span: escape})
			} else if r != 'x' {
				if consumed != digits || !utf8.ValidRune(rune(value)) {
					errs.Error(ErrInvalidEscape{Span: escape})
				}
			}

			if r == 'x' {
				buf.WriteByte(byte(value))
			} else {
				buf.WriteRune(rune(value))
			}
		default:
			escape := l.NewSpan(start, l.cursor)
			errs.Error(ErrInvalidEscape{Span: escape})
		}
	}

	token := l.PushToken(l.cursor-start, TokenString)
	if haveEsc {
		l.literals[token.raw] = buf.String()
	}

	quoted := token.Text()
	if quoted[0] != quoted[len(quoted)-1] {
		errs.Error(ErrUnterminatedStringLiteral{Token: token})
	}

	return token
}

// Done returns whether or not we're done lexing runes.
func (l *lexer) Done() bool {
	return l.Rest() == ""
}

// Rest returns unlexed text.
func (l *lexer) Rest() string {
	return l.Text()[l.cursor:]
}

// Peek peeks the next character; returns that character and its length.
//
// Returns -1 if l.Done().
func (l *lexer) Peek() rune {
	return decodeRune(l.Rest())
}

// Pop consumes the next character; returns that character and its length.
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

func isASCIIIdent(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}
