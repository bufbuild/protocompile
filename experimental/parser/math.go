// Copyright 2020-2024 Buf Technologies, Inc.
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

package parser

import (
	"math/big"
	"math/bits"
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
	var bigBase *big.Int
	for _, r := range digits {
		if r == '_' {
			result.hasThousands = true
			continue
		}

		digit := parseDigit(r)
		if digit >= base {
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

			// We overflowed, so we need to spill into a big.Int.
			result.big = new(big.Int)
			result.big.SetUint64(result.small)

			bigBase = big.NewInt(int64(base)) // Memoize converting the base.
		}

		result.big.Mul(result.big, bigBase)
		result.big.Add(result.big, big.NewInt(int64(digit)))
	}

	return result, true
}

// parseDigit parses a digit up to hexadecimal; returns 0xff if d is not a valid
// digit rune. This allows checking for the base of the digit, or if it is
// a valid digit at all, in one comparison.
//
// E.g., parseDigit('7') < 10 checks for valid decimal digits.
func parseDigit(d rune) byte {
	switch {
	case d >= '0' && d <= '9':
		return byte(d) - '0'

	case d >= 'a' && d <= 'f':
		return byte(d) - 'a' + 10

	case d >= 'A' && d <= 'F':
		return byte(d) - 'A' + 10

	default:
		return 0xff
	}
}
