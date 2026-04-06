package decimal

import (
	"fmt"
	"math/big"
	"math/bits"
	"strconv"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
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

	z.lockFloat(false)
	defer z.float.Store(nil)

	z.exp = 0
	z.binExp = false
	z.neg = false

	if z.raw.data == nil {
		z.raw.data = &z.raw.cap
		z.raw.len = 0
		z.raw.cap = 0
	} else {
		m := z.get()[:1]
		m[0] = 0
		z.set(m)
	}

	i := 0
	switch s[0] {
	case '-':
		z.neg = true
		fallthrough
	case '+':
		i++
	}

	base := 10
	if len(s) >= 2 && s[0] == '0' && (s[0] == 'x' || s[0] == 'X') {
		base = 16
		s = s[2:]
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

			fmt.Printf("%q, %q\n", s[:stop], s[:skip])

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

		if d != 0 {
			nonzero = true
		}

		if !dot && nonzero {
			z.exp++
		} else if dot && !nonzero {
			z.exp--
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
		z.binExp = true
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

	var expMag uint
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

		var extra uint
		extra, expMag = bits.Mul(expMag, 10)
		if extra != 0 {
			return z, &parseError{err: strconv.ErrRange}
		}
		expMag, extra = bits.Add(expMag, uint(d), 0)
		if extra != 0 {
			return z, &parseError{err: strconv.ErrRange}
		}
	}

	z.exp += int(expMag) * expSign

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
