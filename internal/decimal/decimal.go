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
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

const (
	mantBits64        = 52 // Mantissa bits in a binary64.
	mantMask64 uint64 = 1<<mantBits64 - 1
	maxMant64  uint64 = 1<<(mantBits64+1) + 1 // Largest possible exact binary64 integer value.
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

// flags is various flags set on a [Decimal].
type flags uint32

const (
	sign  flags = 0b01 // Sign bit: is this value negative?
	base2 flags = 0b10 // Is the exponent in base 2?

	inf       flags = 0b01_00 // Is this value an infinity?
	nan       flags = 0b10_00 // Is this value a NaN? If so, the payload lives in get()[0].
	nonfinite flags = 0b11_00 // Mask for checking whether a value is nonfinite.
)

// Decimal is an arbitrary-precision numeric type capable of representing any
// rational number with a power of 10 denominator, up to memory limits.
//
// Decimal also has special sentinel values for signed infinity, and
// distinguishes 0 and -0, much like IEEE 754 floats.
type Decimal struct {
	_ unsafex.NoCopy

	// Mantissa digits are of the form d.dddd in whatever base this value uses
	// (either 10 or 2 depending on base2).
	//
	// TODO: The mantissa itself is currently encoded in base 2, but it may be
	// wise to consider a binary coded decimal representation for arithmetic.
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
	flags flags
}

// IsZero returns whether this value is 0.0 or -0.0.
func (z *Decimal) IsZero() bool {
	return len(z.get()) == 0
}

// IsFinite returns whether this value is finite.
func (z *Decimal) IsFinite() bool {
	return z.flags&nonfinite == 0
}

// IsInf returns whether this value is an infinity.
func (z *Decimal) IsInf() bool {
	return z.flags&inf != 0
}

// IsNaN returns whether this value is a NaN.
func (z *Decimal) IsNaN() bool {
	return z.flags&nan != 0
}

// NaN returns the NaN payload within this value, or -1 if it is
// not a NaN.
func (z *Decimal) NaN() int64 {
	if !z.IsNaN() {
		return -1
	}
	return int64(bigx.Uint64(z.get()) & mantMask64)
}

// Negative returns whether this value's sign bit is set.
func (z *Decimal) Negative() bool {
	return z.flags&sign != 0
}

// SetNegative sets whether this value's sign bit is set.
func (z *Decimal) SetNegative(neg bool) *Decimal {
	if neg {
		z.flags |= sign
	} else {
		z.flags &^= sign
	}
	return z
}

// Clear resets this decimal value to zero.
func (z *Decimal) Clear() {
	z.exp = 0
	z.flags = 0

	if z.raw.data == nil {
		z.raw.data = &z.raw.small[0]
	}

	z.raw.small[0] = 0
	z.raw.small[1] = 0
}

// SetInf sets z to +Infinity or -Infinity.
func (z *Decimal) SetInf(neg bool) *Decimal {
	z.Clear()
	z.flags |= inf
	if neg {
		z.flags |= sign
	}
	return z
}

// SetNaN sets this value to a quiet NaN with zero payload.
func (z *Decimal) SetNaN(neg bool) *Decimal {
	return z.SetNaNPayload(neg, 0)
}

// SetNaNPayload sets this value to a NaN with arbitrary payload.
func (z *Decimal) SetNaNPayload(neg bool, payload uint64) *Decimal {
	z.Clear()
	z.flags |= nan
	if neg {
		z.flags |= sign
	}
	z.set(bigx.SetUint64(z.get(), payload&mantBits64))
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
	if z.base2() {
		w = bigx.Scale2(w, z.get(), uint(n))
	} else {
		w = bigx.Scale10(w, z.get(), uint(n))
	}

	return x.SetBits(w)
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
	z.Clear()

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
//
// z.exp - z.digits() gives the value e such that z is n * b^e, where n is an
// integer and b is the base.
func (z *Decimal) digits() int {
	if z.base2() {
		return bigx.Log2(z.get()) + 1
	}
	return bigx.Log10(z.get()) + 1
}

func (z *Decimal) base2() bool  { return z.flags&base2 != 0 }
func (z *Decimal) base10() bool { return z.flags&base2 == 0 }
