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

package seq

import "slices"

// TODO: Would this optimize better if this was a single type parameter
// constrained by interface { Wrap(E) T; Unwrap(T) E }? Indexer values are
// ephemera, so the size of this struct is not crucial, but it would save on
// having to allocate two [runtime.funcval]s when returning an Indexer.

// Slice implements [Indexer][T] using an ordinary slice as the backing storage,
// and using the given functions to perform the conversion to and from the
// underlying raw values.
type Slice[T, E any] struct {
	Slice  []E
	Wrap   func(E) T
	Unwrap func(T) E
}

// Len implements [Indexer].
func (s Slice[T, _]) Len() int {
	return len(s.Slice)
}

// At implements [Indexer].
func (s Slice[T, _]) At(idx int) T {
	return s.Wrap(s.Slice[idx])
}

// SetAt implements [Setter].
func (s Slice[T, _]) SetAt(idx int, value T) {
	s.Slice[idx] = s.Unwrap(value)
}

// SliceInserter is like [Slice], but also implements [Inserter][T].
type SliceInserter[T, E any] struct {
	Slice  *[]E
	Wrap   func(E) T
	Unwrap func(T) E
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
	return s.Wrap((*s.Slice)[idx])
}

// SetAt implements [Setter].
func (s SliceInserter[T, _]) SetAt(idx int, value T) {
	(*s.Slice)[idx] = s.Unwrap(value)
}

// Insert implements [Inserter].
func (s SliceInserter[T, _]) Insert(idx int, value T) {
	*s.Slice = slices.Insert(*s.Slice, idx, s.Unwrap(value))
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

	Wrap   func(E1, E2) T
	Unwrap func(T) (E1, E2)
}

// Len implements [Indexer].
func (s Slice2[T, _, _]) Len() int {
	return len(s.Slice1)
}

// At implements [Indexer].
func (s Slice2[T, _, _]) At(idx int) T {
	return s.Wrap(s.Slice1[idx], s.Slice2[idx])
}

// SetAt implements [Setter].
func (s Slice2[T, _, _]) SetAt(idx int, value T) {
	s.Slice1[idx], s.Slice2[idx] = s.Unwrap(value)
}

// SliceInserter2 is like Slice2, but also implements [Inserter][T].
type SliceInserter2[T, E1, E2 any] struct {
	// All functions on SliceInserter2 assume that these two slices are
	// always of equal length.
	Slice1 *[]E1
	Slice2 *[]E2

	Wrap   func(E1, E2) T
	Unwrap func(T) (E1, E2)
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
	return s.Wrap((*s.Slice1)[idx], (*s.Slice2)[idx])
}

// SetAt implements [Setter].
func (s SliceInserter2[T, _, _]) SetAt(idx int, value T) {
	(*s.Slice1)[idx], (*s.Slice2)[idx] = s.Unwrap(value)
}

// Insert implements [Inserter].
func (s SliceInserter2[T, _, _]) Insert(idx int, value T) {
	r1, r2 := s.Unwrap(value)
	*s.Slice1 = slices.Insert(*s.Slice1, idx, r1)
	*s.Slice2 = slices.Insert(*s.Slice2, idx, r2)
}

// Delete implements [Inserter].
func (s SliceInserter2[T, _, _]) Delete(idx int) {
	*s.Slice1 = slices.Delete(*s.Slice1, idx, idx+1)
	*s.Slice2 = slices.Delete(*s.Slice2, idx, idx+1)
}
