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
	"cmp"
	"iter"
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// MergeKey an n-way merge of sorted slices, using a function to extract a
// comparison key. This function will be called at most once per element.
// The key extraction function is passed both the element of the slice, and
// the index of which of the input slices it was extracted from.
//
// The resulting slice will be sorted, but not necessarily stably. In other
// words, the result is as if by calling Sort(Concat(slices)), but with
// better time complexity.
//
// Time complexity is O(m log n), where m is the total number of elements to
// merge, and n is the number of slices to merge from.
func MergeKey[S ~[]E, E any, K cmp.Ordered](s []S, key func(slice int, elem E) K) S {
	switch len(s) {
	case 0:
		return nil
	case 1:
		return s[0]
		// TODO: can implement other common cases here, such as a pair of slices
		// where the last element of the first is less than the first element of
		// the second.
	}

	return MergeKeySeq(slices.Values(s), key, func(_ int, e E) E { return e })
}

// MergeKeySeq is like [MergeKey], but instead requires callers to provide an
// iterator that yields slices.
//
// Unlike MergeKey, it also permits modifying each element of the output slice
// before it is appended, with the knowledge of which of the input
// slices it came from.
func MergeKeySeq[S ~[]E, E any, K cmp.Ordered, V any](
	slices iter.Seq[S],
	key func(slice int, elem E) K,
	mapper func(slice int, elem E) V,
) []V {
	type entry struct {
		index int
		slice S
	}

	// Holds the slices according to key(slice[0]).
	heap := NewHeap[K, entry](0)

	// Preload the heap with the first entry of each slice. This is also
	// an opportunity to learn the total number of entries so we can allocate
	// a slice of that size.
	var total int
	for i, slice := range iterx.Enumerate(slices) {
		total += len(slice)
		if len(slice) > 0 {
			heap.Insert(key(i, slice[0]), entry{i, slice})
		}
	}

	// As long as there are entries in the queue, pop the first one, whose
	// first entry is the least among all of the slices. Pop the first entry
	// of that slice, write it to output, and the push the rest of the
	// slice back onto the heap.
	output := make([]V, 0, total)
	for heap.Len() > 0 {
		_, entry := heap.Peek()
		output = append(output, mapper(entry.index, entry.slice[0]))

		if len(entry.slice) == 1 {
			heap.Pop()
		} else {
			entry.slice = entry.slice[1:]
			heap.Update(key(entry.index, entry.slice[0]), entry)
		}
	}

	return output
}
