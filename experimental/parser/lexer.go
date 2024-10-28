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
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/rivo/uniseg"
)

// Lex performs lexical analysis on the file contained in ctx, and appends any
// diagnostics that results in to l.
func Lex(ctx token.Context, errs *report.Report) {
	lexer := &lexer{
		Context: ctx,
		Stream:  ctx.Stream(),
		Report:  errs,
	}
	lexer.Lex()
}

// lexer is a Protobuf lexer.
type lexer struct {
	token.Context
	*token.Stream // Embedded so we don't have to call Stream() everywhere.
	*report.Report

	// This is outlined so that it's easy to print in the panic handler.
	lexerState
}

type lexerState struct {
	cursor, count int
	openStack     []token.Token
}

// Lex performs lexical analysis, and places any diagnostics in report.
func (l *lexer) Lex() {
	defer func() {
		if panicked := recover(); panicked != nil {
			panic(fmt.Sprintf("panic while lexing: %s; %#v", panicked, l.lexerState))
		}
	}()

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

	var prevCount int
	for !l.Done() {
		start := l.cursor
		r := l.Pop()

		if prevCount > 0 && prevCount == l.count {
			panic(fmt.Sprintf("protocompile/ast: lexer failed to make progress at offset %d; this is a bug in protocompile", l.cursor))
		}
		prevCount = l.count

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
				l.Error(ErrUnterminated{Span: l.SpanFrom(l.cursor - 2)})
				text = l.SeekEOF()
			}
			l.Push(len("/*")+len(text), token.Comment)
		case r == '*' && l.Peek() == '/':
			l.cursor++ // Skip the /.

			// The user definitely thought nested comments were allowed. :/
			tok := l.Push(len("*/"), token.Unrecognized)
			l.Error(ErrUnterminated{Span: tok.Span()})

		case strings.ContainsRune(";,/:=-", r): // . is handled elsewhere.
			// Random punctuation that doesn't require special handling.
			l.Push(utf8.RuneLen(r), token.Punct)

		case strings.ContainsRune("([{<", r): // Push the opener, close it later.
			tok := l.Push(utf8.RuneLen(r), token.Punct)
			l.openStack = append(l.openStack, tok)
		case strings.ContainsRune(")]}>", r):
			tok := l.Push(utf8.RuneLen(r), token.Punct)
			if len(l.openStack) == 0 {
				l.Error(ErrUnterminated{Span: tok.Span()})
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
				if tok.Text() != expected {
					l.Error(ErrUnterminated{Span: l.openStack[end].Span(), Mismatch: tok.Span()})
				}

				token.Fuse(l.openStack[end], tok)
				l.openStack = l.openStack[:end]
			}

		case r == '"', r == '\'':
			l.cursor-- // Back up to behind the quote before resuming.
			l.LexString()

		case r == '.':
			// A . is normally a single token, unless followed by a digit, which makes it
			// into a digit.
			if r := l.Peek(); !unicode.IsDigit(r) {
				l.Push(1, token.Punct)
				continue
			}
			fallthrough
		case unicode.IsDigit(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			l.LexNumber()

		case r == '_' || xidStart(r):
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)
			rawId := l.TakeWhile(xidContinue)

			// Eject any trailing unprintable characters.
			id := strings.TrimRightFunc(rawId, func(r rune) bool {
				return !unicode.IsPrint(r)
			})
			if id == "" {
				// This "identifier" appears to consist entirely of unprintable
				// characters (e.g. combining marks).
				tok := l.Push(len(rawId), token.Unrecognized)
				l.Error(ErrUnrecognized{Token: tok})
				continue
			}

			l.cursor -= len(rawId) - len(id)
			tok := l.Push(len(id), token.Ident)

			// Legalize non-ASCII runes.
			if !isASCIIIdent(tok.Text()) {
				l.Error(ErrNonASCIIIdent{Token: tok})
			}

		default:
			// Back up behind the rune we just popped.
			l.cursor -= utf8.RuneLen(r)

			// Consume as many grapheme clusters as possible, and diagnose it.
			unknown := l.TakeGraphemesWhile(func(g string) bool {
				r, _ := utf8.DecodeRuneInString(g)
				return !strings.ContainsRune(";,/:=-.([{<>}])_\"'", r) &&
					!xidContinue(r) &&
					!unicode.In(r, unicode.Pattern_White_Space)
			})
			tok := l.Push(len(unknown), token.Unrecognized)
			l.Error(ErrUnrecognized{Token: tok})
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range l.openStack {
		l.Error(ErrUnterminated{Span: open.Span()})
	}
	// In backwards order, generate empty tokens to fuse with
	// the unclosed delimiters.
	for i := len(l.openStack) - 1; i >= 0; i-- {
		empty := l.Push(0, token.Unrecognized)
		token.Fuse(l.openStack[i], empty)
	}

	// Perform implicit string concatenation.
	catStrings := func(start, end token.Token) {
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
		if tok.IsSynthetic() {
			return false
		}

		switch tok.Kind() {
		case token.Space, token.Comment:
			break
		case token.String:
			if start.Nil() {
				start = tok
			} else {
				end = tok
			}
		default:
			if !start.Nil() && !end.Nil() {
				catStrings(start, end)
			}
			start = token.Nil
			end = token.Nil
		}
		return true
	})
	if !start.Nil() && !end.Nil() {
		catStrings(start, end)
	}
}

