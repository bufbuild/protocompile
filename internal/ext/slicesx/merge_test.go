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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	type V [2]int
	first := func(_ int, x V) int { return x[0] }
	tests := []struct {
		slices [][]V
		want   []V
	}{
		{
			slices: nil,
			want:   nil,
		},
		{
			slices: [][]V{{
				{1, 28}, {12, 31}, {17, 79}, {40, 62}, {55, 98},
				{59, 2}, {60, 37}, {66, 60}, {72, 13}, {98, 25}}},
			want: []V{
				{1, 28}, {12, 31}, {17, 79}, {40, 62}, {55, 98},
				{59, 2}, {60, 37}, {66, 60}, {72, 13}, {98, 25}},
		},
		{
			slices: [][]V{
				{
					{1, 28}, {12, 31}, {17, 79}, {40, 62}, {55, 98},
					{59, 2}, {60, 37}, {66, 60}, {72, 13}, {98, 25},
				},
				{
					{7, 32}, {32, 15}, {39, 64}, {69, 54}, {82, 97},
					{83, 61}, {91, 27}, {95, 81}, {97, 54}, {98, 1},
				},
			},
			want: []V{
				{1, 28}, {7, 32}, {12, 31}, {17, 79}, {32, 15},
				{39, 64}, {40, 62}, {55, 98}, {59, 2}, {60, 37},
				{66, 60}, {69, 54}, {72, 13}, {82, 97}, {83, 61},
				{91, 27}, {95, 81}, {97, 54}, {98, 1}, {98, 25}},
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, slicesx.MergeKey(test.slices, first))
	}
}
