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

package source_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/source/length"
)

func TestLocation(t *testing.T) {
	t.Parallel()

	file := source.NewFile(
		"test",
		"foo\nbar\ncat: 🐈‍⬛\ntail",
	)

	tests := []struct {
		loc  source.Location
		unit length.Unit
	}{
		{loc: source.Location{0, 1, 1}, unit: length.Bytes},
		{loc: source.Location{0, 1, 1}, unit: length.UTF16},
		{loc: source.Location{0, 1, 1}, unit: length.Runes},
		{loc: source.Location{0, 1, 1}, unit: length.TermWidth},

		{loc: source.Location{2, 1, 3}, unit: length.Bytes},
		{loc: source.Location{2, 1, 3}, unit: length.UTF16},
		{loc: source.Location{2, 1, 3}, unit: length.Runes},
		{loc: source.Location{2, 1, 3}, unit: length.TermWidth},

		{loc: source.Location{13, 3, 6}, unit: length.Bytes},
		{loc: source.Location{13, 3, 6}, unit: length.UTF16},
		{loc: source.Location{13, 3, 6}, unit: length.Runes},
		{loc: source.Location{13, 3, 6}, unit: length.TermWidth},

		{loc: source.Location{23, 3, 16}, unit: length.Bytes},
		{loc: source.Location{23, 3, 10}, unit: length.UTF16},
		{loc: source.Location{23, 3, 9}, unit: length.Runes},
		{loc: source.Location{23, 3, 8}, unit: length.TermWidth},
		{loc: source.Location{24, 4, 1}, unit: length.UTF16},
		{loc: source.Location{27, 4, 4}, unit: length.UTF16},
		{loc: source.Location{28, 4, 5}, unit: length.UTF16},
		{loc: source.Location{28, 4, 5}, unit: length.Runes},
		{loc: source.Location{28, 4, 5}, unit: length.Bytes},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			t.Logf("%q | %q", file.Text()[:test.loc.Offset], file.Text()[test.loc.Offset:])
			assert.Equal(t, test.loc, file.Location(test.loc.Offset, test.unit), "offset/%s -> line/col", test.unit)

			if test.unit != length.TermWidth {
				assert.Equal(t, test.loc, file.InverseLocation(test.loc.Line, test.loc.Column, test.unit), "line/col -> offset/%s", test.unit)
			}
		})
	}
}
