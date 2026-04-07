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

package decimal

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"strconv"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

// Parse parses a decimal value from the given string, without losing precision.
//
// Parsing is done as follows. First, a leading '+' or '-' is removed. Then,
// a leading '0x' or '0X' is removed, which indicates a hexadecimal literal.
// Then, digits in the appropriate base are parsed, accounting for at most one
// decimal digit. Then, one of 'e', 'E', 'p', or 'P' indicates a signed exponent
// in base 10 or 2, respectively.
//
// If z is not nil, its storage is re-used for this parse.
func (z *Decimal) Parse(s string) (*Decimal, error) {
	if s == "" {
		return z, syntaxf("empty input")
	}
	if z == nil {
		z = new(Decimal)
	}

	z.lockFloat(true)
	defer z.float.Store(nil)
	z.clear()

	i := 0
	switch s[0] {
	case '-':
		z.neg = true
		fallthrough
	case '+':
		i++
	}

	base := 10
	places := 1 // Number of places in base that a digit corresponds to.
	if len(s)-i >= 2 && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		base = 16
		i += 2
		places = 4 // 0xf normalizes to 0x0.fp+4
		z.base2 = true
	}

	var dot, nonzero bool
	stop := len(s)
	skip := 0
mant:
	for ; i < stop; i++ {
		b := s[i]
		switch b {
		case '_':
			continue
		case '.':
			if dot {
				return z, syntaxf("extra decimal dot")
			}
			dot = true

			// If we have a decimal dot, we need to avoid trailing zeros.
			// We look ahead now to figure out where that is going to be, so
			// we can avoid redundant multiplications.
			//
			// After this loop, s[skip] either the end of the string or the
			// start of the exponent.
			skip = i
		lookahead:
			for ; skip < len(s); skip++ {
				switch s[skip] {
				case 'e', 'E':
					if base == 10 {
						break lookahead
					}
				case 'p', 'P':
					break lookahead
				}
			}

			// Back up from skip, counting trailing zeros.
			stop = skip
			for ; stop > 0; stop-- {
				switch s[stop-1] {
				case '0', '_':
					continue
				}
				break
			}

			continue
		case 'e', 'E':
			if base == 10 {
				break mant
			}
		case 'p', 'P':
			break mant
		}

		d, ok := unicodex.Digit(rune(b), byte(base))
		if !ok {
			r, _ := stringsx.Rune(s, i)
			return z, syntaxf("unrecognized rune: %c", r)
		}

		places := places
		if !nonzero && d != 0 {
			nonzero = true

			if base == 16 {
				// For the first nonzero digit in hex, we need places to be
				// the bit length of the digit. This ensures that, for example,
				// 0x1 has exponent 1, not 4.
				places = bits.Len(uint(d))
			}
		}

		if !dot && nonzero {
			z.exp += int32(places)
		} else if dot && !nonzero {
			z.exp -= int32(places)
		}

		m := z.get()
		m = bigx.FMAScalar(m, m, big.Word(base), big.Word(d))
		z.set(m)
	}

	if skip > 0 {
		i = skip
	}

	switch len(s) - i {
	case 0:
		return z, nil
	case 1:
		return z, syntaxf("expected exponent")
	}

	if s[i] == 'p' || s[i] == 'P' {
		z.base2 = true
	}
	i++

	expSign := 1
	switch s[i] {
	case '-':
		expSign = -1
		fallthrough
	case '+':
		i++
	}
	if i == len(s) {
		return z, nil
	}

	var exp int
	for ; i < len(s); i++ {
		b := s[i]
		if b == '_' {
			continue
		}

		d, ok := unicodex.Digit(rune(b), byte(base))
		if !ok {
			r, _ := stringsx.Rune(s, i)
			return z, syntaxf("unrecognized rune: %c", r)
		}

		exp = bitsx.MulSaturate(exp, 10)
		exp = bitsx.AddSaturate(exp, int(d))
	}

	exp *= expSign
	exp = bitsx.AddSaturate(exp, int(z.exp))
	if exp > math.MaxInt32 || exp < math.MinInt32 {
		neg := z.neg
		z.clear()
		if exp > 0 {
			z.inf = true
		}
		z.neg = neg
		return z, &parseError{err: strconv.ErrRange}
	}

	z.exp = int32(exp)

	return z, nil
}

// parseError is returned by [Parse] on failure.
type parseError struct {
	message string
	err     error
}

func (p *parseError) Error() string {
	if p.message == "" {
		return fmt.Sprintf("Decimal.Parse: %s", p.err)
	}
	return fmt.Sprintf("Decimal.Parse: %s: %s", p.err, p.message)
}

func (p *parseError) Unwrap() error {
	return p.err
}

func syntaxf(format string, args ...any) error {
	return &parseError{
		message: fmt.Sprintf(format, args...),
		err:     strconv.ErrSyntax,
	}
}
