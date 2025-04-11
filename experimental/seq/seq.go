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

// Package seq provides an interface for sequence-like types that can be indexed
// and inserted into.
//
// Protocompile avoids storing slices of its public types as a means of achieving
// memory efficiency. However, this means that it cannot return those slices,
// and thus must use proxy types that implement the interfaces in this package.
package seq

import (
	"iter"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Indexer is a type that can be indexed like a slice.
type Indexer[T any] interface {
	// Len returns the length of this sequence.
	Len() int

	// At returns the element at the given index.
	//
	// Should panic if idx < 0 or idx => Len().
	At(idx int) T
}

// Setter is an [Indexer] that can be mutated by modifying already present
// values.
type Setter[T any] interface {
	Indexer[T]

	// SetAt sets the value of the element at the given index.
	//
	// Should panic if idx < 0 or idx => Len().
	SetAt(idx int, value T)
}

// Inserter is an [Indexer] that can be mutated by inserting or deleting values.
type Inserter[T any] interface {
	Setter[T]

	// Insert inserts an element at the given index.
	//
	// Should panic if idx < 0 or idx > Len().
	Insert(idx int, value T)

	// Delete deletes the element at the given index.
	//
	// Should panic if idx < 0 or idx => Len().
	Delete(idx int)
}

// All returns an iterator over the elements in seq, like [slices.All].
func All[T any](seq Indexer[T]) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		n := seq.Len()
		for i := range n {
			if !yield(i, seq.At(i)) {
				return
			}
		}
	}
}

// Backward returns an iterator over the elements in seq in reverse,
// like [slices.Backward].
func Backward[T any](seq Indexer[T]) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := seq.Len() - 1; i >= 0; i-- {
			if !yield(i, seq.At(i)) {
				return
			}
		}
	}
}

// Values returns an iterator over the elements in seq, like [slices.Values].
func Values[T any](seq Indexer[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		n := seq.Len()
		for i := range n {
			if !yield(seq.At(i)) {
				return
			}
		}
	}
}

// Map is like [slicesx.Map].
func Map[T, U any](seq Indexer[T], f func(T) U) iter.Seq[U] {
	return iterx.Map(Values(seq), f)
}

// Append appends values to an Inserter.
func Append[T any](seq Inserter[T], values ...T) {
	for _, v := range values {
		seq.Insert(seq.Len(), v)
	}
}

// ToSlice copies an [Indexer] into a slice.
func ToSlice[T any](seq Indexer[T]) []T {
	out := make([]T, seq.Len())
	for i := range out {
		out[i] = seq.At(i)
	}
	return out
}
