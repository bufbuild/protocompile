// Copyright 2020-2024 Buf Technologies, Inc.
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

import "cmp"

// MergeKey an n-way merge of sorted slices, using a function to extract a
// comparison key. This function will be called at most once per element.
//
// The resulting slice will be sorted, but not necessarily stably. In other
// words, the result is as if by calling Sort(Concat(slices)), but with
// better time complexity.
//
// Time complexity is O(m log n), where m is the total number of elements to
// merge, and n is the number of slices to merge from.
func MergeKey[T any, K cmp.Ordered](slices [][]T, key func(T) K) []T {
	switch len(slices) {
	case 0:
		return nil
	case 1:
		return slices[0]
		// TODO: can implement other common cases here, such as a pair of slices
		// where the last element of the first is less than the first element of
		// the second.
	}

	// Holds the slices according to key(slice[0]).
	heap := NewHeap[K, []T](len(slices))

	// Preload the heap with the first entry of each slice. This is also
	// an opportunity to learn the total number of entries so we can allocate
	// a slice of that size.
	var total int
	for _, slice := range slices {
		total += len(slice)
		if len(slice) > 0 {
			heap.Insert(key(slice[0]), slice)
		}
	}

	// As long as there are entries in the queue, pop the first one, whose
	// first entry is the least among all of the slices. Pop the first entry
	// of that slice, write it to output, and the push the rest of the
	// slice back onto the heap.
	output := make([]T, 0, total)
	for heap.Len() > 0 {
		_, slice := heap.Peek()
		output = append(output, slice[0])

		if len(slice) == 1 {
			heap.Pop()
		} else {
			slice = slice[1:]
			heap.Update(key(slice[0]), slice)
		}
	}

	return output
}
