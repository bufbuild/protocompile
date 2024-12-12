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

// package unsafex contains extensions to Go's package unsafe.
//
// Importing this package should be treated as equivalent to importing unsafe.
package unsafex

import "unsafe"

// Size is like [unsafe.Sizeof], but it is a generic function and it returns
// an int instead of a uintptr (Go does not have types so large they would
// overflow an int).
func Size[T any]() int {
	var zero T
	return int(unsafe.Sizeof(zero))
}

// Index is like [unsafe.Add], but it operates on a typed pointer and scales the
// offset by that type's size, similar to pointer arithmetic in Rust or C.
//
// This function has the same safety caveats as [unsafe.Add].
func Index[T any](p *T, idx int) *T {
	raw := unsafe.Pointer(p)
	raw = unsafe.Add(raw, idx*Size[T]())
	return (*T)(raw)
}

// Cast performs an unchecked cast of one pointer type to another, through
// an [unsafe.Pointer].
func Cast[To, From any](p *From) *To {
	return (*To)(unsafe.Pointer(p))
}

// Transmute bit-casts a value of type From to a value of type To.
//
// This operation is very dangerous, because it can be used to break package
// export barriers, read uninitialized memory, and forge pointers in violation
// of [unsafe.Pointer]'s contract, resulting in memory errors in the GC.
//
// Transmute should only be used on types the caller owns, and those types
// should have identical GC shapes.
func Transmute[To, From any](p From) To {
	return *Cast[To](&p)
}

// StringAlias returns a string that aliases a slice. This is useful for
// situations where we're allocating a string on the stack, or where we have
// a slice that will never be written to and we want to interpret as a string
// without a copy.
//
// data must not be written to: for the lifetime of the returned string (that
// is, until its final use in the program upon which a finalizer set on it could
// run), it must be treated as if goroutines are concurrently reading from it:
// data must not be mutated in any way.
func StringAlias[S ~[]E, E any](data S) string {
	return unsafe.String(Cast[byte](unsafe.SliceData(data)), len(data)*Size[E]())
}
