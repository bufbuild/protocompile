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

package ast2

import (
	"fmt"
	"math/bits"
	"strings"
)

// pointersMinLenShift is the log2 of the size of the smallest slice in
// a pointers[T].
const (
	pointersMinLenShift = 4
	pointersMinLen      = 1 << pointersMinLenShift
)

// Slice is a type that offers the same interface as an ordinary Go
// slice.
//
// This is used to provide a consistent interface to various AST nodes that
// contain a variable number of "something", but the actual backing array
// is some compressed representation.
type Slice[T any] interface {
	// Len returns this slice's length.
	Len() int

	// At returns the nth value of this slice.
	//
	// Panics if n >= Len().
	At(n int) T

	// Iter is an iterator over the slice.
	Iter(yield func(int, T) bool)
}

// Commas is like [Slice], but it's for a comma-delimited list of some kind.
//
// This makes it easy to work with the list as though it's a slice, while also
// allowing access to the commas.
type Commas[T any] interface {
	Slice[T]

	// Comma is like [Slice.At] but returns the comma that follows the nth
	// element.
	Comma(n int) Token
}

// pointers is a growable slice that provides pointer stability: it does not
// copy its elements when resized. It implements [Slice[*T]].
//
// It does this by maintaining a table of logarithmically-growing slices that mimic
// the resizing behavior of an ordinary slice. This trades off the linear 8-byte
// overhead of []*T for a logarithmic 24-byte overhead. Lookup time remains O(1).
//
// A zero pointers[T] is empty and ready to use.
type pointers[T any] struct {
	// Invariants:
	// 1. cap(table[0]) == 1<<pointersMinLenShift.
	// 2. cap(table[n]) == 2*cap(table[n-1]).
	// 3. cap(table[n]) == len(table[n]) for n < len(table)-1.
	//
	// These invariants are needed for lookup to be O(1).

	table [][]T
}

// Validate that Slice is implemented correctly.
var _ Slice[*int] = (*pointers[int])(nil)

// Len implements [Slice] for pointers.
func (p *pointers[T]) Len() int {
	if len(p.table) == 0 {
		return 0
	}

	// Only the last slice will be not-fully-filled.
	return p.lenOfFirstNSlices(len(p.table)-1) + len(p.table[len(p.table)-1])
}

// At implements [Slice] for pointers.
func (p *pointers[T]) At(n int) *T {
	slice, idx := p.coordinates(n)
	return &p.table[slice][idx]
}

// Iter implements [Slice] for pointers.
func (p *pointers[T]) Iter(yield func(int, *T) bool) {
	var idx int
	for _, slice := range p.table {
		for i := 0; i < len(slice); i++ {
			if !yield(idx, &slice[i]) {
				return
			}
			idx++
		}
	}
}

// Append grows the slice by appending the given elements.
func (p *pointers[T]) Append(values ...T) {
	if len(values) == 0 {
		return
	}

	if p.table == nil {
		p.table = [][]T{make([]T, 0, pointersMinLen)}
	}

	for _, value := range values {
		last := &p.table[len(p.table)-1]
		if len(*last) == cap(*last) {
			// If the last slice is full, grow by doubling the size
			// of the next slice.
			p.table = append(p.table, make([]T, 0, 2*cap(*last)))
			last = &p.table[len(p.table)-1]
		}

		*last = append(*last, value)
	}
}

// String implements [strings.Stringer] for pointers.
func (p pointers[T]) String() string {
	var b strings.Builder
	b.WriteRune('[')
	// Don't use p.Iter, we want to subtly show off the boundaries of the
	// subarrays.
	for i, slice := range p.table {
		if i != 0 {
			b.WriteRune('|')
		}
		for i, v := range slice {
			if i != 0 {
				b.WriteRune(' ')
			}
			fmt.Fprint(&b, v)
		}
	}
	b.WriteRune(']')
	return b.String()
}

// lenOfNthSlice returns the length of the nth slice, even if it isn't
// allocated yet.
func (*pointers[T]) lenOfNthSlice(n int) int {
	return pointersMinLen << n
}

// lenOfNthSlice returns the length of the first n slices.
func (p *pointers[T]) lenOfFirstNSlices(n int) int {
	// Note the following identity:
	//
	// 2^m + 2^(m+1) + ... + 2^n = 2^(n+1) - 2^m
	//
	// This tells us that the sum of p.lenOfNthSlice(m) from 0 to n-1 (the first
	// n slices) is
	return max(0, p.lenOfNthSlice(n)-p.lenOfNthSlice(0))
}

// coordinates calculates the coordinates of the given index in table. It
// also performs a bounds check.
func (p *pointers[T]) coordinates(idx int) (int, int) {
	if idx >= p.Len() || idx < 0 {
		panic(fmt.Sprintf("protocompile/ast: index out of range [%d] with length %d", idx, p.Len()))
	}

	// Given pointersMinLenShift == n, the cumulative starting index of each slice is
	//
	// 0b0 << n, 0b1 << n, 0b11 << n, 0b111 << n
	//
	// Thus, to find which slice an index corresponds to, we add 0b1 << n (pointersMinLen).
	// Because << distributes over addition, we get
	//
	// 0b1 << n, 0b10 << n, 0b100 << n, 0b1000 << n
	//
	// Taking the one-indexed high order bit, which maps this sequence to
	//
	// 1+n, 2+n, 3+n, 4+n
	//
	// We can subtract off n+1 to obtain the actual slice index:
	//
	// 0, 1, 2, 3

	slice := bits.UintSize - bits.LeadingZeros(uint(idx)+pointersMinLen)
	slice -= pointersMinLenShift + 1

	// Then, the offset within table[slice] is given by subtracting off the
	// length of all prior slices from idx.
	idx -= p.lenOfFirstNSlices(slice)

	return slice, idx
}
