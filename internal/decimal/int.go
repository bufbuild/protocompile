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
	"math"
	"math/big"
	"math/bits"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
)

var pow10s = [...]uint64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}

// IsInt returns whether this value is an integer.
func (z *Decimal) IsInt() bool {
	return z.IsZero() || int(z.exp) >= z.digits()
}

// Int sets x to the nearest integer to z.
//
// If z is non-finite, returns nil and leaves x unchanged.
//
// The size of the result grows with z's exponent, so it may be far larger than
// the text z was parsed from. Use [Decimal.Uint64] when only values that fit
// in 64 bits are of interest.
func (z *Decimal) Int(x *big.Int) *big.Int {
	if !z.IsFinite() {
		return nil
	}

	if x == nil {
		x = new(big.Int)
	}

	n := int(z.exp) - z.digits()
	if n < 0 {
		return x.SetUint64(0)
	}

	w := x.Bits()
	if z.base2() {
		w = bigx.Scale2(w, z.get(), uint(n))
	} else {
		w = bigx.Scale10(w, z.get(), uint(n))
	}

	return x.SetBits(w)
}

// Uint64 returns z as a uint64.
//
// Returns false if z is not an integer in the range of a uint64; this
// includes non-finite and negative values. Unlike [Decimal.Int], this does not
// materialize values that cannot fit.
func (z *Decimal) Uint64() (uint64, bool) {
	if z.IsZero() {
		return 0, z.IsFinite()
	}
	if !z.IsFinite() || z.Negative() || !z.IsInt() {
		return 0, false
	}

	if z.base2() {
		// The mantissa is normalized to 0.1bbbb, so z is at least 2^(exp-1).
		if z.exp > 64 {
			return 0, false
		}
	} else if z.exp > 20 {
		// The mantissa is normalized to 0.dddd, so z is at least 10^(exp-1).
		return 0, false
	}

	m := z.get()
	if !bigx.IsUint64(m) {
		return 0, false
	}
	v := bigx.Uint64(m)
	n := int(z.exp) - z.digits()
	if z.base2() {
		return v << n, true
	}
	hi, lo := bits.Mul64(v, pow10s[n])
	return lo, hi == 0
}

// SetUint64 sets this decimal's value to x.
func (z *Decimal) SetUint64(x uint64) *Decimal {
	// Doing it this way gives us a good shot to get this slice to allocate
	// on the stack.
	xb := new(big.Int).SetBits(bigx.SetUint64(make([]big.Word, 0, 2), x))
	return z.setInt(xb, false)
}

// SetInt sets this decimal's value to x.
func (z *Decimal) SetInt(x *big.Int) *Decimal {
	return z.setInt(x, false)
}

// ReuseInt sets this decimal's value to x, consuming x's storage in the
// process.
func (z *Decimal) ReuseInt(x *big.Int) *Decimal {
	return z.setInt(x, true)
}

func (z *Decimal) setInt(x *big.Int, reuse bool) *Decimal {
	z.SetZero()

	if x.Sign() < 0 {
		z.flags |= sign
	}

	exp := x.BitLen()
	if exp > math.MaxInt32 || exp < math.MinInt32 {
		z.flags |= inf
		return z
	}

	// Because this is an integer, we can use a power of 2 exponent.
	// This simplifies the task of calculating an exponent, punting the
	// "convert to base 10" problem to later, if necessary at all.
	z.flags |= base2

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
