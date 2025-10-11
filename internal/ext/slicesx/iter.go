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

import (
	"iter"
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Map is a helper for generating a mapped iterator over a slice, to avoid
// a noisy call to [Values].
func Map[S ~[]E, E, U any](s S, f func(E) U) iter.Seq[U] {
	return iterx.Map(slices.Values(s), f)
}

// Join is a helper for applying [iterx.Join] to a slice.
func Join[S ~[]E, E any](s S, sep string) string {
	return iterx.Join(slices.Values(s), sep)
}

// Transform is like calling [slices.Collect] with [Map], but is able to
// preallocate.
func Transform[S ~[]E, E, U any](s S, f func(E) U) []U {
	out := make([]U, len(s))
	for i, e := range s {
		out[i] = f(e)
	}
	return out
}

// Pointers returns an iterator over pointers to values in s.
func Pointers[S ~[]E, E any](s S) iter.Seq2[int, *E] {
	return func(yield func(int, *E) bool) {
		for i := range s {
			if !yield(i, &s[i]) {
				return
			}
		}
	}
}

// PartitionFunc returns an iterator of the largest substrings of s of equal
// elements.
//
// In other words, suppose key is the identity function. Then, the slice
// [a a a b c c] is yielded as the subslices [a a a], [b], and [c c c].
//
// The iterator also yields the index at which each subslice begins.
//
// Will never yield an empty slice.
//
//nolint:dupword
func Partition[S ~[]E, E comparable](s S) iter.Seq2[int, S] {
	return PartitionKey(s, func(e E) E { return e })
}

// PartitionKey is like [Partition], but instead the subslices are all such
// that ever element has the same value for key(e).
//
// [Partition] is equivalent to PartitionKey with the identity function.
func PartitionKey[S ~[]E, E any, K comparable](s S, key func(E) K) iter.Seq2[int, S] {
	return func(yield func(int, S) bool) {
		var start int
		var prev K
		for i, r := range s {
			next := key(r)
			if i == 0 {
				prev = next
				continue
			}

			if prev == next {
				continue
			}

			if !yield(start, s[start:i]) {
				return
			}

			start = i
			prev = next
		}

		if start < len(s) {
			yield(start, s[start:])
		}
	}
}

// PartitionFunc is like [Partition], but instead the subslices are split
// whenever split returns true for adjacent elements.
//
// [Partition] is PartitionFunc with != as the splitting function.
func PartitionFunc[S ~[]E, E any](s S, split func(E, E) bool) iter.Seq2[int, S] {
	return func(yield func(int, S) bool) {
		var start int
		var prev E
		for i, next := range s {
			if i == 0 {
				prev = next
				continue
			}

			if !split(prev, next) {
				prev = next
				continue
			}

			if !yield(start, s[start:i]) {
				return
			}

			start = i
			prev = next
		}

		if start < len(s) {
			yield(start, s[start:])
		}
	}
}
