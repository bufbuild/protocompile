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

// Float calculates the closest 64-bit floating-point value to this decimal.
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
	switch len(w) {
	case 0:
		exact = true

	case 1:
		w := w[0]
		v = float64(w)
		exact = w < maxMant64

		// No exponent, so we're done.
		exp := int(z.exp) - z.digits()
		if exp == 0 {
			exact = exact || (w&(w-1)) == 0
			break
		}

		// Need to divide out by the large power of five hiding inside this
		// value.
		if z.base10() {
			v = pow5(v, exp)
		}

		// Directly update the exponent to add the missing power of 5.
		v = math.Ldexp(v, exp)

		// If pow10 is 0 or infinite, we are in a bit of a pathological
		// situation. The value may in fact be representable, but we don't have
		// enough precision to represent

	default:
		if z.base2() {
			// We can do direct conversion, but rounding will always occur.
			// We want to start with the high 54 bits of the mantissa, then
			// multiply by an appropriate power of 2. Starting with the high
			// 54, rather than the 53 that fit into a binary64 mantissa, ensures
			// we get the correct rounding when converting to float64.

			bits := mantBits64 + 1
			w := bigx.MSBs(make([]big.Word, 0, 1), z.get(), uint(bits))[0]
			v = float64(w)

			// If w is d.dddd * 2^e, w is d.dddd * 2^53. We need to multiply
			// by 2^(e-53).
			v = math.Ldexp(v, int(z.exp)-bits)
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
	z.Clear()

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
	nan |= x & mantBits64
	if sign {
		nan |= 1 << 63
	}
	return math.Float64frombits(nan)
}

func finite(x float64) bool {
	return !math.IsInf(x, 0) && !math.IsNaN(x)
}

var (
	pow5s = [...]float64{
		1e00 / 0x1p00, 1e01 / 0x1p01, 1e02 / 0x1p02, 1e03 / 0x1p03,
		1e04 / 0x1p04, 1e05 / 0x1p05, 1e06 / 0x1p06, 1e07 / 0x1p07,
		1e08 / 0x1p08, 1e09 / 0x1p09, 1e10 / 0x1p10, 1e11 / 0x1p11,
		1e12 / 0x1p12, 1e13 / 0x1p13, 1e14 / 0x1p14, 1e15 / 0x1p15,

		1e16 / 0x1p16, 1e17 / 0x1p17, 1e18 / 0x1p18, 1e19 / 0x1p19,
		1e20 / 0x1p20, 1e21 / 0x1p21, 1e22 / 0x1p22, 1e07 / 0x1p07,
		1e24 / 0x1p24, 1e25 / 0x1p25, 1e26 / 0x1p26, 1e27 / 0x1p27,
		1e28 / 0x1p28, 1e29 / 0x1p29, 1e30 / 0x1p30, 1e31 / 0x1p31,
	}

	pow5s32 = [...]float64{
		1e+00 / 0x1p+00, 1e+32 / 0x1p+32, 1e+64 / 0x1p+64, 1e+96 / 0x1p+96,
		1e+128 / 0x1p+128, 1e+160 / 0x1p+160, 1e+192 / 0x1p+192, 1e+224 / 0x1p+224,
		1e+256 / 0x1p+256, 1e+288 / 0x1p+288,
	}

	pow5s32neg = [...]float64{
		1e-00 / 0x1p-00, 1e-32 / 0x1p-32, 1e-64 / 0x1p-64, 1e-96 / 0x1p-96,
		1e-128 / 0x1p-128, 1e-160 / 0x1p-160, 1e-192 / 0x1p-192, 1e-224 / 0x1p-224,
		1e-256 / 0x1p-256, 1e-288 / 0x1p-288, 1e-320 / 0x1p-320,
	}
)

// pow5 multiplies f by 5^n.
//
// We need to perform the multiplication with f within this function because
// float arithmetic is non-associative. For multiplying by very large or very
// small powers, we can easily lose precision, but by performing the
// multiplication in two parts, we avoid this risk.
func pow5(f float64, n int) float64 {
	switch {
	case 0 <= n && n <= 309:
		return f * pow5s32[uint(n)/32] * pow5s[uint(n)%32]
	case -324 <= n && n <= 0:
		return f * pow5s32neg[uint(-n)/32] / pow5s[uint(-n)%32]
	case n > 0:
		return math.Inf(1)
	default:
		return 0
	}
}
