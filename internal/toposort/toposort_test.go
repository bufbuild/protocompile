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

package toposort_test

import (
	"iter"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/toposort"
)

type dag map[int][]int

func (d dag) children(n int) iter.Seq[int] {
	return slices.Values(d[n])
}

func TestSort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		dag   dag
		roots []int
		want  []int
	}{
		{
			name: "empty",
		},
		{
			name:  "list",
			dag:   dag{1: {2}, 2: {3}, 3: {4}, 4: {}},
			roots: []int{1},
			want:  []int{4, 3, 2, 1},
		},
		{
			name:  "list",
			dag:   dag{1: {2}, 2: {3}, 3: {4}, 4: {}},
			roots: []int{2, 1},
			want:  []int{4, 3, 2, 1},
		},
		{
			name:  "list",
			dag:   dag{1: {2}, 2: {3}, 3: {4}, 4: {}},
			roots: []int{1, 2},
			want:  []int{4, 3, 2, 1},
		},
		{
			name:  "diamond",
			dag:   dag{1: {2, 3}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{1},
			want:  []int{4, 3, 2, 1},
		},
		{
			name:  "diamond",
			dag:   dag{1: {2, 3}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{2},
			want:  []int{4, 2},
		},
		{
			name:  "diamond",
			dag:   dag{1: {2, 3}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{2, 3, 1},
			want:  []int{4, 2, 3, 1},
		},
		{
			name:  "diamond",
			dag:   dag{1: {3, 2}, 2: {3}, 3: {}},
			roots: []int{1},
			want:  []int{3, 2, 1},
		},
		{
			name:  "y",
			dag:   dag{1: {2}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{1},
			want:  []int{4, 2, 1},
		},
		{
			name:  "y",
			dag:   dag{1: {2}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{1, 3},
			want:  []int{4, 2, 1, 3},
		},
		{
			name:  "y",
			dag:   dag{1: {2}, 2: {4}, 3: {4}, 4: {}},
			roots: []int{3, 1},
			want:  []int{4, 3, 2, 1},
		},
	}

	var mu sync.Mutex
	s := toposort.Sorter[int, int]{Key: func(n int) int { return n }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize the tests, but run them in an arbitrary order.
			t.Parallel()
			mu.Lock()
			defer mu.Unlock()

			assert.Equal(t, tt.want, slices.Collect(s.Sort(
				tt.roots,
				tt.dag.children,
			)))
		})
	}
}

func TestCycle(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		iterx.Exhaust(toposort.Sort(
			[]int{0},
			func(n int) int { return n },
			func(_ int) iter.Seq[int] { return iterx.Of(0) },
		))
	})
}

func TestReentrant(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		s := toposort.Sorter[int, int]{Key: func(n int) int { return n }}
		dag := func(_ int) iter.Seq[int] { return iterx.Of[int]() }

		for range s.Sort([]int{0}, dag) {
			iterx.Exhaust(s.Sort([]int{0}, dag))
		}
	})
}
