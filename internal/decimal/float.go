// Copyright 2020-2026 Buf Technologies, Inc.
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
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"strconv"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/bytesx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// IsFloat returns whether this value is representable as a base 2 float.
func (z *Decimal) IsFloat() bool {
	if z.base2() || len(z.get()) == 0 {
		return true
	}

	// Calculate the exponent for an integer mantissa.
	exp := int(z.exp) - z.digits()
	if exp >= 0 {
		// Positive exponents are always representable; just multiply by an
		// appropriate power of 5.
		return true
	}

	// Need to check that the mantissa is divisible by an appropriate power of
	// 5. When converting to a float, we are looking for a number k such that
	// m * 10^e = k * 2^e. Then we must have that k = 5^e * m. If e is negative,
	// m must contain that factor of 5 within it so that dividing by it produces
	// no remainder.
	//
	// For example, consider 18.1875. This is a valid float, because it's equal
	// to 291 / 16. As a decimal, its representation is 0.181875e+07, or
	// 181875e-4. As a float, the equivalent form is 291p-4, obtained by
	// dividing by 5^4 = 625.

	// 5^-exp is larger than 4^-exp = 2^(-2*exp), so a mantissa with at most
	// -2*exp bits cannot be divisible by it. Check this before computing
	// 5^-exp, whose size grows with the exponent rather than with the mantissa.
	if bigx.Log2(z.get())/2 < -exp {
		return false
	}

	// Calculate the factor of 5 that must be in the mantissa.
	pow5, ok := slicesx.Get(fives[:], -exp)
	if !ok {
		pow5 = new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(-exp)), nil).Bits()
	}

	// Check to see if we have a chance of successful division. For nonzero
	// a < b, b cannot divide a.
	if bigx.Cmp(z.get(), pow5) < 0 {
		return false
	}

	// Perform the very slow division and hope that everything cancels out.
	rem := bigx.Rem(nil, z.get(), pow5)
	return len(rem) == 0
}

// Float64 calculates the closest 64-bit floating-point value to this decimal.
//
// Returns whether or not the resulting value is exact.
func (z *Decimal) Float64() (v float64, exact bool) {
	// Algorithm based on github.com/ericlagergren/decimal.
	// License: https://github.com/ericlagergren/decimal/blob/master/LICENSE

	if z.IsInf() {
		return math.Inf(-int(z.flags & sign)), true
	}
	if z.IsNaN() {
		return nanPayload(z.Negative(), uint64(z.NaN())), true
	}

	// The trick here is that for difficult cases, it is actually quite
	// efficient to go through strconv.ParseFloat, which implements a pretty
	// good decimal-to-float algorithm. In cases where the values involved
	// are small enough (common case), we can do the arithmetic ourselves.

	w := z.get()
	switch {
	case len(w) == 0:
		exact = true

	case bigx.IsUint64(w):
		w := bigx.Uint64(w)
		v = float64(w)
		exact = w <= maxMant64

		exp := int(z.exp) - z.digits()
		if exp == 0 {
			// No exponent, so we're done.
			break
		}

		// Multiplying by 5^exp only rounds once if 5^exp is itself exact.
		if exact && (!z.base10() || -maxPow5 <= exp && exp <= maxPow5) {
			// Need to divide out by the large power of five hiding inside this
			// value.
			if z.base10() {
				v = pow5(v, exp)
				exact = pow5Exact(w, exp)
			}

			// Directly update the exponent to add the missing power of 5.
			v = math.Ldexp(v, exp)
			if z.base2() {
				// Ldexp rounds if the result is subnormal.
				exact = math.Ldexp(v, -exp) == float64(w)
			}

			break
		}

		// We may lose precision by going through the above in some cases. For
		// example, when the target value is very close to the dynamic range
		// limits, converting the mantissa to float may lose precision before
		// we even multiply by 5^e.
		exact = false
		fallthrough
	default:
		if z.base2() {
			// Rounding to a float64 only needs the top 64 bits plus whether
			// any bit below them is set. When one is, fold it into bit 0,
			// which is always below the rounding position.
			top, shift, sticky := msb64(z.get())
			if sticky {
				top |= 1
			}

			// big.Float rounds to nearest even, including into the subnormal
			// range, so the result is correctly rounded.
			var x big.Float
			x.SetUint64(top)
			x.SetMantExp(&x, int(z.exp)-z.digits()+shift)

			var acc big.Accuracy
			v, acc = x.Float64()
			exact = !sticky && acc == big.Exact
			break
		}

		// Slowest case. We convert to a string of the form ddddennn and
		// push that into atof.
		buf, _ := bufs.Get().(*bytesx.Writer)
		if buf == nil {
			buf = new(bytesx.Writer)
		}
		defer func() {
			buf.Reset()
			bufs.Put(buf)
		}()

		*buf = bigx.Format(*buf, z.get(), 10)
		_, _ = fmt.Fprintf(buf, "e%d", int(z.exp)-z.digits())

		var err error
		v, err = strconv.ParseFloat(unsafex.StringAlias(*buf), 64)
		if errors.Is(err, strconv.ErrSyntax) {
			panic(fmt.Errorf("Decimal.Float64: unexpected syntax error: %w", err))
		}
	}

	if z.Negative() {
		v = -v
	}
	return v, exact && finite(v)
}

