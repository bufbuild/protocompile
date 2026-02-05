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
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/interval"
)

func TestInsert(t *testing.T) {
	t.Parallel()
	type in struct {
		start, end int
		value      string
	}
	type out = interval.Entry[int, []string]

	tests := []struct {
		name   string
		ranges []in // Ranges to insert.
		want   []out
		join   []out
	}{
		{
			name:   "empty-map",
			ranges: []in{{0, 9, "foo"}},
			want: []out{
				{0, 9, []string{"foo"}},
			},
			join: []out{{0, 9, nil}},
		},
		{
			name: "new-max",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 9, nil}, {30, 39, nil}},
		},
		{
			name: "new-min",
			ranges: []in{
				{30, 39, "bar"},
				{0, 9, "foo"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 9, nil}, {30, 39, nil}},
		},

		{
			name: "case-1",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 25, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{20, 25, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 9, nil}, {20, 25, nil}, {30, 39, nil}},
		},
		{
			name: "case-1",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 29, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{20, 29, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 9, nil}, {20, 39, nil}},
		},
		{
			name: "case-1",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 19, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{10, 19, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 19, nil}, {30, 39, nil}},
		},
		{
			name: "case-1",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 29, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{10, 29, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 39, nil}},
		},

		{
			name: "case-2",
			ranges: []in{
				{0, 9, "foo"},
				{1, 2, "baz"},
			},
			want: []out{
				{0, 0, []string{"foo"}},
				{1, 2, []string{"foo", "baz"}},
				{3, 9, []string{"foo"}},
			},
			join: []out{{0, 9, nil}},
		},
		{
			name: "case-2",
			ranges: []in{
				{0, 9, "foo"},
				{0, 2, "baz"},
			},
			want: []out{
				{0, 2, []string{"foo", "baz"}},
				{3, 9, []string{"foo"}},
			},
			join: []out{{0, 9, nil}},
		},
		{
			name: "case-2",
			ranges: []in{
				{0, 9, "foo"},
				{0, 9, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo", "baz"}},
			},
			join: []out{{0, 9, nil}},
		},

		{
			name: "case-3",
			ranges: []in{
				{0, 9, "foo"},
				{9, 12, "baz"},
			},
			want: []out{
				{0, 8, []string{"foo"}},
				{9, 9, []string{"foo", "baz"}},
				{10, 12, []string{"baz"}},
			},
			join: []out{{0, 12, nil}},
		},
		{
			name: "case-3",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 12, "baz"},
			},
			want: []out{
				{0, 8, []string{"foo"}},
				{9, 9, []string{"foo", "baz"}},
				{10, 12, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 12, nil}, {30, 39, nil}},
		},
		{
			name: "case-3",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 29, "baz"},
			},
			want: []out{
				{0, 8, []string{"foo"}},
				{9, 9, []string{"foo", "baz"}},
				{10, 29, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{0, 39, nil}},
		},
		{
			name: "case-3",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 30, "baz"},
			},
			want: []out{
				{0, 8, []string{"foo"}},
				{9, 9, []string{"foo", "baz"}},
				{10, 29, []string{"baz"}},
				{30, 30, []string{"bar", "baz"}},
				{31, 39, []string{"bar"}},
			},
			join: []out{{0, 39, nil}},
		},

		{
			name: "case-4",
			ranges: []in{
				{0, 10, "foo"},
				{-2, 0, "baz"},
			},
			want: []out{
				{-2, -1, []string{"baz"}},
				{0, 0, []string{"foo", "baz"}},
				{1, 10, []string{"foo"}},
			},
			join: []out{{-2, 10, nil}},
		},
		{
			name: "case-4",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 32, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{20, 29, []string{"baz"}},
				{30, 32, []string{"bar", "baz"}},
				{33, 39, []string{"bar"}},
			},
			join: []out{{0, 9, nil}, {20, 39, nil}},
		},
		{
			name: "case-4",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 32, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{10, 29, []string{"baz"}},
				{30, 32, []string{"bar", "baz"}},
				{33, 39, []string{"bar"}},
			},
			join: []out{{0, 39, nil}},
		},

		{
			name: "case-5",
			ranges: []in{
				{0, 9, "foo"},
				{-2, 12, "baz"},
			},
			want: []out{
				{-2, -1, []string{"baz"}},
				{0, 9, []string{"foo", "baz"}},
				{10, 12, []string{"baz"}},
			},
			join: []out{{-2, 12, nil}},
		},
		{
			name: "case-5",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{-2, 29, "baz"},
			},
			want: []out{
				{-2, -1, []string{"baz"}},
				{0, 9, []string{"foo", "baz"}},
				{10, 29, []string{"baz"}},
				{30, 39, []string{"bar"}},
			},
			join: []out{{-2, 39, nil}},
		},
		{
			name: "case-5",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{-2, 30, "baz"},
			},
			want: []out{
				{-2, -1, []string{"baz"}},
				{0, 9, []string{"foo", "baz"}},
				{10, 29, []string{"baz"}},
				{30, 30, []string{"bar", "baz"}},
				{31, 39, []string{"bar"}},
			},
			join: []out{{-2, 39, nil}},
		},
		{
			name: "case-5",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{29, 40, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{29, 29, []string{"baz"}},
				{30, 39, []string{"bar", "baz"}},
				{40, 40, []string{"baz"}},
			},
			join: []out{{0, 9, nil}, {29, 40, nil}},
		},
		{
			name: "case-5",
			ranges: []in{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{29, math.MaxInt, "baz"},
			},
			want: []out{
				{0, 9, []string{"foo"}},
				{29, 29, []string{"baz"}},
				{30, 39, []string{"bar", "baz"}},
				{40, math.MaxInt, []string{"baz"}},
			},
			join: []out{{0, 9, nil}, {29, math.MaxInt, nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := new(interval.Intersect[int, string])
			for _, e := range tt.ranges {
				m.Insert(e.start, e.end, e.value)
			}

			assert.Equal(t, tt.want, slices.Collect(m.Entries()))
			assert.Equal(t, tt.join, slices.Collect(m.Contiguous(false)))
		})
	}
}
