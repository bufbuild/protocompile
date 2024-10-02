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

package ast2

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/report2"
)

// lexer is a Protobuf lexer.
type lexer struct {
	*Context
	cursor    int
	openStack []Token
}

// Lex performs lexical analysis, and places any diagnostics in report.
func (l *lexer) Lex(report *report2.Report) {
	for l.Rest() != "" {
		r, rLen := l.Peek()
		start := l.cursor

		switch {
		case unicode.IsSpace(r):
			// Whitepace. Consume as much whitespace as possible and mint a
			// whitespace token.
			for {
				l.cursor += rLen
				r, rLen = l.Peek()
				if r == utf8.RuneError || !unicode.IsSpace(r) {
					break
				}
			}
			l.PushToken(l.cursor-start, TokenSpace)
		case l.HasPrefix("//"):
			// Single-line comment. Seek to the next '\n' or the EOF.
			if comment, ok := l.SeekInclusive("\n"); ok {
				l.PushToken(len(comment), TokenComment)
			} else {
				l.PushToken(len(l.SeekEOF()), TokenComment)
			}
		case l.HasPrefix("/*"):
			// Block comment. Seek to the next "*/". Protobuf comments
			// unfortunately do not nest, and allowing them to nest can't
			// be done in a backwards-compatible manner. We acknowledge that
			// this behavior is user-hostile.
			//
			// If we encounter no "*/", seek EOF and emit a diagnostic. Trying
			// to lex a partial comment is hopeless.
			if comment, ok := l.SeekInclusive("*/"); ok {
				l.PushToken(len(comment), TokenComment)
			} else {
				report.Error(
					ErrUnterminatedBlockComment,
					// Create a span for the /*, that's what we're gonna highlight.
					report2.Snippet(l.NewSpan(l.cursor, l.cursor+2), ""),
				)
				l.PushToken(len(l.SeekEOF()), TokenComment)
			}
		case l.HasPrefix("*/"):
			// The user definitely thought nested comments were allowed. :/
			report.Error(
				ErrUnterminatedBlockComment,
				report2.Snippet(l.NewSpan(l.cursor, l.cursor+2), ""),
				report2.Note("Protobuf does not permit nested block comments"),
			)
			l.PushToken(2, TokenUnrecognized)
		case strings.ContainsRune(";,/:=-", r): // . is handled elsewhere.
			// Random punctuation that doesn't require special handling.
			l.cursor += rLen
			l.PushToken(rLen, TokenPunct)

		case strings.ContainsRune("([{<", r): // Push the opener, close it later.
			l.cursor += rLen
			token := l.PushToken(rLen, TokenPunct)
			l.openStack = append(l.openStack, token)
		case strings.ContainsRune(")]}>", r):
			l.cursor += rLen
			token := l.PushToken(rLen, TokenPunct)
			if len(l.openStack) == 0 {
				report.Error(ErrUnopenedDelimiter, report2.Snippet(token, ""))
			} else {
				end := len(l.openStack) - 1
				l.FuseTokens(l.openStack[end], token)
				l.openStack = l.openStack[:end]
			}

		case r == '"', r == '\'':
			// Quoted strings.
			q := r
			l.cursor += rLen

			// Seek to the end of the string, unescaping as we go. We do not
			// materialize an unescaped string if this string does not require escaping.
			var buf strings.Builder
			var haveEsc bool
			for {
				r, rLen = l.Peek()
				if r == utf8.RuneError {
					goto unterminated
				}

				l.cursor += rLen
				if r == q {
					break
				}

				// Warn if the user has a non-printable character in their string that isn't
				// ASCII whitespace.
				if !unicode.IsGraphic(r) && !strings.ContainsRune(" \n\t\r", r) {
					report.Warn(
						fmt.Errorf("non-ASCII space non-printable character in string literal"),
						report2.Snippet(l.NewSpan(l.cursor-rLen, l.cursor), "this is the rune U+%04x", r),
					)
				}

				if r != '\\' {
					// We intentionally do not legalize against \0 and \n. The above warning
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
					buf.WriteString(l.Text()[start : l.cursor-1])
					haveEsc = true
				}

				r, rLen = l.Peek()
				l.cursor += rLen
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
					for range 2 {
						r, rLen = l.Peek()
						if r == utf8.RuneError {
							goto unterminated
						}

						if r < '0' || r > '7' {
							break
						}

						l.cursor += rLen
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
					for range digits {
						r, rLen = l.Peek()
						if r == utf8.RuneError {
							goto unterminated
						}

						switch {
						case r >= '0' && r <= '9':
							value |= uint32(r) - '0'
						case r >= 'a' && r <= 'f':
							value |= uint32(r) - 'a'
						case r >= 'A' && r <= 'F':
							value |= uint32(r) - 'A'
						default:
							break digits
						}

						l.cursor += rLen
						value *= 16
						consumed++
					}

					if r == 'x' && consumed == 0 {
						report.Error(
							ErrInvalidEscape,
							report2.Snippet(
								l.NewSpan(start, l.cursor),
								"`\\x` must be followed by at least one hex digit",
							),
						)
					} else if digits != consumed {
						report.Error(
							ErrInvalidEscape,
							report2.Snippet(
								l.NewSpan(start, l.cursor),
								"`\\%c` must be followed by exactly %d hex digits",
								r, consumed,
							),
						)
					}

					if r == 'x' {
						buf.WriteByte(byte(value))
					} else if !utf8.ValidRune(rune(value)) {
						report.Error(
							ErrInvalidEscape,
							report2.Snippet(
								l.NewSpan(start+2, l.cursor),
								"must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF",
							),
						)
					} else {
						buf.WriteRune(rune(value))
					}
				default:
					report.Error(ErrInvalidEscape, report2.Snippet(l.NewSpan(start+2, l.cursor), ""))
				}

			unterminated:
				report.Error(
					ErrUnterminatedStringLiteral,
					report2.Snippet(l.NewSpan(l.cursor, l.cursor+2), ""),
				)
				break
			}

			token := l.PushToken(l.cursor-start, TokenString)
			if haveEsc {
				l.literals[token.id] = buf.String()
			}
		case r == '.':
			// A . is normally a single token, unless followed by a digit, which makes it
			// into a digit.
			if r, _ := l.RuneAt(1); !unicode.IsDigit(r) {
				l.cursor += rLen
				l.PushToken(1, TokenPunct)
				continue
			}

			fallthrough
		case unicode.IsDigit(r): // Accept all digits, legalize later.
			// Consume the largest prefix that satisfies the rules at
			// https://protobuf.com/docs/language-spec#numeric-literals
		more:
			l.cursor += rLen
			r, rLen = l.Peek()
			if r == 'e' || r == 'E' {
				r2, _ := l.RuneAt(1)
				if r2 == '+' || r2 == '-' {
					rLen = 2
				}
				goto more
			}
			if r == '.' || unicode.IsDigit(r) || unicode.IsLetter(r) ||
				// We consume _ because 0_n is not valid in any context, so we
				// can offer _ digit separators as an extension.
				r == '_' {
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
					l.literals[token.id] = value
					continue
				}

				// Emit a diagnostic. Which diagnostic we emit depends on the error Go
				// gave us. NB: all ParseUint errors are *strconv.NumError, as promised
				// by the documentation.
				why := err.(*strconv.NumError).Err
				if why == strconv.ErrSyntax {
					if why == strconv.ErrRange {
						report.Error(
							ErrIntegerOverflow,
							report2.Snippet(token, "must be in the range `0x0` to `0x%x`", uint64(math.MaxUint64)),
							report2.Note("hexadecimal literals must always fit in a uint64"),
						)
					} else if strings.Contains(digits, ".") {
						// The user might have thought we support hex floats like Go and C do.
						report.Error(
							ErrInvalidFloatLiteral,
							report2.Snippet(token, ""),
							report2.Note("Protobuf does not support hexadecimal floats (e.g., `0x1.ffp45`)"),
						)
					} else {
						report.Error(
							ErrInvalidHexLiteral,
							report2.Snippet(token, ""),
						)
					}
				}
			case strings.HasPrefix(digits, "0") && strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '7' }) == -1:
				// Annoyingly, 0777 is octal, but 0888 is not, so we have to handle this case specially.
				fallthrough
			case strings.HasPrefix(digits, "0o"): // Rust/Python-style octal ints are an extension.
				value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0o"), 8, 64)
				if err == nil {
					l.literals[token.id] = value
					continue
				}

				// Emit a diagnostic. Which diagnostic we emit depends on the error Go
				// gave us. NB: all ParseUint errors are *strconv.NumError, as promised
				// by the documentation.
				why := err.(*strconv.NumError).Err
				if why == strconv.ErrSyntax {
					if why == strconv.ErrRange {
						report.Error(
							ErrIntegerOverflow,
							report2.Snippet(token, "must be in the range `0` to `0%o`", uint64(math.MaxUint64)),
							report2.Note("octal literals must always fit in a uint64"),
						)
					} else if strings.Contains(digits, ".") {
						report.Error(
							ErrInvalidFloatLiteral,
							report2.Snippet(token, ""),
							report2.Note("Protobuf does not support octal floats"),
						)
					} else {
						report.Error(
							ErrInvalidOctLiteral,
							report2.Snippet(token, ""),
						)
					}
				}
			case strings.HasPrefix(digits, "0b"): // Binary ints are an extension.
				value, err := strconv.ParseUint(strings.TrimPrefix(digits, "0b"), 2, 64)
				if err == nil {
					l.literals[token.id] = value
					continue
				}

				// Emit a diagnostic. Which diagnostic we emit depends on the error Go
				// gave us. NB: all ParseUint errors are *strconv.NumError, as promised
				// by the documentation.
				why := err.(*strconv.NumError).Err
				if why == strconv.ErrSyntax {
					if why == strconv.ErrRange {
						report.Error(
							ErrIntegerOverflow,
							report2.Snippet(token, "must be in the range `0` to `0b%b`", uint64(math.MaxUint64)),
							report2.Note("octal literals must always fit in a uint64"),
						)
					} else if strings.Contains(digits, ".") {
						report.Error(
							ErrInvalidFloatLiteral,
							report2.Snippet(token, ""),
							report2.Note("Protobuf does not support binary floats"),
						)
					} else {
						report.Error(
							ErrInvalidBinLiteral,
							report2.Snippet(token, ""),
						)
					}
				}
			default:
				// This is either a float or a decimal number. Try parsing as decimal first, otherwise
				// try parsing as float.
				_, err := strconv.ParseUint(digits, 10, 64)
				if err == nil {
					// This is the most common result. It gets computed ON DEMAND by Token.AsNumber.
					// DO NOT place it into l.literals.
					continue
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
					l.literals[token.id] = value
					continue
				}

				// If it was not well-formed, it might be a float.
				value, err := strconv.ParseFloat(digits, 64)

				// This time, the syntax might be invalid. If it is, we decide whether
				// this is a bad float or a bad integer based on whether it contains
				// periods.
				if err.(*strconv.NumError).Err == strconv.ErrSyntax {
					if strings.Contains(digits, ".") {
						report.Error(
							ErrInvalidFloatLiteral,
							report2.Snippet(token, ""),
						)
					} else {
						report.Error(
							ErrInvalidDecLiteral,
							report2.Snippet(token, ""),
						)
					}
				}
				// If the error is ErrRange we don't care, it will clamp to Infinity in
				// the way we want, as noted by the comment above.
				l.literals[token.id] = value
			}

		case unicode.IsLetter(r): // Consume fairly-open-ended identifiers, legalize to ASCII later.
			for {
				r, rLen = l.Peek()
				if r != '_' && !unicode.IsDigit(r) && !unicode.IsLetter(r) {
					break
				}
				l.cursor += rLen
			}
			token := l.PushToken(l.cursor-start, TokenIdent)

			// Legalize non-ASCII runes.
			for _, r := range token.Text() {
				if r >= 0x80 {
					report.Error(ErrInvalidDecLiteral, report2.Snippet(token, ""))
					break
				}
			}

		default: // Consume as much stuff we don't understand as possible, diagnose it.
			for {
				r, rLen = l.Peek()
				if strings.ContainsRune(";,/:=-.([{<>}])_", r) || unicode.IsDigit(r) || unicode.IsLetter(r) {
					break
				}
				l.cursor += rLen
			}

			token := l.PushToken(l.cursor-start, TokenUnrecognized)
			report.Error(ErrUnrecognized, report2.Snippet(token, ""))
		}
	}

	// Legalize against unclosed delimiters.
	for _, open := range l.openStack {
		report.Error(ErrUnclosedDelimiter, report2.Snippet(open, ""))
	}

	// Perform implicit string concatenation.
	catStrings := func(start, end Token) {
		var buf strings.Builder
		for i := start.id; i <= end.id; i++ {
			tok := i.With(l.Context)
			if s, ok := tok.AsString(); ok {
				buf.WriteString(s)
				delete(l.literals, tok.id)
			}
		}
		l.literals[start.id] = buf.String()
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

// Rest returns unlexed text.
func (l *lexer) Rest() string {
	return l.Text()[l.cursor:]
}

// Peek peeks the next character; returns that character and its length.
func (l *lexer) Peek() (rune, int) {
	return utf8.DecodeRuneInString(l.Rest())
}

// RuneAt decodes a rune at the given offset from the cursor.
func (l *lexer) RuneAt(offset int) (rune, int) {
	return utf8.DecodeRuneInString(l.Rest()[offset:])
}

// HasPrefix checks if the given text exists past the cursor.
func (l *lexer) HasPrefix(prefix string) bool {
	return strings.HasPrefix(l.Rest(), prefix)
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
	l.cursor = len(rest)
	return rest
}
