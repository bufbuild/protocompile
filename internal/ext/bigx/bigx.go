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

// Package bigx provides direct access to arithmetic primitives from math/big.
package bigx

import (
	"math"
	"math/big"
	"math/bits"
	"sync"
)

var scratch sync.Pool

const (
	WordBits = bits.UintSize
	MaxWords = math.MaxUint
)

// Add computes z = x + y.
func Add(z, x, y []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bz.Add(bx, by).Bits()
}

// AddScalar computes z = x + y.
func AddScalar(z, x []big.Word, y big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits([]big.Word{y})
	return bz.Add(bx, by).Bits()
}

// Sub computes z = x - y.
func Sub(z, x, y []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bz.Sub(bx, by).Bits()
}

// SubScalar computes z = x - y.
func SubScalar(z, x []big.Word, y big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits([]big.Word{y})
	return bz.Sub(bx, by).Bits()
}

// Mul computes z = x * y.
func Mul(z, x, y []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bz.Mul(bx, by).Bits()
}

// MulScalar computes z = x * y.
func MulScalar(z, x []big.Word, y big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits([]big.Word{y})
	return bz.Mul(bx, by).Bits()
}

// Div computes z = x / y (truncating division).
func Div(z, x, y []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bz.Quo(bx, by).Bits()
}

// Rem computes z = x % y (truncating division).
func Rem(z, x, y []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bz.Rem(bx, by).Bits()
}

// FMA computes z = x * y + w.
func FMA(z, x, y, w []big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	bw := new(big.Int).SetBits(w)
	return bz.Add(bz.Mul(bx, by), bw).Bits()
}

// FMAScalar computes z = x * y + w.
func FMAScalar(z, x []big.Word, y, w big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits([]big.Word{y})
	bw := new(big.Int).SetBits([]big.Word{w})
	return bz.Add(bz.Mul(bx, by), bw).Bits()
}

// Shl computes z = x << y.
func Shl(z, x []big.Word, y uint) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	return bz.Lsh(bx, y).Bits()
}

// Shr computes z = x >> y.
func Shr(z, x []big.Word, y uint) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	return bz.Rsh(bx, y).Bits()
}

// MSBs sets z to the n highest bits of x.
func MSBs(z, x []big.Word, n uint) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)

	shift := max(0, bx.BitLen()-int(n))
	return bz.Rsh(bx, uint(shift)).Bits()
}

// Cmp unsigned-compares x and y.
func Cmp(x, y []big.Word) int {
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bx.Cmp(by)
}

// Returns whether z can fit into a uint64.
func IsUint64(z []big.Word) bool {
	return len(z) <= 64/WordBits
}

// Uint64 returns the low 64 bits of z.
func Uint64(z []big.Word) uint64 {
	switch len(z) {
	case 0:
		return 0
	case 1:
		return uint64(z[0])
	default:
		if WordBits == 32 {
			return uint64(z[0]) | (uint64(z[1]) << 32)
		}
		return uint64(z[0])
	}
}

// SetUint64 sets z = x.
func SetUint64(z []big.Word, x uint64) []big.Word {
	if x > MaxWords {
		// Only possible when sizeof(Word) == 4.
		return append(z[:0], big.Word(x), big.Word(x>>32))
	}
	return append(z[:0], big.Word(x))
}

// TrailingZeros returns the number of trailing zeros in z.
func TrailingZeros(z []big.Word) int {
	return int(new(big.Int).SetBits(z).TrailingZeroBits())
}

// Format writes bits to the given writer with the given requested format.
func Format(buf []byte, z []big.Word, base int) []byte {
	return new(big.Int).SetBits(z).Append(buf, base)
}

// setStringLeaf is the length at or below which [SetString] hands its input
// to [big.Int.SetString].
const setStringLeaf = 256

// SetString sets z to the value of s, read as an unsigned integer in the given
// base. s must consist only of digits valid in base, with no sign, prefix, or
// separators. An empty s is zero.
//
// [big.Int.SetString] does one multiply-add per word of digits, which is
// quadratic in len(s) for bases that are not a power of two. Instead, this
// splits s in two, converts each half, and recombines them as
// hi*base^len(lo) + lo, so that the cost is that of a few large
// multiplications.
func SetString(z []big.Word, s string, base int) []big.Word {
	var pows []*big.Int // pows[k] is base^(setStringLeaf << k).

	var conv func(z *big.Int, s string) *big.Int
	conv = func(z *big.Int, s string) *big.Int {
		if len(s) <= setStringLeaf {
			if _, ok := z.SetString(s, base); !ok {
				z.SetUint64(0)
			}
			return z
		}

		// Split off the low setStringLeaf<<k digits, for the largest k that
		// leaves at least one high digit. The low part is then at least half
		// of s, and each power of the base is computed only once.
		k := bits.Len(uint((len(s)-1)/setStringLeaf)) - 1
		n := setStringLeaf << k
		for len(pows) <= k {
			if len(pows) == 0 {
				pows = append(pows, new(big.Int).Exp(big.NewInt(int64(base)), big.NewInt(setStringLeaf), nil))
				continue
			}
			pow := pows[len(pows)-1]
			pows = append(pows, new(big.Int).Mul(pow, pow))
		}

		lo := conv(new(big.Int), s[len(s)-n:])
		conv(z, s[:len(s)-n])
		z.Mul(z, pows[k])
		return z.Add(z, lo)
	}

	return conv(new(big.Int).SetBits(z), s).Bits()
}
