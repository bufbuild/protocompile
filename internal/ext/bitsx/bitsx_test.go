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

package bitsx_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/bitsx"
)

func TestAddOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		x, y, z int
		of      bool
	}{
		{x: 1, y: 2, z: 3},
		{x: 0, y: -5, z: -5},
		{x: -3, y: -5, z: -8},
		{x: -5, y: 3, z: -2},
		{x: 5, y: -3, z: 2},
		{x: math.MaxInt, y: -1, z: math.MaxInt - 1},
		{x: math.MinInt, y: 1, z: math.MinInt + 1},
		{x: math.MaxInt, y: math.MinInt, z: -1},
		{x: math.MinInt, y: math.MaxInt, z: -1},
		{x: math.MaxInt, y: 1, z: math.MinInt, of: true},
		{x: math.MinInt, y: -1, z: math.MaxInt, of: true},
		{x: -1, y: math.MinInt, z: math.MaxInt, of: true},
		{x: math.MinInt, y: math.MinInt, z: 0, of: true},
	}

	for _, tt := range tests {
		z, of := bitsx.AddOverflow(tt.x, tt.y)
		assert.Equal(t, tt.z, z, "%d + %d", tt.x, tt.y)
		assert.Equal(t, tt.of, of, "%d + %d", tt.x, tt.y)
	}
}

func TestMulOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		x, y, z int
		of      bool
	}{
		{x: 0, y: math.MinInt, z: 0},
		{x: math.MinInt, y: 0, z: 0},
		{x: -3, y: 5, z: -15},
		{x: -3, y: -5, z: 15},
		{x: 1, y: math.MinInt, z: math.MinInt},
		{x: -1, y: math.MaxInt, z: -math.MaxInt},
		{x: -1, y: math.MinInt, z: math.MinInt, of: true},
		{x: math.MinInt, y: -1, z: math.MinInt, of: true},
		{x: math.MaxInt, y: 2, z: -2, of: true},
	}

	for _, tt := range tests {
		z, of := bitsx.MulOverflow(tt.x, tt.y)
		assert.Equal(t, tt.z, z, "%d * %d", tt.x, tt.y)
		assert.Equal(t, tt.of, of, "%d * %d", tt.x, tt.y)
	}
}

func TestSaturate(t *testing.T) {
	t.Parallel()

	// Check every pair of edge values against math/big.
	edges := []int{
		0, 1, 2, 3, -1, -2, -3,
		math.MaxInt, math.MaxInt - 1, math.MaxInt / 2, math.MaxInt/2 + 1,
		math.MinInt, math.MinInt + 1, math.MinInt / 2, math.MinInt/2 - 1,
	}
	minInt, maxInt := big.NewInt(math.MinInt), big.NewInt(math.MaxInt)
	overflows := func(v *big.Int) bool {
		return v.Cmp(minInt) < 0 || v.Cmp(maxInt) > 0
	}
	clamp := func(v *big.Int) int {
		switch {
		case v.Cmp(maxInt) > 0:
			return math.MaxInt
		case v.Cmp(minInt) < 0:
			return math.MinInt
		default:
			return int(v.Int64())
		}
	}

	for _, x := range edges {
		for _, y := range edges {
			bx, by := big.NewInt(int64(x)), big.NewInt(int64(y))

			sum := new(big.Int).Add(bx, by)
			_, of := bitsx.AddOverflow(x, y)
			assert.Equal(t, overflows(sum), of, "%d + %d", x, y)
			assert.Equal(t, clamp(sum), bitsx.AddSaturate(x, y), "%d + %d", x, y)

			prod := new(big.Int).Mul(bx, by)
			_, of = bitsx.MulOverflow(x, y)
			assert.Equal(t, overflows(prod), of, "%d * %d", x, y)
			assert.Equal(t, clamp(prod), bitsx.MulSaturate(x, y), "%d * %d", x, y)
		}
	}
}
