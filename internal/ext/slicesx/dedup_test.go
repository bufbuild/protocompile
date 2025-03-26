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

package slicesx_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

func TestDedup(t *testing.T) {
	t.Parallel()

	type V [2]int
	first := func(x V) int { return x[0] }
	second := func(x V) int { return x[1] }
	tests := []struct {
		input, want []V
	}{
		{
			input: []V{},
			want:  []V{},
		},
		{
			input: []V{
				{1, 2}, {1, 1}, {1, 3}, {2, 2}, {3, 1}, {3, 3},
			},
			want: []V{
				{1, 1}, {2, 2}, {3, 1},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, slicesx.DedupKey(test.input, first, func(run []V) V {
				return slices.MinFunc(run, cmpx.Key(second))
			}))
		})
	}
}
