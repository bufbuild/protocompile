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
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/token"
)

// lexNumber lexes a number starting at the current cursor.
func lexNumber(l *lexer) token.Token {
	tok := lexRawNumber(l)
	digits := tok.Text()

	// Select the correct base we're going to be parsing.
	var (
		prefix      string
		base        byte
		legacyOctal bool // Whether this is a C-style 0777 octal.
	)
	if len(digits) >= 2 {
		prefix = digits[:2]
	}
	switch prefix {
	case "0b", "0B":
		digits = digits[2:]
		base = 2
	case "0o", "0O":
		digits = digits[2:]
		base = 8
	case "0x", "0X":
		digits = digits[2:]
		base = 16
	default:
		if strings.HasPrefix(digits, "0") {
			base = 8
			legacyOctal = true
		} else {
			base = 10
		}
	}

	result, ok := parseInt(digits, base)
	if !ok {
		// This may be a floating-point number. Confirm this by hunting
		// for a for a decimal point or an exponent.
		if strings.ContainsAny(digits, ".Ee") {
			if legacyOctal {
				base = 10
			}

			if base != 10 {
				// TODO: We should return ErrInvalidBase here but that requires
				// validating the syntax of the float to distinguish it from
				// cases where we want tor return ErrInvalidNumber instead.
				l.Error(ErrInvalidNumber{Token: tok})
				token.SetValue(tok, math.NaN())
				return tok
			}

			// Delete all underscores.
			filteredDigits := strings.ReplaceAll(digits, "_", "")
			hasThousands := len(filteredDigits) < len(digits)

			// We want this to overflow to Infinity as needed, which ParseFloat
			// will do for us. Otherwise it will ties-to-even as the
			// protobuf.com spec requires.
			//
			// ParseFloat itself says it "returns the nearest floating-point
			// number rounded using IEEE754 unbiased rounding", which is just a
			// weird, non-standard way to say "ties-to-even".
			value, err := strconv.ParseFloat(digits, 64)

			//nolint:errcheck // The strconv package guarantees this assertion.
			if err != nil && err.(*strconv.NumError).Err == strconv.ErrSyntax {
				l.Error(ErrInvalidNumber{Token: tok})
				token.SetValue(tok, math.NaN())
			} else {
				token.SetValue(tok, value)

				if hasThousands {
					// Diagnose any thousands separators. We parse it as an
					// extension currently.
					l.Error(ErrThousandsSep{Token: tok})
				}
			}
		} else {
			l.Error(ErrInvalidNumber{Token: tok})
			// Need to set a value to avoid parse errors in Token.AsInt.
			token.SetValue(tok, uint64(0))
		}

		return tok
	}

	if result.big != nil {
		token.SetValue(tok, result.big)
	} else if base != 10 || result.hasThousands {
		// We explicitly do not call SetValue for the most common case of base
		// 10 integers, because that is handled for us on-demand in AsInt. This
		// is a memory consumption optimization.
		token.SetValue(tok, result.small)
	}

	// Diagnose against number literals we currently accept but which are not
	// part of Protobuf.
	if base == 2 || (base == 8 && !legacyOctal) {
		l.Error(ErrInvalidBase{
			Token: tok,
			Base:  int(base),
		})
	} else if result.hasThousands {
		l.Error(ErrThousandsSep{Token: tok})
	}

	return tok
}

// lexRawNumber lexes a raw number per the rules at
// https://protobuf.com/docs/language-spec#numeric-literals
func lexRawNumber(l *lexer) token.Token {
	start := l.cursor

	for !l.Done() {
		r := l.Peek()
		//nolint:gocritic // This trips a noisy "use a switch" lint that makes
		// this code less readable.
		if r == 'e' || r == 'E' {
			l.Pop()
			r = l.Peek()
			if r == '+' || r == '-' {
				l.Pop()
			}
		} else if r == '.' || unicode.IsDigit(r) || unicode.IsLetter(r) ||
			// We consume _ because 0_n is not valid in any context, so we
			// can offer _ digit separators as an extension.
			r == '_' {
			l.Pop()
		} else {
			break
		}
	}

	// Create the token, even if this is an invalid number. This will help
	// the parser pick up bad numbers into number literals.
	digits := l.Text()[start:l.cursor]
	return l.Push(len(digits), token.Number)
}
