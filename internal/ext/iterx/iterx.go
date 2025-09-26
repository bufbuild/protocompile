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
	"fmt"
	"iter"
	"slices"
)

// Empty returns whether an iterator yields any elements.
func Empty[T any](seq iter.Seq[T]) bool {
	for range seq {
		return false
	}
	return true
}

// Empty2 returns whether an iterator yields any elements.
func Empty2[T, U any](seq iter.Seq2[T, U]) bool {
	for range seq {
		return false
	}
	return true
}

// Take returns an iterator over the first n elements of a sequence.
func Take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if n == 0 || !yield(v) {
				return
			}
			n--
		}
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

// Strings maps an iterator with [fmt.Sprint], yielding an iterator of strings.
func Strings[T any](seq iter.Seq[T]) iter.Seq[string] {
	return Map(seq, func(v T) string {
		if s, ok := any(v).(string); ok {
			return s // Avoid dumb copies.
		}
		return fmt.Sprint(v)
	})
}

// Chain returns an iterator that calls a sequence of iterators in sequence.
func Chain[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, seq := range seqs {
			for v := range seq {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Of returns an iterator that yields the given values.
func Of[T any](v ...T) iter.Seq[T] {
	return slices.Values(v)
}
