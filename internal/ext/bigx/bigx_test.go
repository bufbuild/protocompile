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

package bigx_test

import (
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
)

func TestSetString(t *testing.T) {
	t.Parallel()

	assert.Empty(t, bigx.SetString(nil, "", 10))

	r := rand.New(rand.NewPCG(1, 2))
	for _, base := range []int{2, 8, 10, 16} {
		// Lengths around multiples of the size at which SetString splits its
		// input.
		for _, n := range []int{1, 255, 256, 257, 511, 512, 513, 1000, 5000} {
			digits := make([]byte, n)
			for i := range digits {
				digits[i] = "0123456789abcdef"[r.IntN(base)]
			}

			want, ok := new(big.Int).SetString(string(digits), base)
			if !ok {
				t.Fatalf("invalid test input in base %d: %s", base, digits)
			}
			got := new(big.Int).SetBits(bigx.SetString(nil, string(digits), base))
			assert.Zero(t, want.Cmp(got), "base %d, %d digits", base, n)
		}
	}
}
