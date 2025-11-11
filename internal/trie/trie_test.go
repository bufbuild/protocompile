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

package trie_test

import (
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/internal/trie"
	"github.com/stretchr/testify/assert"
)

func TestTrie(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data []string
		keys []string
		want []int
	}{
		{
			data: []string{"fo", "foo", "ba", "bar", "baz"},
			keys: []string{"fo", "foo", "ba", "bar", "baz"},
			want: []int{1, 2, 3, 4, 5},
		},
		{
			data: []string{"fo", "foo", "ba", "bar", "baz"},
			keys: []string{"f", "fooo", "barr", "bazr", "baar"},
			want: []int{0, 2, 4, 5, 3},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			trie := new(trie.Trie[int])
			for i, s := range test.data {
				trie.Insert(s, i+1)
			}
			t.Log(trie.Dump())

			for i, key := range test.keys {
				_, v := trie.Get(key)
				assert.Equal(t, test.want[i], v, "#%d", i)
			}
		})
	}
}

func TestHammerTrie(t *testing.T) {
	t.Parallel()

	trie := new(trie.Trie[int])

	for i := range 1000 {
		trie.Insert(strings.Repeat("a", i), i+1)
	}
	t.Log(trie.Dump())

	for i := range 1000 {
		_, v := trie.Get(strings.Repeat("a", i))
		assert.Equal(t, i+1, v)
	}
}
