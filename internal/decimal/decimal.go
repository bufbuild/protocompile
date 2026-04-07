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

// Package decimal provides a big decimal type, i.e., a version of [big.Float]
// which works on a base 10 exponent, rather than base 2, allowing it to
// represent values such as 0.1 exactly.
package decimal

import (
	"math"
	"math/big"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

var (
	fives = func() [64][]big.Word {
		five := new(big.Int).SetUint64(5)
		acc := new(big.Int).SetUint64(1)

		var table [64][]big.Word
		for i := range table {
			table[i] = acc.Bits()

			acc = new(big.Int).Mul(acc, five)
		}

		return table
	}()
)

// Decimal is an arbitrary-precision numeric type capable of representing values.
type Decimal struct {
	_ unsafex.NoCopy

	// Mantissa digits are of the form d.dddd in whatever base this value uses
	// (either 10 or 2 depending on base2).
	//
	// Note that we *do not* use big.Int because it does not believe in -0.0.
	raw struct {
		// data may point to small for the purposes of holding particularly
		// small values. This case is detected by data == &small. The length
		// is stored implicitly, by removing leading zeros.
		data  *big.Word
		small [2]big.Word // [len, cap] when not small.
	}

	exp   int32
	neg   bool
	base2 bool
	inf   bool

	float atomic.Pointer[big.Float]
}

// IsZero returns whether this value is 0.0 or -0.0.
func (z *Decimal) IsZero() bool {
	return len(z.get()) == 0
}

// Clear resets this decimal value to zero.
func (z *Decimal) Clear() {
	z.lockFloat(false)
	defer z.float.Store(nil)
	z.clear()
}

func (z *Decimal) clear() {
	z.exp = 0
	z.base2 = false
	z.neg = false
	z.inf = false

	if z.raw.data == nil {
		z.raw.data = &z.raw.small[0]
	}

	z.raw.small[0] = 0
	z.raw.small[1] = 0
}

// SetInf sets z to +Infinity or -Infinity.
func (z *Decimal) SetInf(neg bool) *Decimal {
	z.lockFloat(false)
	defer z.float.Store(nil)
	z.clear()

	z.inf = true
	z.neg = neg
	return z
}

// IsInt returns whether this value is an integer.
func (z *Decimal) IsInt() bool {
	return z.IsZero() || int(z.exp) >= z.digits()
}

// Int sets x to the nearest integer to z.
func (z *Decimal) Int(x *big.Int) *big.Int {
	if x == nil {
		x = new(big.Int)
	}

	n := int(z.exp) - z.digits()
	if n < 0 {
		return x.SetUint64(0)
	}

	w := x.Bits()
	if z.base2 {
		w = bigx.Scale2(w, z.get(), uint(n))
	} else {
		w = bigx.Scale10(w, z.get(), uint(n))
	}

	return x.SetBits(w)
}

// SetInt sets this decimal's value to x.
func (z *Decimal) SetInt(x *big.Int) *Decimal {
	return z.setInt(x, false)
}

// SetInt sets this decimal's value to x.
func (z *Decimal) SetUint64(x uint64) *Decimal {
	// Doing it this way gives us a good shot to get this slice to allocate
	// on the stack.
	xb := new(big.Int).SetBits(bigx.Uint64(make([]big.Word, 0, 2), x))
	return z.setInt(xb, false)
}

// ReuseInt sets this decimal's value to x, consuming x's storage in the
// process.
func (z *Decimal) ReuseInt(x *big.Int) *Decimal {
	return z.setInt(x, true)
}

func (z *Decimal) setInt(x *big.Int, reuse bool) *Decimal {
	z.lockFloat(false)
	defer z.float.Store(nil)
	z.clear()

	z.neg = x.Sign() < 0

	exp := x.BitLen()
	if exp > math.MaxInt32 || exp < math.MinInt32 {
		z.inf = true
		return z
	}

	// Because this is an integer, we can use a power of 2 exponent.
	// This simplifies the task of calculating an exponent, punting the
	// "convert to base 10" problem to later, if necessary at all.
	z.base2 = true

	w := x.Bits()
	if !reuse || cap(w) < cap(z.get()) {
		w = append(z.get()[:0], w...)
	}

	// Knock off any trailing zeros. Because of the representation we've chosen,
	// trailing zeros are never part of the final value.
	//
	// Because we're putting this in 0.bbbbb * 2^e form, if there are trailing
	// zeros before the binary point, they are automatically filled by the
	// << implied by the 2^e.
	z.set(bigx.Shr(w, w, x.TrailingZeroBits()))
	z.exp = int32(exp)

	return z
}

// IsInt returns whether this value is representable as a base 2 float.
func (z *Decimal) IsFloat() bool {
	if z.base2 || len(z.get()) == 0 {
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
	//
	// Now consider a very small exponent resulting in leading zero, say
	// 0.0009765625 equal to 9765625e-10, or 1p-10. We need to divide by
	// 5^10=1.024e+13. But this division can't work, because the exponent is
	// huge.

	// Calculate the factor of 5 that must be in the mantissa.
	var pow5 []big.Word
	if -exp < len(fives) {
		pow5 = fives[-exp]
	} else {
		five := new(big.Int).SetUint64(5)
		pow5 = five.Exp(five, new(big.Int).SetUint64(uint64(-exp)), nil).Bits()
	}

	if bigx.Cmp(z.get(), pow5) < 0 {
		return false
	}

	// Perform the very slow division and hope that everything cancels out.
	rem := bigx.Rem(nil, z.get(), pow5)
	return len(rem) == 0
}

// Float calculates the closest floating-point value to this decimal.
func (z *Decimal) Float() *big.Float {
	f := z.lockFloat(true)
	if f != nil {
		return f
	}

	f = new(big.Float)
	defer z.float.Store(f)

	if z.inf {
		f.SetInf(z.neg)
		return f
	}

	digits := int(z.exp) - z.digits()
	f.SetInt(new(big.Int).SetBits(z.get()))
	f.SetMantExp(f, digits)
	if z.neg {
		f.Mul(f, new(big.Float).SetInt64(-1))
	}

	// Essentially, we need to multiply z.mant by 5^z.exp to correct the
	// exponent. Currently, f is set to mant * 2^exp, and to have its value be
	// mant * 10^exp, it has to be multiplied by 5^z.exp.
	if !z.base2 && digits != 0 {
		abs := digits
		if abs < 0 {
			abs = -abs
		}

		var pow5 *big.Int
		if abs < len(fives) {
			pow5 = new(big.Int).SetBits(fives[abs])
		} else {
			pow5 = new(big.Int).SetUint64(5)
			pow5 = pow5.Exp(pow5, new(big.Int).SetUint64(uint64(abs)), nil)
		}

		scale := new(big.Float)
		scale.SetPrec(scale.Prec() + uint(pow5.BitLen()) + 1)
		scale.SetInt(pow5)
		if digits > 0 {
			f.Mul(f, scale)
		} else {
			f.Quo(f, scale)
		}
	}

	return f
}

// SetInt sets this decimal's value to x.
func (z *Decimal) SetFloat64(x float64) *Decimal {
	f := z.lockFloat(false)
	if f != nil {
		f = big.NewFloat(x)
	}
	defer z.float.Store(f)
	z.clear()

	z.neg = math.Signbit(x)
	if x == 0 {
		return z
	}
	if math.IsInf(x, 0) {
		z.inf = true
		return z
	}

	mant, exp := math.Frexp(x)
	z.set(bigx.Uint64(z.get(), 1<<63|math.Float64bits(mant)<<11))
	z.exp = int32(exp)
	z.base2 = true

	return z
}

func (z *Decimal) get() []big.Word {
	if z.raw.data == &z.raw.small[0] {
		s := unsafe.Slice(z.raw.data, 2)
		if s[1] == 0 {
			if s[0] == 0 {
				s = s[:0]
			} else {
				s = s[:1]
			}
		}
		return s
	}
	return unsafe.Slice(z.raw.data, z.raw.small[1])[:z.raw.small[0]]
}

func (z *Decimal) set(mant []big.Word) {
	data := unsafe.SliceData(mant)
	if data == &z.raw.small[0] {
		z.raw.data = &z.raw.small[0]
		switch len(mant) {
		case 0:
			z.raw.small[0] = 0
			fallthrough
		case 1:
			z.raw.small[1] = 0
		}
		return
	}
	if data == &z.raw.small[1] {
		z.raw.small[0] = z.raw.small[1]
		z.raw.small[1] = 0
		data = &z.raw.small[0]
	}
	z.raw.data = data
	z.raw.small[0] = big.Word(len(mant))
	z.raw.small[1] = big.Word(cap(mant))
}

// digits returns the number of digits in the mantissa in the given base.
func (z *Decimal) digits() int {
	if z.base2 {
		return bigx.Log2(z.get()) + 1
	}
	return bigx.Log10(z.get()) + 1
}

var locked big.Float

// lockFloat locks z.float and returns the current value. To unlock, simply
// store to z.float.
//
// If lockIfNil is true, only locks if z.float is nil.
func (z *Decimal) lockFloat(lockIfNil bool) *big.Float {
again:
	f := z.float.Load()
	if f == &locked {
		runtime.Gosched()
		goto again
	}
	if lockIfNil && f != nil {
		return f
	}
	if !z.float.CompareAndSwap(f, &locked) {
		goto again
	}
	return f
}
