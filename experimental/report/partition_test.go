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

package report

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPartition(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tests := []struct {
		slice   []int
		indices []int
		subs    [][]int
		breakAt int
	}{
		{
			breakAt: -1,
		},
		{
			slice:   []int{1},
			indices: []int{0},
			subs:    [][]int{{1}},
			breakAt: -1,
		},
		{
			slice:   []int{1, 1, 2, 3, 3, 3},
			indices: []int{0, 2, 3},
			subs:    [][]int{{1, 1}, {2}, {3, 3, 3}},
			breakAt: -1,
		},
		{
			slice:   []int{1, 1, 2, 3, 3, 3},
			breakAt: 0,
		},
		{
			slice:   []int{1, 1, 2, 3, 3, 3},
			indices: []int{0},
			subs:    [][]int{{1, 1}},
			breakAt: 1,
		},
		{
			slice:   []int{1, 1, 2, 3, 3, 3},
			indices: []int{0, 2},
			subs:    [][]int{{1, 1}, {2}},
			breakAt: 2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(fmt.Sprint(test.slice), func(t *testing.T) {
			t.Parallel()

			var (
				is    []int
				ss    [][]int
				count int
			)
			it := partition(test.slice, func(a, b *int) bool { return *a != *b })
			it(func(i int, s []int) bool {
				if test.breakAt == count {
					return false
				}
				is = append(is, i)
				ss = append(ss, s)
				count++
				return true
			})

			assert.Equal(test.indices, is)
			assert.Equal(test.subs, ss)
		})
	}
}
