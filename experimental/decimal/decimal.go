// Package decimal provides a big decimal type, i.e., a version of [big.Float]
// which works on a base 10 exponent, rather than base 2, allowing it to
// represent values such as 0.1 exactly.
package decimal

import (
	"math/big"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
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

// Decimal is an arbitrary-precision numeric type capable of representing values
type Decimal struct {
	// Mantissa digits are of the form d.dddd in whatever base this value uses
	// (either 10 or 2 depending on binExp).
	//
	// Note that we *do not* use big.Int because it does not believe in -0.0.
	raw struct {
		// data may point to cap for the purposes of holding particularly
		// small values. This case is detected by data == &cap
		data     *big.Word
		len, cap big.Word
	}

	exp    int
	neg    bool
	binExp bool

	float atomic.Pointer[big.Float]
}

// IsZero returns whether this value is 0.0 or -0.0.
func (z *Decimal) IsZero() bool {
	for _, w := range z.get() {
		if w != 0 {
			return false
		}
	}
	return true
}

// Float calculates the closest floating-point value to this decimal.
func (z *Decimal) Float() *big.Float {
	f := z.lockFloat(true)
	if f != nil {
		return f
	}

	f = new(big.Float)
	defer z.float.Store(f)

	digits := z.exp - z.digits()
	f.SetInt(new(big.Int).SetBits(z.get()))
	f.SetMantExp(f, digits)
	if z.neg {
		f.Mul(f, new(big.Float).SetInt64(-1))
	}

	// Essentially, we need to multiply z.mant by 5^z.exp to correct the
	// exponent. Currently, f is set to mant * 2^exp, and to have its value be
	// mant * 10^exp, it has to be multiplied by 5^z.exp.
	if !z.binExp && digits != 0 {
		abs := digits
		if abs < 0 {
			abs = -abs
		}

		var powFive *big.Int
		if abs < len(fives) {
			powFive = new(big.Int).SetBits(fives[abs])
		} else {
			powFive = new(big.Int).SetUint64(5)
			powFive = powFive.Exp(powFive, new(big.Int).SetUint64(uint64(abs)), nil)
		}

		scale := new(big.Float)
		scale.SetPrec(scale.Prec() + uint(powFive.BitLen()) + 1)
		scale.SetInt(powFive)
		if digits > 0 {
			f.Mul(f, scale)
		} else {
			f.Quo(f, scale)
		}
	}

	return f
}

func (z *Decimal) get() []big.Word {
	if z.raw.data == &z.raw.cap {
		return unsafe.Slice(z.raw.data, 1)
	}
	return unsafe.Slice(z.raw.data, z.raw.cap)[:z.raw.len]
}

func (z *Decimal) set(mant []big.Word) {
	data := unsafe.SliceData(mant)
	if data == &z.raw.cap {
		z.raw.data = &z.raw.cap
		return
	}
	z.raw.data = data
	z.raw.len = big.Word(len(mant))
	z.raw.cap = big.Word(cap(mant))
}

// digits returns the number of digits in the mantissa in the given base.
func (z *Decimal) digits() int {
	if z.binExp {
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
