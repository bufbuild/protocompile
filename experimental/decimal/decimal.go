// Package decimal provides a big decimal type, i.e., a version of [big.Float]
// which works on a base 10 exponent, rather than base 2, allowing it to
// represent values such as 0.1 exactly.
package decimal

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"slices"
	"sync"
	"sync/atomic"
	"unicode"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/bytesx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var log5 = math.Log2(5)

type Decimal struct {
	// The mantissa. mant.abs may point to small to avoid an extra heap
	// allocation for cases where the value is in the range of a word.
	//
	// If mant.abs is nil, the value is exactly float.
	mant big.Int
	exp  int

	float atomic.Pointer[big.Float]
}

// Float calculates the closest floating-point value to this decimal.
func (z *Decimal) Float() *big.Float {
	f := z.float.Load()
	if f != nil {
		return f
	}
	defer z.float.CompareAndSwap(nil, f)

	// Essentially, we need to multiply z.mant by 5^z.exp to convert the
	// exponent.
	exp := z.exponent()
	if exp > 0 {
		exp = -exp
	}
	n := new(big.Int)
	n.SetUint64(5)
	n.Exp(n, new(big.Int).SetInt64(int64(exp)), nil)

	f = new(big.Float)
	f.SetInt(n)
	if z.exponent() < 0 {
	}
	f = f.Quo(new(big.Float).SetInt64(1), f)

	f.SetPrec(f.Prec() + uint(z.mant.BitLen()))
	f.Mul(f, new(big.Float).SetInt(&z.mant))
	return f
}

// Exponent returns the base 10 exponent for this decimal.
func (z *Decimal) exponent() int {
	if z == nil || slicesx.PointerEqual(z.mant.Bits(), z.expWords()) {
		return 0
	}
	return z.exp
}

func (z *Decimal) expWords() []big.Word {
	return unsafe.Slice((*big.Word)(unsafe.Pointer(&z.exp)), 1)
}

var bufs sync.Pool

// Format implements [fmt.Formatter].
func (z *Decimal) Format(state fmt.State, verb rune) {
	if z == nil {
		fmt.Fprintf(state, "<nil>")
		return
	}

	if z.mant.Bits() == nil {
		format := fmt.FormatString(state, verb)
		fmt.Fprintf(state, format, z.Float())
		return
	}

	prec, ok := state.Precision()
	if !ok {
		prec = -1
	}

	var scientific bool
	switch verb {
	case 'f', 'F':
		break
	case 'v', 'g', 'G':
		if !ok {
			prec = 6
		}
		scientific = z.exp < -4 || int(z.exp) > prec
	case 'e', 'E':
		if !ok {
			prec = 6
		}
		scientific = true
	case 'x', 'X':
		fmt.Fprintf(state, fmt.FormatString(state, verb), z.Float())
	case 'b':
		fmt.Fprintf(state, "%de%+d", z.mant, z.exp)
		return
	default:
		fmt.Fprintf(state, "%%%c<%T=%v>", verb, z, z)
		return
	}

	buf := bufs.Get().(*bytesx.Writer)
	if buf == nil {
		buf = new(bytesx.Writer)
	}
	defer func() {
		buf.Reset()
		bufs.Put(buf)
	}()

	switch {
	case z.mant.Sign() < 0:
		buf.WriteByte('-')
	case state.Flag('+'):
		buf.WriteByte('+')
	}

	e := 'e'
	upper := unicode.IsUpper(verb)
	if upper {
		e = unicode.ToUpper(e)
	}

	// Write out all the digits we have to work with in this base.
	start := len(*buf)
	fmt.Fprintf(buf, "%d", z.mant)
	digits := len(*buf) - start
	exp := z.exponent()

	// Add missing digits and a decimal point. This depends on the number of
	// digits before the decimal point. In scientific mode, this is 1.
	// Otherwise, it's the number of digits plus the exponent.
	point := 1
	if !scientific {
		point = digits + exp
	}

	// If point is negative, we need to insert that many leading zeros, plus
	// a zero before the decimal point.
	if point < 0 {
		*buf = slices.Insert(*buf, start, bytes.Repeat([]byte{'0'}, 1-point)...)
		point = 1
		digits = len(*buf) - start

	}

	// Now we need to discard extra digits, or add extra digits if necessary.
	if prec+1 < digits {
		// TODO: rounding!
		*buf = (*buf)[:start+prec+1]
	}
	// If there aren't enough digits, append some.
	for range prec + 1 - digits {
		buf.WriteByte('0')
	}

	if scientific {
		// Discard extra digits.

		// If prec == 0, we don't include a decimal point.
		if prec > 0 {
			// Stick a dot right after the first byte of digits.
			*buf = slices.Insert(*buf, start+1, '.')
		}

		// Print the exponent: we're done!
		fmt.Fprintf(buf, "%c%+03d", e, z.exponent())
		return
	}

	// Convert binary digits into mBase digits. Note that digits in base b
	// is log_b(n). Note that, per the log base change formula:
	//
	// log_2(n) = log_b(n)/log_b(2)
	// log_b(2) = log_2(2)/log_2(b) = 1/log_2(b)
	//
	// Thus, we want to multiply by 1/log_2(b), which we precompute in
	// a table.
	digits := float64(len(z.mant.Bits())*bits.UintSize) * log2Table[mBase-1]
}
