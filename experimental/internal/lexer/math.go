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

package lexer

import (
	"math"
	"math/big"
	"math/bits"

	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

var log2Table = func() (logs [16]float64) {
	for i := range logs {
		logs[i] = math.Log2(float64(i + 1))
	}
	return logs
}()

type parseIntResult struct {
	small        uint64
	big          *big.Float
	hasThousands bool
}

// parseInt parses an integer into a uint64 or, on overflow, into a big.Int.
//
// This function ignores any thousands separator underscores in digits.
func parseInt(digits string, base byte) (result parseIntResult, ok bool) {
	var bigBase, bigDigit *big.Float
	for _, r := range digits {
		if r == '_' {
			result.hasThousands = true
			continue
		}

		digit, ok := unicodex.Digit(r, base)
		if !ok {
			return result, false
		}

		if result.big == nil {
			// Perform arithmetic while checking for overflow.
			extra, shift := bits.Mul64(result.small, uint64(base))
			sum, carry := bits.Add64(shift, uint64(digit), 0)
			if extra == 0 && carry == 0 {
				result.small = sum
				continue
			}

			// We overflowed, so we need to spill into a big.Float.
			result.big = new(big.Float)
			result.big.SetUint64(result.small)

			bigBase = new(big.Float).SetUint64(uint64(base)) // Memoize converting the base.
			bigDigit = new(big.Float)
		}

		result.big.Mul(result.big, bigBase)
		result.big.Add(result.big, bigDigit.SetUint64(uint64(digit)))
	}

	return result, true
}