func (l *lexer) Push(length int, kind token.Kind) token.Token {
	l.count++
	return l.Stream.Push(length, kind)
}

// LexString lexes a number starting at the current cursor.
func (l *lexer) LexNumber() token.Token {
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
	tok := l.Push(len(digits), token.Number)

	// Delete all _s from digits and normalize to lowercase.
	digits = strings.ToLower(strings.ReplaceAll(digits, "_", ""))

	// Now, let's see if this is actually a valid number that needs a diagnostic.
	// First, try to reify it as a uint64.
	switch {
	case strings.HasPrefix(digits, "0x"):
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0x"), 16, 64)
		if err == nil {
			token.SetValue(tok, value)
			return tok
		}

		// Emit a diagnostic. Which diagnostic we emit depends on the error Go
		// gave us. NB: all ParseUint errors are *strconv.NumError, as promised
		// by the documentation.
		if err.(*strconv.NumError).Err == strconv.ErrRange {
			l.Error(ErrIntegerOverflow{Token: tok})
		} else {
			l.Error(ErrInvalidNumber{Token: tok})
		}
	case strings.HasPrefix(digits, "0") && strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '7' }) == -1:
		// Annoyingly, 0777 is octal, but 0888 is not, so we have to handle this case specially.
		fallthrough
	case strings.HasPrefix(digits, "0o"): // Rust/Python-style octal ints are an extension.
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0o"), 8, 64)
		if err == nil {
			token.SetValue(tok, value)
			return tok
		}

		if err.(*strconv.NumError).Err == strconv.ErrRange {
			l.Error(ErrIntegerOverflow{Token: tok})
		} else {
			l.Error(ErrInvalidNumber{Token: tok})
		}
	case strings.HasPrefix(digits, "0b"): // Binary ints are an extension.
		value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0b"), 2, 64)
		if err == nil {
			token.SetValue(tok, value)
			return tok
		}

		if err.(*strconv.NumError).Err == strconv.ErrRange {
			l.Error(ErrIntegerOverflow{Token: tok})
		} else {
			l.Error(ErrInvalidNumber{Token: tok})
		}
	}

	// This is either a float or a decimal number. Try parsing as decimal first, otherwise
	// try parsing as float.
	_, err := strconv.ParseUint(digits, 10, 64)
	if err == nil {
		// This is the most common result. It gets computed ON DEMAND by token.Token.AsNumber.
		// DO NOT place it into l.literals.
		return tok
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
		token.SetValue(tok, value)
		return tok
	}

	// If it was not well-formed, it might be a float.
	value, err := strconv.ParseFloat(digits, 64)

	// This time, the syntax might be invalid. If it is, we decide whether
	// this is a bad float or a bad integer based on whether it contains
	// periods.
	if err.(*strconv.NumError).Err == strconv.ErrSyntax {
		l.Error(ErrInvalidNumber{Token: tok})
	}

	// If the error is ErrRange we don't care, it will clamp to Infinity in
	// the way we want, as noted by the comment above.
	token.SetValue(tok, value)
	return tok
}

// LexString lexes a string starting at the current cursor.
//
// The cursor position should be just before the string's first quote character.
func (l *lexer) LexString() token.Token {
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
			l.Warnf("non-printable character in string literal").With(
				report.Snippetf(l.SpanFrom(l.cursor-utf8.RuneLen(r)), "this is the rune U+%04x", r),
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
		case 't':
			buf.WriteByte('\t')
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

			escape := l.SpanFrom(start)
			if consumed == 0 {
				l.Error(ErrInvalidEscape{Span: escape})
			} else if r != 'x' {
				if consumed != digits || !utf8.ValidRune(rune(value)) {
					l.Error(ErrInvalidEscape{Span: escape})
				}
			}

			if r == 'x' {
				buf.WriteByte(byte(value))
			} else {
				buf.WriteRune(rune(value))
			}
		default:
			escape := l.SpanFrom(start)
			l.Error(ErrInvalidEscape{Span: escape})
		}
	}

	tok := l.Push(l.cursor-start, token.String)
	if haveEsc {
		token.SetValue(tok, buf.String())
	}

	quoted := tok.Text()
	if quoted[0] != quoted[len(quoted)-1] {
		l.Error(ErrUnterminatedStringLiteral{Token: tok})
	}

	return tok
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

// TakeWhile consumes grapheme clusters while they match the given function.
// Returns consumed characters.
func (l *lexer) TakeGraphemesWhile(f func(string) bool) string {
	start := l.cursor

	for gs := uniseg.NewGraphemes(l.Rest()); gs.Next(); {
		g := gs.Str()
		if !f(g) {
			break
		}
		l.cursor += len(g)
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
