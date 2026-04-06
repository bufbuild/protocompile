// Package bigx provides direct access to arithmetic primitives from math/big.
package bigx

import (
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

var scratch sync.Pool

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
	return bz.Add(bx, by).Bits()
}

// MulScalar computes z = x * y.
func MulScalar(z, x []big.Word, y big.Word) []big.Word {
	bz := new(big.Int).SetBits(z)
	bx := new(big.Int).SetBits(x)
	by := new(big.Int).SetBits([]big.Word{y})
	return bz.Add(bx, by).Bits()
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

// Format writes bits to the given writer with the given requested format.
func Format(w io.Writer, format string, z []big.Word) (int, error) {
	bz := new(big.Int).SetBits(z)

	// Passing pointers to fmt causes them to escape, but this is rarely
	// necessary. It certainly isn't in this case.
	bz = unsafex.NoEscape(bz)

	return fmt.Fprintf(w, format, bz)
}
