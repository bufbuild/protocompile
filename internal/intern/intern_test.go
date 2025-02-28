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

package intern_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/intern"
)

func TestIntern(t *testing.T) {
	t.Parallel()

	data := []string{
		"",
		"a",
		"abc",
		"?",
		"xy.z",
		"a_b_c",
		".....",
		"foo.",
		"foo.a",
		"very long",
		" ",
		"verylong",
	}

	var table intern.Table
	for i := range 3 {
		for _, s := range data {
			t.Run(fmt.Sprintf("%s/%d", s, i), func(t *testing.T) {
				t.Parallel()

				id := table.Intern(s)
				assert.Equal(t, s, table.Value(id), "id: %v", id)
				assert.Equal(t, shouldInline(s), id < 0)
			})
		}
	}
}

func shouldInline(s string) bool {
	if s == "" || len(s) > 5 || strings.HasSuffix(s, ".") {
		return false
	}

	for _, r := range s {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '_', r == '.':

		default:
			return false
		}
	}

	return true
}
