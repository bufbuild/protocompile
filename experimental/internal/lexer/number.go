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
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

var (
	decFloat = fpRegexp("0-9", "eEpP")
	hexFloat = fpRegexp("0-9a-fA-F", "pP")
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
		if l.OldStyleOctal && strings.HasPrefix(digits, "0") {
			if strings.ContainsAny(digits, "123456789") {
				prefix = digits[:1]
				base = 8
				legacyOctal = true
				break
			}
		}

		prefix = ""
		base = 10
	}

	if base != 10 {
		token.MutateMeta[tokenmeta.Number](tok).Base = base
	}

	isFloat := taxa.IsFloatText(digits)
	expBase := 1
	if isFloat {
		switch {
		case base != 16 && strings.ContainsAny(digits, "eE"):
			expBase = 10
		case strings.ContainsAny(digits, "pP"):
			expBase = 2
		}
	}

	if expBase != 1 {
		token.MutateMeta[tokenmeta.Number](tok).ExpBase = byte(expBase)
	}

	// Peel a suffix off of digits consisting of characters not in the
	// desired base.
	haystack := digits
	suffixBase := base
	switch expBase {
	case 2:
		suffixBase = 10
		haystack = haystack[strings.IndexAny(digits, "pP")+1:]
	case 10:
		suffixBase = 10
		haystack = haystack[strings.IndexAny(digits, "eE")+1:]
	}

	suffixIdx := strings.IndexFunc(haystack, func(r rune) bool {
		if strings.ContainsRune("_.+-", r) {
			return false
		}
		_, ok := unicodex.Digit(r, suffixBase)
		return !ok
	})

	var suffix int
	if suffixIdx != -1 {
		suffix = len(haystack) - suffixIdx
	}
	digits = digits[:len(digits)-suffix]

	if prefix != "" {
		token.MutateMeta[tokenmeta.Number](tok).Prefix = uint32(len(prefix))
	}
	if suffix != 0 {
		token.MutateMeta[tokenmeta.Number](tok).Prefix = uint32(suffix)
	}

	result, ok := parseInt(digits, base)
	switch {
	case !ok:
		if l.scratchFloat == nil {
			l.scratchFloat = new(big.Float)
		}

		v := l.scratchFloat
		meta := token.MutateMeta[tokenmeta.Number](tok)
		if legacyOctal {
			base = 10
			meta.Prefix = 0
		}
		meta.Base = base
		meta.IsFloat = true

		var err error
		if base == 16 && hexFloat.MatchString(digits) {
			l.scratch = l.scratch[:0]
			l.scratch = append(l.scratch, "0x"...)
			l.scratch = append(l.scratch, digits...)
			digits := unsafex.StringAlias(l.scratch)
			v, _, err = v.Parse(digits, 0)
		} else if match := decFloat.FindStringSubmatch(digits); match != nil {
			if match[2] == "p" || match[2] == "P" {
				v, _, err = v.Parse(match[1], 0)

				exp, err := strconv.ParseInt(match[3], 10, 64)
				if err != nil {
					exp = math.MaxInt
				}
				exp += int64(v.MantExp(nil))
				v.SetMantExp(v, int(exp))
			} else {
				v, _, err = v.Parse(digits, 0)
			}
		} else {
			l.Error(errInvalidNumber{Token: tok})
			meta.SyntaxError = true
			return tok
		}

		if err != nil {
			l.Error(errInvalidNumber{Token: tok})
			meta.SyntaxError = true
		} else {
			// We want this to overflow to Infinity as needed, which ParseFloat
			// will do for us. Otherwise it will ties-to-even as the
			// protobuf.com spec requires.
			//
			// ParseFloat itself says it "returns the nearest floating-point
			// number rounded using IEEE754 unbiased rounding", which is just a
			// weird, non-standard way to say "ties-to-even".
			f64, acc := v.Float64()
			if acc != big.Exact {
				meta.Big = v
				l.scratchFloat = nil
			} else {
				meta.Word = math.Float64bits(f64)
				l.scratchFloat.SetUint64(0)
			}
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

	if result.hasThousands {
		token.MutateMeta[tokenmeta.Number](tok).ThousandsSep = true
	}

	return tok
}

// lexRawNumber lexes a raw number per the rules at
// https://protobuf.com/docs/language-spec#numeric-literals
func lexRawNumber(l *lexer) token.Token {
	start := l.cursor

	for !l.done() {
		r := l.peek()
		//nolint:gocritic // This trips a noisy "use a switch" lint that makes
		// this code less readable.
		if r == 'e' || r == 'E' {
			l.pop()
			r = l.peek()
			if r == '+' || r == '-' {
				l.pop()
			}
		} else if r == '.' || unicode.IsDigit(r) || unicode.IsLetter(r) ||
			// We consume _ because 0_n is not valid in any context, so we
			// can offer _ digit separators as an extension.
			r == '_' {
			l.pop()
		} else {
			break
		}
	}

	// Create the token, even if this is an invalid number. This will help
	// the parser pick up bad numbers into number literals.
	digits := l.Text()[start:l.cursor]
	return l.push(len(digits), token.Number)
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

// fpRegexp constructs a regexp for a float with the given digits and exponent
// characters.
func fpRegexp(digits string, exp string) *regexp.Regexp {
	block := func(digits string) string {
		// This ensures that underscores only appear between digits: either
		// the subpattern consisting of just digits matches, or the subpattern
		// containing underscores, but bookended by digits, matches.
		return fmt.Sprintf(`[%[1]s]+|[%[1]s][%[1]s_]+[%[1]s]`, digits)
	}

	pat := fmt.Sprintf(
		`^((?:%[1]s)?(?:\.(?:%[1]s)?)?)(?:([%[2]s])([+-]?%[3]s))?$`,
		block(digits), exp, block("0-9"),
	)

	return regexp.MustCompile(pat)
}
