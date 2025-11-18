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

	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

var (
	decFloat = fpRegexp("0-9", "eEpP")
	hexFloat = fpRegexp("0-9a-fA-F", "pP")

	// e and E are conspicuously missing here; this is so that 01e1 is treated
	// as a decimal float.
	treatAsOctal = regexp.MustCompile("^[0-9a-dfA-DF_]+$")
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
		if l.OldStyleOctal &&
			len(digits) >= 2 && digits[0] == '0' && // Note: `0` is not octal.
			treatAsOctal.MatchString(digits) { // Float-likes are not octal.
			prefix = digits[:1]
			base = 8
			legacyOctal = true
			break
		}

		prefix = ""
		base = 10
	}

	if base != 10 {
		token.MutateMeta[tokenmeta.Number](tok).Base = base
	}

	isFloat := taxa.IsFloatText(digits)
	expBase := 1
	expIdx := -1
	if isFloat {
		if expIdx = strings.IndexAny(digits, "pP"); expIdx != -1 {
			expBase = 2
		} else if expIdx = strings.IndexAny(digits, "eE"); expIdx != -1 {
			expBase = 10
		}
	}
	if expBase != 1 {
		token.MutateMeta[tokenmeta.Number](tok).ExpBase = byte(expBase)
	}

	// Peel a suffix off of digits consisting of characters not in the
	// desired base.
	haystack := digits
	suffixBase := base
	if expIdx != -1 {
		suffixBase = 10
		haystack = haystack[expIdx+1:]
	}

	suffixIdx := strings.IndexFunc(haystack, func(r rune) bool {
		if strings.ContainsRune("_.+-", r) {
			return false
		}
		_, ok := unicodex.Digit(r, suffixBase)
		return !ok
	})

	var suffix string
	if suffixIdx != -1 {
		suffix = haystack[suffixIdx:]

		// Check to see if we like this suffix.
		if l.IsAffix != nil && l.IsAffix(suffix, token.Number, true) {
			digits = digits[:len(digits)-len(suffix)]
		} else {
			suffix = ""
		}
	}

	if prefix != "" {
		token.MutateMeta[tokenmeta.Number](tok).Prefix = uint32(len(prefix))
	}
	if suffix != "" {
		token.MutateMeta[tokenmeta.Number](tok).Suffix = uint32(len(suffix))
	}
	if expIdx != -1 {
		// Example: 123e456suffix, want len("e456").
		//   len(digits) = 13
		//   expIdx      = 3
		//   suffix      = 6
		//
		// -> 13 - 6 - 3 = 13 - 9 = 4
		offset := len(digits) - expIdx - len(suffix)
		token.MutateMeta[tokenmeta.Number](tok).Exp = uint32(offset)
	}

	result, ok := parseInt(digits, base)
	switch {
	case !ok:
		if l.scratchFloat == nil {
			l.scratchFloat = new(big.Float)
		}

		v := l.scratchFloat
		meta := token.MutateMeta[tokenmeta.Number](tok)

		// Convert legacyOctal values that are *not* pure integers into decimal
		// floats.
		if legacyOctal && !treatAsOctal.MatchString(digits) {
			base = 10
			meta.Base = 10
			meta.Prefix = 0
		}
		meta.IsFloat = strings.ContainsAny(digits, ".-") // Positive exponents are not necessarily floats.
		meta.ThousandsSep = strings.Contains(digits, "_")

		// Wrapper over big.Float.Parse that ensures we never round. big.Float
		// does not have a parse mode that uses maximum precision for that
		// input, so we infer the correct precision from the size of the
		// mantissa and exponent.
		parse := func(v *big.Float, digits string) (*big.Float, error) {
			n := tok.AsNumber()
			mant := n.Mantissa().Text()
			exp := n.Exponent().Text()

			// Convert digits into binary digits. Note that digits in base b
			// is log_b(n). Note that, per the log base change formula:
			//
			// log_2(n) = log_b(n)/log_b(2)
			// log_b(2) = log_2(2)/log_2(b) = 1/log_2(b)
			//
			// Thus, we want to multiply by log_2(b), which we precompute in
			// a table.
			prec := float64(len(mant)) * log2Table[base-1]
			// If there is an exponent, add it to the precision.
			if exp != "" {
				// Convert to the right base and add it to prec.
				exp, _ := strconv.ParseInt(exp, 0, 64)
				prec += math.Abs(float64(exp)) * log2Table[expBase-1]
			}

			v.SetPrec(uint(math.Ceil(prec)))
			_, _, err := v.Parse(digits, 0)
			return v, err
		}

		var err error
		switch base {
		case 10:
			match := decFloat.FindStringSubmatch(digits)
			if match == nil {
				goto fail
			}

			if expBase != 2 {
				v, err = parse(v, digits)
				break
			}

			v, err = parse(v, match[1])
			exp, err := strconv.ParseInt(match[3], 10, 64)
			if err != nil {
				exp = math.MaxInt
			}
			exp += int64(v.MantExp(nil))
			v.SetMantExp(v, int(exp))

		case 16:
			if !hexFloat.MatchString(digits) {
				goto fail
			}

			l.scratch = l.scratch[:0]
			l.scratch = append(l.scratch, "0x"...)
			l.scratch = append(l.scratch, digits...)
			digits := unsafex.StringAlias(l.scratch)

			v, err = parse(v, digits)

		default:
			goto fail
		}

		if err != nil {
			goto fail
		}

		// We want this to overflow to Infinity as needed, which ParseFloat
		// will do for us. Otherwise it will ties-to-even as the
		// protobuf.com spec requires.
		//
		// ParseFloat itself says it "returns the nearest floating-point
		// number rounded using IEEE754 unbiased rounding", which is just a
		// weird, non-standard way to say "ties-to-even".
		if meta.IsFloat {
			f64, acc := v.Float64()
			if acc != big.Exact {
				meta.Big = v
				l.scratchFloat = nil
			} else {
				meta.Word = math.Float64bits(f64)
				l.scratchFloat.SetUint64(0)
			}
		} else {
			u64, acc := v.Uint64()
			if acc != big.Exact {
				meta.Big = v
				l.scratchFloat = nil
			} else {
				meta.Word = u64
				l.scratchFloat.SetUint64(0)
			}
		}
		return tok

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

fail:
	l.Error(errtoken.InvalidNumber{Token: tok})
	token.MutateMeta[tokenmeta.Number](tok).SyntaxError = true
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
