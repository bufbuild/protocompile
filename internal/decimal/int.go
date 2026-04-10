package decimal

import (
	"math"
	"math/big"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
)

// IsInt returns whether this value is an integer.
func (z *Decimal) IsInt() bool {
	return z.IsZero() || int(z.exp) >= z.digits()
}

// Int sets x to the nearest integer to z.
//
// If z is non-finite, returns nil and leaves x unchanged.
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
