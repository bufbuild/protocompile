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

package trie

import "strings"

// Trie implements a map from strings to V, except lookups return the key
// which is the longest prefix of a given query.
//
// The zero value is empty and ready to use.
type Trie[V any] struct {
	impl interface {
		get(key string) (prefix string, found int)
		insert(key string) int

		dump(*strings.Builder)
	}
	values []V
}

// Get returns the value corresponding to the longest prefix of key present
// in the trie. The match is exact when len(key) == len(prefix).
//
// If no key in the trie is a prefix of key, returns "" and the zero value of v.
func (t *Trie[V]) Get(key string) (prefix string, value V) {
	if t.impl == nil {
		return "", value
	}

	prefix, n := t.impl.get(key)
	if n == -1 {
		return "", value
	}

	return prefix, t.values[n]
}

// Insert adds a new value to this trie.
func (t *Trie[V]) Insert(key string, value V) {
	if t.impl == nil {
		t.impl = &nybbles[uint8]{}
	}

again:
	n := t.impl.insert(key)
	if n == -1 {
		switch impl := t.impl.(type) {
		case *nybbles[uint8]:
			t.impl = grow[uint16](impl)
		case *nybbles[uint16]:
			t.impl = grow[uint32](impl)
		case *nybbles[uint32]:
			t.impl = grow[uint64](impl)
		default:
			panic("unreachable")
		}

		goto again
	}

	if len(t.values) <= n {
		t.values = append(t.values, make([]V, n+1-len(t.values))...)
	}
	t.values[n] = value
}
