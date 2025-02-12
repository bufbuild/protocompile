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

// package iterx contains extensions to Go's package iter.
package iterx

import (
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile/internal/iter"
)

// Limit limits a sequence to only yield at most limit times.
func Limit[T any](limit uint, seq iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		seq(func(value T) bool {
			if limit == 0 || !yield(value) {
				return false
			}
			limit--
			return true
		})
	}
}

// First retrieves the first element of an iterator.
func First[T any](seq iter.Seq[T]) (v T, ok bool) {
	seq(func(x T) bool {
		v = x
		ok = true
		return false
	})
	return v, ok
}

// OnlyOne retrieved the only element of an iterator.
func OnlyOne[T any](seq iter.Seq[T]) (v T, ok bool) {
	seq(func(x T) bool {
		if !ok {
			v = x
		}
		ok = !ok
		return ok
	})
	return v, ok
}

// Find returns the first element that matches a predicate.
func Find[T any](seq iter.Seq[T], p func(T) bool) (v T, ok bool) {
	seq(func(x T) bool {
		if p(x) {
			v, ok = x, true
			return false
		}
		return true
	})
	return v, ok
}

// All returns whether every element of an iterator satisfies the given
// predicate. Returns true if seq yields no values.
func All[T any](seq iter.Seq[T], p func(T) bool) bool {
	all := true
	seq(func(v T) bool {
		all = p(v)
		return all
	})
	return all
}

// Count counts the number of elements in seq that match the given predicate.
//
// If p is nil, it is treated as func(_ T) bool { return true }.
func Count[T any](seq iter.Seq[T], p func(T) bool) int {
	var total int
	seq(func(v T) bool {
		if p == nil || p(v) {
			total++
		}
		return true
	})
	return total
}

// Strings maps an iterator with [fmt.Sprint], yielding an iterator of strings.
func Strings[T any](seq iter.Seq[T]) iter.Seq[string] {
	return Map(seq, func(v T) string {
		if s, ok := any(v).(string); ok {
			return s // Avoid dumb copies.
		}
		return fmt.Sprint(v)
	})
}

// Map returns a new iterator applying f to each element of seq.
func Map[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return FilterMap(seq, func(v T) (U, bool) { return f(v), true })
}

// Filter returns a new iterator that only includes values satisfying p.
func Filter[T any](seq iter.Seq[T], p func(T) bool) iter.Seq[T] {
	return FilterMap(seq, func(v T) (T, bool) { return v, p(v) })
}

// FilterMap combines the operations of [Map] and [Filter].
func FilterMap[T, U any](seq iter.Seq[T], f func(T) (U, bool)) iter.Seq[U] {
	return func(yield func(U) bool) {
		seq(func(v T) bool {
			v2, ok := f(v)
			return !ok || yield(v2)
		})
	}
}

// FilterMap1To2 is like [FilterMap], but it also acts a Y pipe for converting a one-element
// iterator into a two-element iterator.
func Map1To2[T, U, V any](seq iter.Seq[T], f func(T) (U, V)) iter.Seq2[U, V] {
	return FilterMap1To2(seq, func(v T) (U, V, bool) {
		x1, x2 := f(v)
		return x1, x2, true
	})
}

// FilterMap1To2 is like [FilterMap], but it also acts a Y pipe for converting a one-element
// iterator into a two-element iterator.
func FilterMap1To2[T, U, V any](seq iter.Seq[T], f func(T) (U, V, bool)) iter.Seq2[U, V] {
	return func(yield func(U, V) bool) {
		seq(func(v T) bool {
			x1, x2, ok := f(v)
			return !ok || yield(x1, x2)
		})
	}
}

// Enumerate adapts an iterator to yield an incrementing index each iteration
// step.
func Enumerate[T any](seq iter.Seq[T]) iter.Seq2[int, T] {
	var i int
	return Map1To2(seq, func(v T) (int, T) {
		i++
		return i - 1, v
	})
}

// Join is like [strings.Join], but works on an iterator. Elements are
// stringified as if by [fmt.Print].
func Join[T any](seq iter.Seq[T], sep string) string {
	var out strings.Builder
	first := true
	seq(func(v T) bool {
		if !first {
			out.WriteString(sep)
		}
		first = false

		fmt.Fprint(&out, v)
		return true
	})
	return out.String()
}

// Chain returns an iterator that calls a sequence of iterators in sequence.
func Chain[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		var done bool
		for _, seq := range seqs {
			if done {
				return
			}
			seq(func(v T) bool {
				done = !yield(v)
				return !done
			})
		}
	}
}
