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

package cmpx_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/cmpx"
)

func TestAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b any
		r    cmpx.Result
	}{
		{a: nil, b: nil, r: cmpx.Equal},
		{a: nil, b: 1, r: cmpx.Less},
		{a: 1, b: nil, r: cmpx.Greater},

		{a: false, b: true, r: cmpx.Less},
		{a: true, b: true, r: cmpx.Equal},

		{a: true, b: 0, r: cmpx.Less},

		{a: byte(0), b: int(-1), r: cmpx.Greater},
		{a: byte(0), b: int(0), r: cmpx.Equal},
		{a: byte(0), b: int(1), r: cmpx.Less},

		{a: int(2), b: uint(1), r: cmpx.Greater},
		{a: int(2), b: uint(2), r: cmpx.Equal},
		{a: int(2), b: uint(3), r: cmpx.Less},

		{a: int(math.MaxInt), b: uint(math.MaxUint), r: cmpx.Less},

		{a: 1.5, b: 2, r: cmpx.Less},
		{a: 2, b: 1.5, r: cmpx.Greater},
		{a: 1.5, b: 2.0, r: cmpx.Less},

		{a: "foo", b: "bar", r: cmpx.Greater},
		{a: 1, b: "1", r: cmpx.Less},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.r, cmpx.Any(tt.a, tt.b), "cmpx.Any(%v, %v)", tt.a, tt.b)
	}
}
