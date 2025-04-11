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
	"cmp"
	"slices"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// IndexFunc is like [slices.IndexFunc], but also takes the index of the element
// being examined as an input to the predicate.
func IndexFunc[S ~[]E, E any](s S, p func(int, E) bool) int {
	return iterx.Index2(slices.All(s), p)
}

// BinarySearchKey is like [slices.BinarySearch], but each element is mapped
// to a comparable type.
func BinarySearchKey[S ~[]E, E any, T cmp.Ordered](s S, target T, key func(E) T) (int, bool) {
	return slices.BinarySearchFunc(s, target, func(e E, t T) int { return cmp.Compare(key(e), t) })
}

// PointerIndex returns an integer n such that p == &s[n], or -1 if there is
// no such integer.
//
//go:nosplit
func PointerIndex[S ~[]E, E any](s S, p *E) int {
	a := unsafe.Pointer(p)
	b := unsafe.Pointer(unsafe.SliceData(s))

	diff := uintptr(a) - uintptr(b)
	size := unsafex.Size[E]()
	byteLen := len(s) * size

	// This comparison checks for the following things:
	//
	// 1. Obviously, that diff is not past the end of s.
	//
	// 2. That the subtraction did not overflow. If it did, diff will be
	//    negative two's complement, i.e. the MSB is set, so it will be
	//	  greater than byteLen, which, due to allocation limitations on
	//    every platform ever, cannot be greater than MaxInt, which all
	//	  "negative" uintptrs are greater than.
	//
	// 3. That byteLen is not zero. If it is zero, this branch is taken
	//    regardless of the value of diff
	//
	// 4. That p is not nil. If it is nil, then either diff will be huge
	//    (because s is a nonempty slice) or byteLen will be zero in which case
	//    (3) applies.
	//
	// Doing this as one branch is much faster than checking all four
	// separately; this is a fairly involved strength reduction that not even
	// LLVM can figure out in many cases, nor can Go tip as of 2024-10-28.
	if diff >= uintptr(byteLen) {
		return -1
	}

	// NOTE: A check for diff % size is not necessary. This would only be needed
	// if the user passed in a pointer that points into the slice, but which
	// does not point to the start of one of the slice's elements. However,
	// because the pointer and slice must have the same type, this would mean
	// that such a pointee straddles two elements of the slice, which Go does
	// not permit (such pointers can only be created by abusing the unsafe
	// package).
	return int(diff) / size
}
