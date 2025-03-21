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

// This file contains the matrix of {Map, Filter, FilterMap} x {1, 2, 1to2, 2to1},
// except that Filter1to2 and Filter2to1 don't really make sense.

// Map returns a new iterator applying f to each element of seq.
func Map[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return FilterMap(seq, func(v T) (U, bool) { return f(v), true })
}

// FlatMap is like [Map], but expects the yielded type to itself be an iterator.
//
// Returns a new iterator over the concatenation of the iterators yielded by f.
func FlatMap[T, U any](seq iter.Seq[T], f func(T) iter.Seq[U]) iter.Seq[U] {
	return func(yield func(U) bool) {
		for x := range seq {
			for y := range f(x) {
				if !yield(y) {
					break
				}
			}
		}
	}
}

// Filter returns a new iterator that only includes values satisfying p.
func Filter[T any](seq iter.Seq[T], p func(T) bool) iter.Seq[T] {
	return FilterMap(seq, func(v T) (T, bool) { return v, p(v) })
}

// FilterMap combines the operations of [Map] and [Filter].
func FilterMap[T, U any](seq iter.Seq[T], f func(T) (U, bool)) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if v2, ok := f(v); ok && !yield(v2) {
				return
			}
		}
	}
}

// Map2 returns a new iterator applying f to each element of seq.
func Map2[T, U, V, W any](seq iter.Seq2[T, U], f func(T, U) (V, W)) iter.Seq2[V, W] {
	return FilterMap2(seq, func(v1 T, v2 U) (V, W, bool) {
		x1, x2 := f(v1, v2)
		return x1, x2, true
	})
}

// Filter2 returns a new iterator that only includes values satisfying p.
func Filter2[T, U any](seq iter.Seq2[T, U], p func(T, U) bool) iter.Seq2[T, U] {
	return FilterMap2(seq, func(v1 T, v2 U) (T, U, bool) { return v1, v2, p(v1, v2) })
}

// FilterMap2 combines the operations of [Map] and [Filter].
func FilterMap2[T, U, V, W any](seq iter.Seq2[T, U], f func(T, U) (V, W, bool)) iter.Seq2[V, W] {
	return func(yield func(V, W) bool) {
		seq(func(v1 T, v2 U) bool {
			x1, x2, ok := f(v1, v2)
			return !ok || yield(x1, x2)
		})
	}
}

// Map2to1 is like [Map], but it also acts a Y pipe for converting a two-element
// iterator into a one-element iterator.
func Map2to1[T, U, V any](seq iter.Seq2[T, U], f func(T, U) V) iter.Seq[V] {
	return FilterMap2to1(seq, func(v1 T, v2 U) (V, bool) {
		return f(v1, v2), true
	})
}

// FilterMap2to1 is like [FilterMap], but it also acts a Y pipe for converting
// a two-element iterator into a one-element iterator.
func FilterMap2to1[T, U, V any](seq iter.Seq2[T, U], f func(T, U) (V, bool)) iter.Seq[V] {
	return func(yield func(V) bool) {
		seq(func(v1 T, v2 U) bool {
			v, ok := f(v1, v2)
			return !ok || yield(v)
		})
	}
}

// Map1To2 is like [Map], but it also acts a Y pipe for converting a one-element
// iterator into a two-element iterator.
func Map1To2[T, U, V any](seq iter.Seq[T], f func(T) (U, V)) iter.Seq2[U, V] {
	return FilterMap1To2(seq, func(v T) (U, V, bool) {
		x1, x2 := f(v)
		return x1, x2, true
	})
}

// FilterMap1To2 is like [FilterMap], but it also acts a Y pipe for converting
// a one-element iterator into a two-element iterator.
func FilterMap1To2[T, U, V any](seq iter.Seq[T], f func(T) (U, V, bool)) iter.Seq2[U, V] {
	return func(yield func(U, V) bool) {
		seq(func(v T) bool {
			x1, x2, ok := f(v)
			return !ok || yield(x1, x2)
		})
	}
}
