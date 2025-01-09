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
	"fmt"
	"unsafe"
)

// Small is a slice that's only two words wide.
//
// Instead of storing both length and capacity, this type only stores a pointer
// and length. This representation is ideal for slices that will not be
// appended to.
type Small[T any] struct {
	_ [0]chan int // Make the type incomparable.

	ptr unsafe.Pointer
	len int
}

// NewSmall wraps a slice as a small slice. This function forgets the capacity,
// so it is equivalent to call NewSmall(s[:len(s):len(s)]).
//
// Beware: because the capacity is discarded the following code will result in
// quadratic runtime.
//
//	var x Small[int]
//	for _, y := range s {
//		// Allocates a fresh backing array each iteration.
//		x = NewSmall(append(x.Slice(), y))
//	}
//
// Instead, it is better to convert back into a Go slice, perform modifications
// in a batch, and then convert back into a Small[T].
func NewSmall[T any](s []T) Small[T] {
	return Small[T]{
		ptr: unsafe.Pointer(unsafe.SliceData(s)),
		len: len(s),
	}
}

// Slice returns a Go slice view into this small slice.
func (s Small[T]) Slice() []T {
	return unsafe.Slice((*T)(s.ptr), s.len)
}

// Format implements fmt.Formatter.
func (s Small[T]) Format(state fmt.State, verb rune) {
	fmt.Fprintf(state, fmt.FormatString(state, verb), s.Slice())
}
