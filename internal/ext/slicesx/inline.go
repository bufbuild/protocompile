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

package slicesx

import (
	"slices"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// Inline is a slice of scalar values, which does not allocate a separate
// underlying array for small slices. It *cannot* hold pointers.
//
// The zero value is empty and ready to use.
type Inline[T unsafex.Int] struct {
	_ [0]chan int // Make the type incomparable.

	// This is either nil, a pointer into inlineSentinels, or a pointer to
	// an [N]uint32.
	data unsafe.Pointer

	// These fields are only a length and capacity when data is not one of the
	// five sentinel values (i.e., when IsInlined is true). In that case, these
	// fields are reinterpreted as a [N]T. This is safe, because neither
	// [2]uint64 and [N]T are both pointer-free types.
	inline struct {
		len, cap uint64
	}
}

// NewInline wraps a slice in an Inlined32.
//
// Wrapping nil returns the zero.
func NewInline[T unsafex.Int](s []T) Inline[T] {
	return Inline[T]{
		data: unsafe.Pointer(unsafe.SliceData(s)),
		inline: struct{ len, cap uint64 }{
			len: uint64(len(s)),
			cap: uint64(cap(s)),
		},
	}
}

// Len returns the length of the slice.
func (s *Inline[T]) Len() int {
	return len(s.unsafeSlice())
}

// Cap returns the capacity of the slice.
//
// Inlined slices always have a capacity equal to 16 / Sizeof(T).
func (s *Inline[T]) Cap() int {
	return cap(s.unsafeSlice())
}

// At returns the value at index n.
//
// Panics if the index is out of range.
func (s *Inline[T]) At(n int) T {
	return s.unsafeSlice()[n]
}

// SetAt sets the value at index n.
//
// Panics if the index is out of range.
func (s *Inline[T]) SetAt(n int, v T) {
	s.unsafeSlice()[n] = v
}

// Insert inserts a value at index n.
//
// Panics if the index is out of range.
func (s *Inline[T]) Insert(n int, v T) {
	s.set(slices.Insert(s.unsafeSlice(), n, v))
}

// Delete deletes the value at index n.
//
// Panics if the index is out of range.
func (s *Inline[T]) Delete(n int) {
	s.set(slices.Delete(s.unsafeSlice(), n, n+1))
}

// Slice returns the underlying slice.
//
// If the slice inlined, this will return a copy, which cannot be mutated
// through.
func (s *Inline[T]) Slice() []T {
	if !s.IsInlined() {
		// We can safely return this slice, because it is not inlined.
		return s.unsafeSlice()
	}

	// Make a copy of the slice. This makes sure that the user cannot mutate
	// *s though the slice we return, and copied will be thrown away after the
	// function returns.
	copied := *s
	return copied.unsafeSlice()
}

// IsInlined returns whether this slice is currently in inlined mode.
func (s *Inline[T]) IsInlined() bool {
	_, ok := inlineLen(s.data)
	return ok
}

// Compact converts this into an inlined slice if possible.
func (s *Inline[T]) Compact() {
	if s.IsInlined() {
		// Nothing to do.
		return
	}

	if int(s.inline.len) > s.maxInlineLen() {
		// Compacting it won't fit.
		return
	}

	old := s.unsafeSlice()
	if s.inline.len == 0 {
		*s = NewInline[T](nil)
	} else {
		s.data = inlineSentinel(len(old))
		copy(s.unsafeSlice(), old)
	}
}

// Format implements fmt.Formatter.
// func (s *Inline[T]) Format(state fmt.State, verb rune) {
// 	fmt.Fprintf(state, fmt.FormatString(state, verb), s.unsafeSlice())
// }

// maxInlineLen returns the maximum number of inline elements.
func (*Inline[T]) maxInlineLen() int {
	return len(inlineSentinels) / unsafex.Size[T]()
}

// set sets the value of this slice to slice.
//
// This function has a special case for when slice is an alias of this slice's
// inline region.
func (s *Inline[T]) set(slice []T) {
	if unsafe.Pointer(unsafe.SliceData(slice)) == unsafe.Pointer(&s.inline) {
		s.data = inlineSentinel(len(slice))
		return
	}

	*s = NewInline(slice)
}

// unsafeSlice returns the slice this Inline represents.
//
// This slice MUST NOT be escaped into public API! If it does, users may write
// this code:
//
//	var s Inline[int32]
//	s.Append(1)
//	s1 := s.unsafeSlice()
//	for range 4 {
//		s.Append(2)
//	}
//
//	s2 := s.unsafeSlice()
//	s1[0] = 10000
//	s2[16] = 42 // Out-of-bounds write.
//
// Upon pushing to the point that the slice spills to the heap, s1 still holds
// a pointer back into the slice, so writing to it will overwrite the length
// and capacity of the slice!
//
// Similarly, users MUST NOT be given access to pointers into this slice.
func (s *Inline[T]) unsafeSlice() []T {
	if idx, ok := inlineLen(s.data); ok {
		return unsafe.Slice(unsafex.Bitcast[*T](&s.inline), s.maxInlineLen())[:idx]
	}

	return unsafe.Slice((*T)(s.data), s.inline.cap)[:s.inline.len]
}

// Need to use unsafe.Sizeof here to make this into a constant.
var inlineSentinels [unsafe.Sizeof(Inline[byte]{}.inline)]byte

func inlineSentinel(n int) unsafe.Pointer {
	if n == 0 {
		return nil
	}
	return unsafe.Pointer(&inlineSentinels[n-1])
}

func inlineLen(p unsafe.Pointer) (int, bool) {
	if p == nil {
		return 0, true
	}
	if idx := PointerIndex(inlineSentinels[:], (*byte)(p)); idx >= 0 {
		return idx + 1, true
	}
	return 0, false
}
