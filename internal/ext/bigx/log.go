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

package bigx

import (
	"math"
	"math/big"
)

var (
	tens = func() [64]big.Int {
		var table [64]big.Int
		for i := range table {
			table[i].Exp(new(big.Int).SetInt64(10), new(big.Int).SetInt64(int64(i)), nil)
		}
		return table
	}()
)

// Scale2 returns z = x * 2^y.
func Scale2(z, x []big.Word, y uint) []big.Word {
	return Shl(z, x, y)
}

// Scale2Fp returns z = x * 2^y.
func Scale2Fp(z *big.Float, x *big.Float, n int) *big.Float {
	if z == nil {
		z = new(big.Float)
	}
	if n == 0 {
		return z.Copy(x)
	}

	// Unfortunately this seems to be the least painful way to implement
	// scalbn, despite the fact it aught to be a primitive...
	pow2 := big.NewFloat(1)
	pow2.SetMantExp(pow2, n)
	return z.Mul(x, pow2)
}

// Scale10 returns z = x * 10^y.
func Scale10(z, x []big.Word, y uint) []big.Word {
	zb := new(big.Int).SetBits(z)
	xb := new(big.Int).SetBits(x)

	var t *big.Int
	if int(y) < len(tens) {
		t = &tens[y]
	} else {
		t, _ = scratch.Get().(*big.Int)
		if t == nil {
			t = new(big.Int)
		}
		defer func() {
			t.SetInt64(0)
			scratch.Put(t)
		}()
		t.Exp(
			new(big.Int).SetInt64(10),
			new(big.Int).SetInt64(int64(y)),
			nil,
		)
	}

	return zb.Mul(xb, t).Bits()
}

// Log2 returns floor(log2(z)).
//
// Runtime is O(1).
func Log2(z []big.Word) int {
	// Need to subtract 1 because bitlen(0b111) is 3, but floor(log2(7)) is 2.
	return new(big.Int).SetBits(z).BitLen() - 1
}

// Log10 returns floor(log10(z)).
//
// Runtime is O(log N * log N).
func Log10(z []big.Word) int {
	bz := new(big.Int).SetBits(z)

	// Algorithm from Reddit 💀
	// See: https://www.reddit.com/r/algorithms/comments/gybkk9/comment/ftet967/
	//
	// 1. Compute n = floor(log_2(N)). This can be done in constant time with
	//    any bignum representation simply by multiplying the bit-width of each
	//    word by the number of words in the representation of N and adding the
	//    floor of the base-2 logarithm of the most significant word. This part
	//    is essentially instantaneous.
	//
	// 2. Compute m = floor(n * ln(2)/ln(10)). This gives in constant time a
	//    lower bound on the base-10 logarithm of N. This is also nearly
	//    instantaneous.
	//
	// 3. Compute 10^(1+m) in full bignum precision, using the method of
	//    exponentiation by squaring. This requires O(log log N) steps, each
	//    step requiring at most O(log N) substeps. This is the slowest part.
	//
	// 4. Compare N with 10^(1+m). This requires O(log N) steps.
	//
	// 5.  If N >= 10^(1+m), then the number of digits is 2+m; otherwise the
	//     number of digits is 1+m.

	n := Log2(z)
	m := int(float64(n) * (math.Ln2 / math.Ln10)) // 77/256 is approximately log_10(2).

	var t *big.Int
	if m+1 < len(tens) {
		t = &tens[m+1]
	} else {
		t, _ = scratch.Get().(*big.Int)
		if t == nil {
			t = new(big.Int)
		}
		defer func() {
			t.SetInt64(0)
			scratch.Put(t)
		}()
		t.Exp(
			new(big.Int).SetInt64(10),
			new(big.Int).SetInt64(int64(m+1)),
			nil,
		)
	}

	if bz.Cmp(t) >= 0 {
		m++
	}
	return m
}
