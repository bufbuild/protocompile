package interval_test

import (
	"testing"

	"github.com/bufbuild/protocompile/internal/interval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsert(t *testing.T) {
	t.Parallel()
	type r struct {
		start, end int
		value      string
	}

	tests := []struct {
		name   string
		ranges []r    // Ranges to insert.
		want   string // If not "", the value of the overlap for the last range.
	}{
		{
			name:   "empty-map",
			ranges: []r{{0, 9, "foo"}},
		},
		{
			name: "new-max",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
			},
		},
		{
			name: "new-min",
			ranges: []r{
				{30, 39, "bar"},
				{0, 9, "foo"},
			},
		},

		{
			name: "case-1",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 25, "baz"},
			},
		},
		{
			name: "case-1",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 29, "baz"},
			},
		},
		{
			name: "case-1",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 19, "baz"},
			},
		},
		{
			name: "case-1",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 29, "baz"},
			},
		},

		{
			name: "case-2",
			ranges: []r{
				{0, 9, "foo"},
				{1, 2, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-2",
			ranges: []r{
				{0, 9, "foo"},
				{0, 2, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-2",
			ranges: []r{
				{0, 9, "foo"},
				{0, 9, "baz"},
			},
			want: "foo",
		},

		{
			name: "case-3",
			ranges: []r{
				{0, 9, "foo"},
				{9, 12, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-3",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 12, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-3",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 29, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-3",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{9, 30, "baz"},
			},
			want: "foo",
		},

		{
			name: "case-4",
			ranges: []r{
				{0, 10, "foo"},
				{-2, 0, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-4",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{20, 32, "baz"},
			},
			want: "bar",
		},
		{
			name: "case-4",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{10, 32, "baz"},
			},
			want: "bar",
		},

		{
			name: "case-5",
			ranges: []r{
				{0, 9, "foo"},
				{-2, 12, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-5",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{-2, 29, "baz"},
			},
			want: "foo",
		},
		{
			name: "case-5",
			ranges: []r{
				{0, 9, "foo"},
				{30, 39, "bar"},
				{-2, 30, "baz"},
			},
			want: "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			type v struct{ v string } // This aids in pretty-printing for assertions.
			m := new(interval.Map[int, v])
			for i, e := range tt.ranges {
				overlap := m.Insert(e.start, e.end, v{e.value})
				if i < len(tt.ranges)-1 || tt.want == "" {
					require.Nil(t, overlap.Value)
				} else {
					assert.Equal(t, &v{tt.want}, overlap.Value)
				}
				t.Logf("%q", m)
			}
		})
	}
}
