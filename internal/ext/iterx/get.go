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

// Package iterx contains extensions to Go's package iter.
package iterx

import (
	"github.com/bufbuild/protocompile/internal/iter"
)

// First retrieves the first element of an iterator.
func First[T any](seq iter.Seq[T]) (v T, ok bool) {
	seq(func(x T) bool {
		v = x
		ok = true
		return false
	})
	return v, ok
}

// OnlyOne retrieves the only element of an iterator.
func OnlyOne[T any](seq iter.Seq[T]) (v T, ok bool) {
	var found T
	seq(func(x T) bool {
		if !ok {
			found = x
		}
		ok = !ok
		return ok
	})
	if ok {
		// Ensure we return the zero value if there is more
		// than one element.
		v = found
	}
	return v, ok
}

// Find returns the first element that matches a predicate.
//
// Returns the value and the index at which it was found, or -1 if it wasn't
// found.
func Find[T any](seq iter.Seq[T], p func(T) bool) (int, T) {
	var v T
	var idx int
	var found bool
	seq(func(x T) bool {
		if p(x) {
			v = x
			found = true
			return false
		}
		idx++
		return true
	})
	if !found {
		idx = -1
	}
	return idx, v
}

// Find2 is like [Find] but for two-element iterators.
func Find2[T, U any](seq iter.Seq2[T, U], p func(T, U) bool) (int, T, U) {
	var v1 T
	var v2 U
	var idx int
	var found bool
	seq(func(x1 T, x2 U) bool {
		if p(x1, x2) {
			v1, v2 = x1, x2
			found = true
			return false
		}
		idx++
		return true
	})
	if !found {
		idx = -1
	}
	return idx, v1, v2
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
