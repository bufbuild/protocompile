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

package iterx

import (
	"iter"
)

// First retrieves the first element of an iterator.
func First[T any](seq iter.Seq[T]) (v T, ok bool) {
	for v = range seq {
		ok = true
		break
	}
	return v, ok
}

// First retrieves the first element a two-element iterator.
func First2[K, V any](seq iter.Seq2[K, V]) (k K, v V, ok bool) {
	for k, v = range seq {
		ok = true
		break
	}
	return k, v, ok
}

// Last retrieves the last element of an iterator.
func Last[T any](seq iter.Seq[T]) (v T, ok bool) {
	for v = range seq {
		ok = true
	}
	return v, ok
}

// Last retrieves the last element of a two-element iterator.
func Last2[K, V any](seq iter.Seq2[K, V]) (k K, v V, ok bool) {
	for k, v = range seq {
		ok = true
	}
	return k, v, ok
}

// OnlyOne retrieves the only element of an iterator.
func OnlyOne[T any](seq iter.Seq[T]) (v T, ok bool) {
	for i, x := range Enumerate(seq) {
		if i > 0 {
			var z T
			// Ensure we return the zero value if there is more
			// than one element.
			return z, false
		}
		v = x
		ok = true
	}
	return v, ok
}

// Find returns the first element that matches a predicate.
//
// Returns the value and the index at which it was found, or -1 if it wasn't
// found.
func Find[T any](seq iter.Seq[T], p func(T) bool) (int, T) {
	for i, x := range Enumerate(seq) {
		if p(x) {
			return i, x
		}
	}
	var z T
	return -1, z
}

// Find2 is like [Find] but for two-element iterators.
func Find2[T, U any](seq iter.Seq2[T, U], p func(T, U) bool) (int, T, U) {
	var i int
	for x1, x2 := range seq {
		if p(x1, x2) {
			return i, x1, x2
		}
		i++
	}
	var z1 T
	var z2 U
	return -1, z1, z2
}

// Index returns the index of the first element of seq that satisfies p.
//
// if not found, returns -1.
func Index[T any](seq iter.Seq[T], p func(T) bool) int {
	idx, _ := Find(seq, p)
	return idx
}

// Index2 is like [Index], but for two-element iterators.
func Index2[T, U any](seq iter.Seq2[T, U], p func(T, U) bool) int {
	idx, _, _ := Find2(seq, p)
	return idx
}

// Contains whether an element exists that satisfies p.
func Contains[T any](seq iter.Seq[T], p func(T) bool) bool {
	idx, _ := Find(seq, p)
	return idx != -1
}

// Contains2 is like [Contains], but for two-element iterators.
func Contains2[T, U any](seq iter.Seq2[T, U], p func(T, U) bool) bool {
	idx, _, _ := Find2(seq, p)
	return idx != -1
}