// SetFloat64 sets this decimal's value to x.
func (z *Decimal) SetFloat64(x float64) *Decimal {
	z.SetZero()

	if math.Signbit(x) {
		z.flags |= sign
	}
	if x == 0 {
		return z
	}
	if math.IsInf(x, 0) {
		z.flags |= inf
		return z
	}

	_, exp := math.Frexp(x)
	w := 1<<mantBits64 | math.Float64bits(x)&mantMask64
	w >>= bits.TrailingZeros64(w)

	z.set(bigx.SetUint64(z.get(), w))
	z.exp = int32(exp)
	z.flags |= base2

	return z
}

// float calculates the closest floating-point value to this decimal.
// This is not public API; it exists only to simplify float formatting, and
// will hopefully be removed eventually.
//
// Does not handle non-finite or negative values.
func (z *Decimal) float(x *big.Float) *big.Float {
	if x == nil {
		x = new(big.Float)
	}

	if z.flags&nonfinite != 0 {
		// big.Float does not handle NaNs, so we convert to an infinity.
		return x.SetInf(z.Negative())
	}

	digits := int(z.exp) - z.digits()
	x.SetPrec(0)
	x.SetInt(new(big.Int).SetBits(z.get()))
	x.SetPrec(uint(max(z.exp, -z.exp, int32(x.Prec())))) // Squeeze as much precision as possible out.
	x.SetMantExp(x, digits)

	// Essentially, we need to multiply z.mant by 5^z.exp to correct the
	// exponent. Currently, f is set to mant * 2^exp, and to have its value be
	// mant * 10^exp, it has to be multiplied by 5^z.exp.
	if z.base10() && digits != 0 {
		abs := max(digits, -digits)

		bits, ok := slicesx.Get(fives[:], abs)
		pow5 := new(big.Int).SetBits(bits)
		if !ok {
			pow5 = new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(abs)), nil)
		}

		scale := new(big.Float)
		scale.SetPrec(scale.Prec() + uint(pow5.BitLen()) + 2)
		scale.SetInt(pow5)

		x.SetPrec(max(x.Prec(), scale.Prec(), uint(z.exp)))
		if digits > 0 {
			x.Mul(x, scale)
		} else {
			x.Quo(x, scale)
		}
	}

	return x
}

func nanPayload(sign bool, x uint64) float64 {
	nan := uint64(0x7FF8000000000000)
	nan |= x & mantMask64
	if sign {
		nan |= 1 << 63
	}
	return math.Float64frombits(nan)
}

func finite(x float64) bool {
	return !math.IsInf(x, 0) && !math.IsNaN(x)
}

// msb64 returns the 64 most significant bits of m and the number of bits
// below them. sticky reports whether any of those lower bits are set.
func msb64(m []big.Word) (top uint64, shift int, sticky bool) {
	const w = bits.UintSize
	if len(m) == 0 {
		return 0, 0, false
	}
	n := (len(m)-1)*w + bits.Len(uint(m[len(m)-1]))
	shift = max(0, n-64)

	for i := len(m) - 1; i >= 0; i-- {
		lo := i * w
		word := uint64(m[i])
		switch {
		case lo >= shift:
			top |= word << uint(lo-shift)
		case lo+w > shift:
			drop := uint(shift - lo)
			top |= word >> drop
			sticky = word&(1<<drop-1) != 0
		default:
			sticky = word != 0
		}
		if sticky {
			// Every bit of top is above this word, so we're done.
			break
		}
	}
	return top, shift, sticky
}

// pow5Exact returns whether w * 5^n is exactly representable as a float64,
// for -maxPow5 <= n <= maxPow5.
func pow5Exact(w uint64, n int) bool {
	if n < 0 {
		return w%uint64(pow5s[-n]) == 0
	}
	// 5^n is odd, so only the odd part of w can push the product past the
	// width of a float64 mantissa.
	hi, lo := bits.Mul64(w>>bits.TrailingZeros64(w), uint64(pow5s[n]))
	return hi == 0 && lo <= maxMant64
}

// pow5s holds 5^n for each n where 5^n is exactly representable as a float64.
var pow5s = [...]float64{
	1e00 / 0x1p00, 1e01 / 0x1p01, 1e02 / 0x1p02, 1e03 / 0x1p03,
	1e04 / 0x1p04, 1e05 / 0x1p05, 1e06 / 0x1p06, 1e07 / 0x1p07,
	1e08 / 0x1p08, 1e09 / 0x1p09, 1e10 / 0x1p10, 1e11 / 0x1p11,
	1e12 / 0x1p12, 1e13 / 0x1p13, 1e14 / 0x1p14, 1e15 / 0x1p15,
	1e16 / 0x1p16, 1e17 / 0x1p17, 1e18 / 0x1p18, 1e19 / 0x1p19,
	1e20 / 0x1p20, 1e21 / 0x1p21, 1e22 / 0x1p22,
}

// maxPow5 is the largest n for which 5^n is in pow5s.
const maxPow5 = len(pow5s) - 1

// pow5 multiplies f by 5^n, for -maxPow5 <= n <= maxPow5.
func pow5(f float64, n int) float64 {
	if n < 0 {
		return f / pow5s[-n]
	}
	return f * pow5s[n]
}
