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

package seq

import (
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// TODO: Would this optimize better if Wrap/Unwrap was a single type parameter
// constrained by interface { Wrap(E) T; Unwrap(T) E }? Indexer values are
// ephemera, so the size of this struct is not crucial, but it would save on
// having to allocate two [runtime.funcval]s when returning an Indexer.

// Slice implements [Indexer][T] using an ordinary slice as the backing storage,
// and using the given functions to perform the conversion to and from the
// underlying raw values.
//
// The first argument of Wrap/Unwrap given is the index the value has/will have
// in the slice.
type Slice[T, E any] struct {
	Slice  []E
	Wrap   func(int, E) T
	Unwrap func(int, T) E
}

// NewSlice constructs a new [Slice].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewSlice[T, E any](
	slice []E,
	wrap func(int, E) T,
	unwrap func(int, T) E,
) Slice[T, E] {
	return Slice[T, E]{slice, wrap, unwrap}
}

// NewFixedSlice constructs a new [Slice] whose Set method panics. This
// function is intended for cases where the [Slice] will immediately be turned
// into an [Indexer].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewFixedSlice[T, E any](
	slice []E,
	wrap func(int, E) T,
) Slice[T, E] {
	return Slice[T, E]{slice, wrap, nil}
}

// Len implements [Indexer].
func (s Slice[T, _]) Len() int {
	return len(s.Slice)
}

// At implements [Indexer].
func (s Slice[T, _]) At(idx int) T {
	return s.Wrap(idx, s.Slice[idx])
}

// SetAt implements [Setter].
func (s Slice[T, _]) SetAt(idx int, value T) {
	s.Slice[idx] = s.Unwrap(idx, value)
}

// SliceInserter is like [Slice], but also implements [Inserter][T].
type SliceInserter[T, E any] struct {
	Slice  *[]E
	Wrap   func(int, E) T
	Unwrap func(int, T) E
}

var empty []uint64 // Maximally aligned.

// EmptySliceInserter returns a [SliceInserter] that is always empty and whose
// insertion operations panic.
func EmptySliceInserter[T, E any]() SliceInserter[T, E] {
	return NewSliceInserter(
		unsafex.Bitcast[*[]E](&empty),
		func(_ int, _ E) T {
			var z T
			return z
		},
		nil,
	)
}

// NewSliceInserter constructs a new [SliceInserter].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewSliceInserter[T, E any](
	slice *[]E,
	wrap func(int, E) T,
	unwrap func(int, T) E,
) SliceInserter[T, E] {
	return SliceInserter[T, E]{slice, wrap, unwrap}
}

// Len implements [Indexer].
func (s SliceInserter[T, _]) Len() int {
	if s.Slice == nil {
		return 0
	}
	return len(*s.Slice)
}

// At implements [Indexer].
func (s SliceInserter[T, _]) At(idx int) T {
	return s.Wrap(idx, (*s.Slice)[idx])
}

// SetAt implements [Setter].
func (s SliceInserter[T, _]) SetAt(idx int, value T) {
	(*s.Slice)[idx] = s.Unwrap(idx, value)
}

// Insert implements [Inserter].
func (s SliceInserter[T, _]) Insert(idx int, value T) {
	*s.Slice = slices.Insert(*s.Slice, idx, s.Unwrap(idx, value))
}

// Delete implements [Inserter].
func (s SliceInserter[T, _]) Delete(idx int) {
	*s.Slice = slices.Delete(*s.Slice, idx, idx+1)
}

// Slice2 is like Slice, but it uses a pair of slices instead of one.
//
// This is useful for cases where the raw data is represented in
// a struct-of-arrays format for memory efficiency.
type Slice2[T, E1, E2 any] struct {
	// All functions on Slice2 assume that these two slices are
	// always of equal length.
	Slice1 []E1
	Slice2 []E2

	Wrap   func(int, E1, E2) T
	Unwrap func(int, T) (E1, E2)
}

// NewSlice2 constructs a new [Slice2].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewSlice2[T, E1, E2 any](
	slice1 []E1,
	slice2 []E2,
	wrap func(int, E1, E2) T,
	unwrap func(int, T) (E1, E2),
) Slice2[T, E1, E2] {
	return Slice2[T, E1, E2]{slice1, slice2, wrap, unwrap}
}

// Len implements [Indexer].
func (s Slice2[T, _, _]) Len() int {
	return len(s.Slice1)
}

// At implements [Indexer].
func (s Slice2[T, _, _]) At(idx int) T {
	return s.Wrap(idx, s.Slice1[idx], s.Slice2[idx])
}

// SetAt implements [Setter].
func (s Slice2[T, _, _]) SetAt(idx int, value T) {
	s.Slice1[idx], s.Slice2[idx] = s.Unwrap(idx, value)
}

// SliceInserter2 is like Slice2, but also implements [Inserter][T].
type SliceInserter2[T, E1, E2 any] struct {
	// All functions on SliceInserter2 assume that these two slices are
	// always of equal length.
	Slice1 *[]E1
	Slice2 *[]E2

	Wrap   func(int, E1, E2) T
	Unwrap func(int, T) (E1, E2)
}

// NewSliceInserter2 constructs a new [SliceInserter2].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewSliceInserter2[T, E1, E2 any](
	slice1 *[]E1,
	slice2 *[]E2,
	wrap func(int, E1, E2) T,
	unwrap func(int, T) (E1, E2),
) SliceInserter2[T, E1, E2] {
	return SliceInserter2[T, E1, E2]{slice1, slice2, wrap, unwrap}
}

// Len implements [Indexer].
func (s SliceInserter2[T, _, _]) Len() int {
	if s.Slice1 == nil {
		return 0
	}
	return len(*s.Slice1)
}

// At implements [Indexer].
func (s SliceInserter2[T, _, _]) At(idx int) T {
	return s.Wrap(idx, (*s.Slice1)[idx], (*s.Slice2)[idx])
}

// SetAt implements [Setter].
func (s SliceInserter2[T, _, _]) SetAt(idx int, value T) {
	(*s.Slice1)[idx], (*s.Slice2)[idx] = s.Unwrap(idx, value)
}

// Insert implements [Inserter].
func (s SliceInserter2[T, _, _]) Insert(idx int, value T) {
	r1, r2 := s.Unwrap(idx, value)
	*s.Slice1 = slices.Insert(*s.Slice1, idx, r1)
	*s.Slice2 = slices.Insert(*s.Slice2, idx, r2)
}

// Delete implements [Inserter].
func (s SliceInserter2[T, _, _]) Delete(idx int) {
	*s.Slice1 = slices.Delete(*s.Slice1, idx, idx+1)
	*s.Slice2 = slices.Delete(*s.Slice2, idx, idx+1)
}
