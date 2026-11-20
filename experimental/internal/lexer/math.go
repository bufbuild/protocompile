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

package lexer

import (
	"math/big"
	"math/bits"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

type parseIntResult struct {
	small        uint64
	big          *big.Int
	hasThousands bool
}

// parseInt parses an integer into a uint64 or, on overflow, into a big.Int.
//
// This function ignores any thousands separator underscores in digits.
func parseInt(digits string, base byte) (result parseIntResult, ok bool) {
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

			// We overflowed, so we need to spill into a big.Int. It is filled
			// in below once every digit has been checked, because multiplying
			// the digits in one at a time is quadratic in their number.
			result.big = new(big.Int)
		}
	}

	if result.big != nil {
		digits = strings.ReplaceAll(digits, "_", "")
		result.big.SetBits(bigx.SetString(nil, digits, int(base)))
	}
	return result, true
}
