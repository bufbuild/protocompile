package cases_test

import (
	"slices"
	"testing"

	"github.com/bufbuild/protocompile/internal/cases"
	"github.com/stretchr/testify/assert"
)

func TestWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		str  string
		want []string
	}{
		{str: ""},
		{str: "_"},
		{str: "__"},

		{str: "foo", want: []string{"foo"}},
		{str: "_foo", want: []string{"foo"}},
		{str: "foo_", want: []string{"foo"}},
		{str: "foo_bar", want: []string{"foo", "bar"}},
		{str: "foo__bar", want: []string{"foo", "bar"}},

		{str: "fooBar", want: []string{"foo", "Bar"}},
		{str: "foo_Bar", want: []string{"foo", "Bar"}},
		{str: "FOOBar", want: []string{"FOO", "Bar"}},
		{str: "FooX", want: []string{"Foo", "X"}},
		{str: "FOO", want: []string{"FOO"}},
		{str: "FooBARBaz", want: []string{"FooBAR", "Baz"}},
	}

	for _, test := range tests {
		t.Run(test.str, func(t *testing.T) {
			got := slices.Collect(cases.Words(test.str))
			assert.Equal(t, test.want, got)
		})
	}
}
