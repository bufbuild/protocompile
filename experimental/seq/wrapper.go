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

// TODO: Would this optimize better if this was a single type parameter
// constrained by interface { Wrap(E) T; Unwrap(T) E }? Indexer values are
// ephemera, so the size of this struct is not crucial, but it would save on
// having to allocate two [runtime.funcval]s when returning an Indexer.

// Wrapper implements [Setter][T] using another Setter as the backing storage,
// and using the given functions to perform the conversion to and from the
// underlying raw values.
type Wrapper[T, E any, S Setter[E]] struct {
	Slice  S
	Wrap   func(E) T
	Unwrap func(T) E
}

// Wrap is a helper for constructing a wrapper without needing to spell out the
// whole type.
func Wrap[T, E any, S Setter[E]](seq S, wrap func(E) T, unwrap func(T) E) Wrapper[T, E, S] {
	return Wrapper[T, E, S]{seq, wrap, unwrap}
}

// Len implements [Indexer].
func (s Wrapper[T, _, _]) Len() int {
	return s.Slice.Len()
}

// At implements [Indexer].
func (s Wrapper[T, _, _]) At(idx int) T {
	return s.Wrap(s.Slice.At(idx))
}

// SetAt implements [Setter].
func (s Wrapper[T, _, _]) SetAt(idx int, value T) {
	s.Slice.SetAt(idx, s.Unwrap(value))
}

// InserterWrapper is like [Wrapper], but also implements [Inserter][T].
type InserterWrapper[T, E any, S Inserter[E]] struct {
	Wrapper[T, E, S]
}

// WrapInserter is a helper for constructing a wrapper without needing to spell out the
// whole type.
func WrapInserter[T, E any, S Inserter[E]](seq S, wrap func(E) T, unwrap func(T) E) InserterWrapper[T, E, S] {
	return InserterWrapper[T, E, S]{Wrap(seq, wrap, unwrap)}
}

// Insert implements [Inserter].
func (s InserterWrapper[T, _, _]) Insert(idx int, value T) {
	s.Slice.Insert(idx, s.Unwrap(value))
}

// Delete implements [Inserter].
func (s InserterWrapper[T, _, _]) Delete(idx int) {
	s.Slice.Delete(idx)
}

// Wrapper2 is like Slice, but it uses a pair of slices instead of one.
//
// This is useful for cases where the raw data is represented in
// a struct-of-arrays format for memory efficiency.
type Wrapper2[T, E1, E2 any, S1 Setter[E1], S2 Setter[E2]] struct {
	// All functions on Slice2 assume that these two slices are
	// always of equal length.
	Slice1 S1
	Slice2 S2

	Wrap   func(E1, E2) T
	Unwrap func(T) (E1, E2)
}

// Wrap2 is a helper for constructing a wrapper without needing to spell out the
// whole type.
func Wrap2[T, E1, E2 any, S1 Setter[E1], S2 Setter[E2]](seq1 S1, seq2 S2, wrap func(E1, E2) T, unwrap func(T) (E1, E2)) Wrapper2[T, E1, E2, S1, S2] {
	return Wrapper2[T, E1, E2, S1, S2]{seq1, seq2, wrap, unwrap}
}

// Len implements [Indexer].
func (s Wrapper2[T, _, _, _, _]) Len() int {
	return s.Slice1.Len()
}

// At implements [Indexer].
func (s Wrapper2[T, _, _, _, _]) At(idx int) T {
	return s.Wrap(s.Slice1.At(idx), s.Slice2.At(idx))
}

// SetAt implements [Setter].
func (s Wrapper2[T, _, _, _, _]) SetAt(idx int, value T) {
	e1, e2 := s.Unwrap(value)
	s.Slice1.SetAt(idx, e1)
	s.Slice2.SetAt(idx, e2)
}

// InserterWrapper2 is like Slice2, but also implements [Inserter][T].
type InserterWrapper2[T, E1, E2 any, S1 Inserter[E1], S2 Inserter[E2]] struct {
	Wrapper2[T, E1, E2, S1, S2]
}

// WrapInserter2 is a helper for constructing a wrapper without needing to spell out the
// whole type.
func WrapInserter2[T, E1, E2 any, S1 Inserter[E1], S2 Inserter[E2]](seq1 S1, seq2 S2, wrap func(E1, E2) T, unwrap func(T) (E1, E2)) InserterWrapper2[T, E1, E2, S1, S2] {
	return InserterWrapper2[T, E1, E2, S1, S2]{Wrap2(seq1, seq2, wrap, unwrap)}
}

// Insert implements [Inserter].
func (s InserterWrapper2[T, _, _, _, _]) Insert(idx int, value T) {
	r1, r2 := s.Unwrap(value)
	s.Slice1.Insert(idx, r1)
	s.Slice2.Insert(idx, r2)
}

// Delete implements [Inserter].
func (s InserterWrapper2[T, _, _, _, _]) Delete(idx int) {
	s.Slice1.Delete(idx)
	s.Slice2.Delete(idx)
}
