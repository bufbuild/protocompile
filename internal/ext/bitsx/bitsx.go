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

// Package bitsx contains extensions to Go's package math/bits.
package bitsx

import (
	"math"
	"math/bits"
)

// IsPowerOfTwo returns whether n is a power of 2.
func IsPowerOfTwo(n uint) bool {
	// See https://github.com/mcy/best/blob/2d94f6b23aecddc46f792edb4c45800aa58074ca/best/math/bit.h#L147
	return bits.OnesCount(n) == 1
}

// NextPowerOfTwo returns the next power of 2 after n, or zero if n is greater
// than the largest power of 2.
func NextPowerOfTwo(n uint) uint {
	// For n == 0, LeadingZeros returns 64, and Go does not mask the shift
	// amount, so -1 >> 64 is zero, which produces the correct answer.
	//
	// If LeadingZeros produces 0 (i.e., the highest bit is set, so it's
	// larger than the largest power of 2) this addition will overflow back to
	// 0 as desired.
	return uint(math.MaxUint)>>uint(bits.LeadingZeros(n)) + 1
}

// MakePowerOfTwo snaps n to a power of 2: i.e., if it isn't already one,
// replaces it with the next power of two.
func MakePowerOfTwo(n uint) uint {
	if IsPowerOfTwo(n) {
		return n
	}
	return NextPowerOfTwo(n)
}
