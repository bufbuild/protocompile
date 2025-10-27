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

// Package arena defines an [Arena] type with compressed pointers.
//
// The benefits of using compressed pointers are as follows:
//
//  1. Pointers are only four bytes wide and four-byte aligned, saving on space
//     in pointer-heavy graph data structures.
//
//  2. The GC has to do substantially less work on such graph data structures,
//     because from its perspective, structures that only contain compressed
//     pointers are not deeply-nested and require less traversal (remember,
//     the bane of a GC is something that looks like a linked list).
//
//  3. Improved cache locality. All values inside of the same arena are likelier
//     to be near each other.
package arena

import (
	"fmt"
	"iter"
	"math/bits"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// pointersMinLenShift is the log2 of the size of the smallest slice in
// a pointers[T].
const (
	pointersMinLenShift = 4
	pointersMinLen      = 1 << pointersMinLenShift
)

// An untyped arena pointer.
//
// The pointer value of a particular pointer in an arena is equal to one
// plus the number of elements allocated before it.
type Untyped uint32

// Nil returns a nil arena pointer.
func Nil() Untyped {
	return 0
}

// Nil returns whether this pointer is nil.
func (p Untyped) Nil() bool {
	return p == 0
}

// String implements [fmt.Stringer].
func (p Untyped) String() string {
	if p.Nil() {
		return "<nil>"
	}
	return fmt.Sprintf("0x%x", uint32(p))
}

// A compressed arena pointer.
//
// Cannot be dereferenced directly; see [Pointer.In].
//
// The zero value is nil.
type Pointer[T any] Untyped

// Nil returns whether this pointer is nil.
func (p Pointer[T]) Nil() bool {
	return Untyped(p).Nil()
}

// Untyped erases this pointer's type.
//
// This function mostly exists for the aid of tab-completion.
func (p Pointer[T]) Untyped() Untyped {
	return Untyped(p)
}

// String implements [fmt.Stringer].
func (p Pointer[T]) String() string {
	return p.Untyped().String()
}

// Arena is an arena that offers compressed pointers. Conceptually, it is a slice
// of T that guarantees the Ts will never be moved.
//
// It does this by maintaining a table of logarithmically-growing slices that
// mimic the resizing behavior of an ordinary slice. This trades off the linear
// 8-byte overhead of []*T for a logarithmic 24-byte overhead. Lookup time
// remains O(1), at the cost of two pointer loads instead of one.
//
// It also does not discard already-allocated memory, reducing the amount of
// garbage it produces over time compared to a plain []T used as an allocation
// pool.
//
// A zero Arena[T] is empty and ready to use.
type Arena[T any] struct {
	// Invariants:
	// 1. cap(table[0]) == 1<<pointersMinLenShift.
	// 2. cap(table[n]) == 2*cap(table[n-1]).
	// 3. cap(table[n]) == len(table[n]) for n < len(table)-1.
	//
	// These invariants are needed for lookup to be O(1).
	table [][]T
}

// New allocates a new value on the arena.
func (a *Arena[T]) New(value T) *T {
	if a.table == nil {
		a.table = [][]T{make([]T, 0, pointersMinLen)}
	}

	last := &a.table[len(a.table)-1]
	if len(*last) == cap(*last) {
		// If the last slice is full, grow by doubling the size
		// of the next slice.
		a.table = append(a.table, make([]T, 0, 2*cap(*last)))
		last = &a.table[len(a.table)-1]
	}

	*last = append(*last, value)
	return &(*last)[len(*last)-1]
}

// NewCompressed allocates a new value on the arena, returning the result of
// compressing the pointer.
func (a *Arena[T]) NewCompressed(value T) Pointer[T] {
	_ = a.New(value)
	return Pointer[T](a.len()) // Note that len, not len-1, is intentional.
}

// Compress returns a compressed pointer into this arena if ptr belongs to it;
// otherwise, returns nil.
func (a *Arena[T]) Compress(ptr *T) Pointer[T] {
	if ptr == nil {
		return 0
	}

	// Check the slices in reverse order: no matter the state of the arena,
	// the majority of the allocated values will be in either the last or
	// second-to-last slice.
	for i := len(a.table) - 1; i >= 0; i-- {
		idx := slicesx.PointerIndex(a.table[i], ptr)
		if idx != -1 {
			return Pointer[T](a.lenOfFirstNSlices(i) + idx + 1)
		}
	}
	return 0
}

// Deref looks up a pointer in this arena.
//
// This arena must be the one tha allocated this pointer, otherwise this will
// either return an arbitrary pointer or panic.
//
// If p is nil, returns nil.
func (a *Arena[T]) Deref(ptr Pointer[T]) *T {
	if ptr.Nil() {
		return nil
	}

	slice, idx := a.coordinates(int(ptr) - 1)
	return &a.table[slice][idx]
}

// Values returns an iterator that yields each value allocated on this arena.
//
// Values freshly allocated with [Arena.New] may or may not be yielded during
// iteration.
func (a *Arena[T]) Values() iter.Seq[*T] {
	return func(yield func(*T) bool) {
		for _, chunk := range a.table {
			for i := range chunk {
				if !yield(&chunk[i]) {
					return
				}
			}
		}
	}
}

func (a *Arena[T]) len() int {
	if len(a.table) == 0 {
		return 0
	}

	// Only the last slice will be not-fully-filled.
	return a.lenOfFirstNSlices(len(a.table)-1) + len(a.table[len(a.table)-1])
}

// String implements [strings.Stringer].
func (a Arena[T]) String() string {
	var b strings.Builder
	b.WriteRune('[')
	// Don't use p.Iter, we want to subtly show off the boundaries of the
	// subarrays.
	for i, slice := range a.table {
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
func (*Arena[T]) lenOfNthSlice(n int) int {
	return pointersMinLen << n
}

// lenOfFirstNSlices returns the length of the first n slices.
func (a *Arena[T]) lenOfFirstNSlices(n int) int {
	// Note the following identity:
	//
	// 2^m + 2^(m+1) + ... + 2^n = 2^(n+1) - 2^m
	//
	// This tells us that the sum of p.lenOfNthSlice(m) from 0 to n-1 (the first
	// n slices) is
	return max(0, a.lenOfNthSlice(n)-a.lenOfNthSlice(0))
}

// coordinates calculates the coordinates of the given index in table. It
// also performs a bounds check.
func (a *Arena[T]) coordinates(idx int) (int, int) {
	if idx >= a.len() || idx < 0 {
		panic(fmt.Sprintf("arena: pointer out of range: %#x", idx))
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
	idx -= a.lenOfFirstNSlices(slice)

	return slice, idx
}
