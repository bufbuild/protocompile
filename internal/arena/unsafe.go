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

// package arena defines an [Arena] type with compressed pointers.
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
	"runtime"
	"unsafe"
)

// pointerIndex returns an integer n such that p == &s[n], or -1 if there is
// no such integer.
func pointerIndex[T any](p *T, s []T) int {
	a := unsafe.Pointer(p)
	b := unsafe.Pointer(unsafe.SliceData(s))
	// KeepAlive escapes its argument, so this ensures that a and b have
	// escaped to the heap and won't be moved.
	runtime.KeepAlive([2]unsafe.Pointer{a, b})

	diff := uintptr(a) - uintptr(b)
	size := unsafe.Sizeof(*p)
	byteLen := uintptr(len(s)) * size

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
	//    regardless of the value of diff.
	//
	// 4. That p is not nil. If it is nil, then either diff will be huge
	//    (because s is a nonempty slice) or byteLen will be zero in which case
	//    (3) applies.
	//
	// Doing this as one branch is much faster than checking all four
	// separately; this is a fairly involved strength reduction that not even
	// LLVM can figure out in many cases, nor can Go tip as of 2024-10-28.
	if diff >= byteLen {
		return -1
	}

	return int(diff / size)
}
