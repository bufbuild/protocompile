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
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
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
			prefix = digits[:1]
			base = 8
			legacyOctal = true
		} else {
			prefix = ""
			base = 10
		}
	}

	if prefix != "" {
		token.MutateMeta[tokenmeta.Number](tok).Prefix = uint32(len(prefix))
	}

	what := taxa.Int
	result, ok := parseInt(digits, base)
	switch {
	case !ok:
		if !taxa.IsFloatText(digits) {
			l.Error(errInvalidNumber{Token: tok})
			// Need to set a value to avoid parse errors in Token.AsInt.
			token.MutateMeta[tokenmeta.Number](tok)
			return tok
		}

		what = taxa.Float
		if legacyOctal {
			base = 10
		}

		meta := token.MutateMeta[tokenmeta.Number](tok)
		meta.FloatSyntax = true
		if base != 10 {
			// TODO: We should return ErrInvalidBase here but that requires
			// validating the syntax of the float to distinguish it from
			// cases where we want tor return ErrInvalidNumber instead.
			l.Error(errInvalidNumber{Token: tok})
			meta.Float = math.NaN()
			return tok
		}

		// We want this to overflow to Infinity as needed, which ParseFloat
		// will do for us. Otherwise it will ties-to-even as the
		// protobuf.com spec requires.
		//
		// ParseFloat itself says it "returns the nearest floating-point
		// number rounded using IEEE754 unbiased rounding", which is just a
		// weird, non-standard way to say "ties-to-even".
		value, err := strconv.ParseFloat(strings.ReplaceAll(digits, "_", ""), 64)

		//nolint:errcheck // The strconv package guarantees this assertion.
		if err != nil && err.(*strconv.NumError).Err == strconv.ErrSyntax {
			l.Error(errInvalidNumber{Token: tok})
			meta.Float = math.NaN()
		} else {
			meta.Float = value
		}

	case result.big != nil:
		token.MutateMeta[tokenmeta.Number](tok).Big = result.big

	case base == 10 && !result.hasThousands:
		// We explicitly do not call SetValue for the most common case of base
		// 10 integers, because that is handled for us on-demand in AsInt. This
		// is a memory consumption optimization.

	default:
		token.MutateMeta[tokenmeta.Number](tok).Word = result.small
	}

	var validBase bool
	switch base {
	case 2:
		validBase = false
	case 8:
		validBase = legacyOctal
	case 10:
		validBase = true
	case 16:
		validBase = what == taxa.Int
	}

	if validBase {
		if result.hasThousands {
			span := tok.Span()
			l.Errorf("%s contains underscores", what).Apply(
				report.SuggestEdits(tok, "remove these underscores", report.Edit{
					Start:   0,
					End:     len(span.Text()),
					Replace: strings.ReplaceAll(span.Text(), "_", ""),
				}),
				report.Notef("Protobuf does not support Go/Java/Rust-style thousands separators"),
			)
		}
		return tok
	}

	// Diagnose against number literals we currently accept but which are not
	// part of Protobuf.
	err := l.Errorf("unsupported base for %s", what)

	if what == taxa.Int {
		switch base {
		case 2:
			value, _ := tok.AsNumber().AsBig()
			err.Apply(
				report.SuggestEdits(tok, "use a hexadecimal literal instead", report.Edit{
					Start:   0,
					End:     len(tok.Text()),
					Replace: fmt.Sprintf("%#x", value),
				}),
				report.Notef("Protobuf does not support binary literals"),
			)
			return tok

		case 8:
			err.Apply(
				report.SuggestEdits(tok, "remove the `o`", report.Edit{Start: 1, End: 2}),
				report.Notef("octal literals are prefixed with `0`, not `0o`"),
			)
			return tok
		}
	}

	var name string
	switch base {
	case 2:
		name = "binary"
	case 8:
		name = "octal"
	case 16:
		name = "hexadecimal"
	}

	err.Apply(
		report.Snippet(tok),
		report.Notef("Protobuf does not support %s %s", name, what),
	)

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

// errInvalidNumber diagnoses a numeric literal with invalid syntax.
type errInvalidNumber struct {
	Token token.Token // The offending number token.
}

// Diagnose implements [report.Diagnose].
func (e errInvalidNumber) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("unexpected characters in %s", taxa.Classify(e.Token)),
		report.Snippet(e.Token),
	)

	// TODO: This is a pretty terrible diagnostic. We should at least add a note
	// specifying the correct syntax. For example, there should be a way to tell
	// that the invalid character is an out-of-range digit.
}
