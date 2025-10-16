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

package report_test

import (
	"testing"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/stretchr/testify/assert"
)

func TestLocation(t *testing.T) {
	t.Parallel()

	file := report.NewFile(
		"test",
		"foo\nbar\ncat: 🐈‍⬛\n",
	)

	tests := []struct {
		loc  report.Location
		unit report.LengthUnit
	}{
		{loc: report.Location{0, 1, 1}, unit: report.ByteLength},
		{loc: report.Location{0, 1, 1}, unit: report.UTF16Length},
		{loc: report.Location{0, 1, 1}, unit: report.RuneLength},
		{loc: report.Location{0, 1, 1}, unit: report.TermWidth},

		{loc: report.Location{2, 1, 3}, unit: report.ByteLength},
		{loc: report.Location{2, 1, 3}, unit: report.UTF16Length},
		{loc: report.Location{2, 1, 3}, unit: report.RuneLength},
		{loc: report.Location{2, 1, 3}, unit: report.TermWidth},

		{loc: report.Location{13, 3, 6}, unit: report.ByteLength},
		{loc: report.Location{13, 3, 6}, unit: report.UTF16Length},
		{loc: report.Location{13, 3, 6}, unit: report.RuneLength},
		{loc: report.Location{13, 3, 6}, unit: report.TermWidth},

		{loc: report.Location{23, 3, 16}, unit: report.ByteLength},
		{loc: report.Location{23, 3, 10}, unit: report.UTF16Length},
		{loc: report.Location{23, 3, 9}, unit: report.RuneLength},
		{loc: report.Location{23, 3, 8}, unit: report.TermWidth},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Logf("%q | %q", file.Text()[:test.loc.Offset], file.Text()[test.loc.Offset:])
			assert.Equal(t, test.loc, file.Location(test.loc.Offset, test.unit), "offset/%s -> line/col", test.unit)

			if test.unit != report.TermWidth {
				assert.Equal(t, test.loc, file.InverseLocation(test.loc.Line, test.loc.Column, test.unit), "line/col -> offset/%s", test.unit)
			}
		})
	}
}
