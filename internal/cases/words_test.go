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

package cases_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/cases"
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
		{str: "fooBAR", want: []string{"foo", "BAR"}},
		{str: "FOOBar", want: []string{"FOO", "Bar"}},
		{str: "FooX", want: []string{"Foo", "X"}},
		{str: "FOO", want: []string{"FOO"}},
		{str: "FooBARBaz", want: []string{"Foo", "BAR", "Baz"}},

		// Regression: 2-char [A-Z][a-z] strings produced an empty word list
		// because the upper+lower boundary case and the last-rune case both
		// matched simultaneously, and the last-rune handling was skipped.
		{str: "Ab", want: []string{"Ab"}},
		{str: "Xq", want: []string{"Xq"}},
		{str: "ABc", want: []string{"A", "Bc"}},

		// Cases from heck's snake.rs test suite, translated to raw word
		// segments. Source: https://github.com/withoutboats/heck/blob/master/src/snake.rs
		{str: "CamelCase", want: []string{"Camel", "Case"}},
		{str: "XMLHttpRequest", want: []string{"XML", "Http", "Request"}},
		{str: "abcDEF", want: []string{"abc", "DEF"}},
		{str: "ABcDE", want: []string{"A", "Bc", "DE"}},
		{str: "FieldNamE11", want: []string{"Field", "Nam", "E11"}},
		{str: "abc123def456", want: []string{"abc123def456"}},
		{str: "99BOTTLES", want: []string{"99BOTTLES"}},
		{str: "abc123DEF456", want: []string{"abc123", "DEF456"}},
		{str: "abc123Def456", want: []string{"abc123", "Def456"}},
		{str: "abc123DEf456", want: []string{"abc123", "D", "Ef456"}},
		{str: "ABC123def456", want: []string{"ABC123def456"}},
		{str: "ABC123DEF456", want: []string{"ABC123DEF456"}},
		{str: "ABC123Def456", want: []string{"ABC123", "Def456"}},
		{str: "ABC123DEf456", want: []string{"ABC123D", "Ef456"}},
		{str: "ABC123dEEf456FOO", want: []string{"ABC123d", "E", "Ef456", "FOO"}},
	}

	for _, test := range tests {
		t.Run(test.str, func(t *testing.T) {
			t.Parallel()

			got := slices.Collect(cases.Words(test.str))
			assert.Equal(t, test.want, got)
		})
	}
}
