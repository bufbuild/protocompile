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

// Package bigx provides direct access to arithmetic primitives from math/big.
package bigx

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"math/bits"
	"sync"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
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

// Shl computes z = x >> y.
func Shr(z, x []big.Word, y uint) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	return bz.Rsh(bx, y).Bits()
}

// Cmp unsigned-compares x and y.
func Cmp(x, y []big.Word) int {
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits(y)
	return bx.Cmp(by)
}

// Uint64 sets z = x.
func Uint64(z []big.Word, x uint64) []big.Word {
	if x > MaxWords {
		// Only possible when sizeof(Word) == 4.
		return append(z, big.Word(x), big.Word(x>>32))
	}
	return append(z, big.Word(x))
}

// Format writes bits to the given writer with the given requested format.
func Format(w io.Writer, format string, z []big.Word) (int, error) {
	bz := new(big.Int).SetBits(z)

	// Passing pointers to fmt causes them to escape, but this is rarely
	// necessary. It certainly isn't in this case.
	bz = unsafex.NoEscape(bz)

	return fmt.Fprintf(w, format, bz)
}
