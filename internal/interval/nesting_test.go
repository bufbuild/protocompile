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

package interval_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/interval"
)

func TestNesting(t *testing.T) {
	t.Parallel()
	type in struct {
		start, end int
		value      string
	}

	tests := []struct {
		name   string
		ranges []in // Ranges to insert.
		want   [][]interval.Entry[int, string]
	}{
		{
			name: "three disjoint",
			ranges: []in{
				{1, 2, "foo"},
				{8, 9, "bar"},
				{4, 6, "baz"},
			},
			want: [][]interval.Entry[int, string]{{
				{Start: 1, End: 2, Value: "foo"},
				{Start: 4, End: 6, Value: "baz"},
				{Start: 8, End: 9, Value: "bar"},
			}},
		},
		{
			name: "towers",
			ranges: []in{
				{1, 10, "foo"},
				{5, 15, "bar"},
				{4, 9, "foo1"},
				{9, 11, "bar1"},
			},
			want: [][]interval.Entry[int, string]{
				{
					{Start: 1, End: 10, Value: "foo"},
					{Start: 4, End: 9, Value: "foo1"},
				},
				{
					{Start: 5, End: 15, Value: "bar"},
					{Start: 9, End: 11, Value: "bar1"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var nesting interval.Nesting[int, string]
			for _, r := range test.ranges {
				nesting.Insert(r.start, r.end, r.value)
			}

			var got [][]interval.Entry[int, string]
			for set := range nesting.Sets() {
				s := slices.Collect(set)
				t.Log(s)
				slices.SortStableFunc(s, cmpx.Key(func(e interval.Entry[int, string]) int {
					return e.Start
				}))
				got = append(got, s)
			}

			assert.Equal(t, test.want, got)
		})
	}
}
