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

package slicesx

// Dedup replaces runs of consecutive equal elements with a single element.
func Dedup[S ~[]E, E comparable](s S) S {
	return DedupKey(s, func(e E) E { return e }, func(e []E) E { return e[0] })
}

// DedupKey deduplicates consecutive elements in a slice, using key to obtain
// a key to deduplicate by, and choose to select which element in a run to keep.
func DedupKey[S ~[]E, E any, K comparable](
	s S,
	key func(E) K,
	choose func([]E) E,
) S {
	return dedup(s, func(a, b E) bool { return key(a) == key(b) }, choose)
}

// DedupFunc deduplicates consecutive elements in a slice based on the equal function. If
// equal returns true, then two elements are considered duplicates, and we always pick the
// first element to keep.
func DedupFunc[S ~[]E, E any](s S, equal func(E, E) bool) S {
	return dedup(s, equal, func(e []E) E { return e[0] })
}

func dedup[S ~[]E, E any](s S, equal func(E, E) bool, choose func([]E) E) S {
	if len(s) == 0 {
		return s
	}

	i := 0 // Index to write the next value at.
	j := 0 // Index of prev.

	prev := s[i]
	for k := 1; k < len(s); k++ {
		next := s[k]
		if equal(prev, next) {
			continue
		}

		s[i] = choose(s[j:k])
		i++
		j = k
		prev = next
	}

	s[i] = choose(s[j:])
	return s[:i+1]
}
