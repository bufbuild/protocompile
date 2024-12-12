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

import (
	"fmt"
	"unsafe"
)

// Layout is the layout of a type.
//
// This is a more convenient abstraction that manipulating the size and
// alignment separately.
type Layout struct {
	Size, Align int

	// TODO: Add something like PointerFree? It's not clear if Go even exposes
	// an easy way to compute this. There's certainly no intrinsic for it...
}

// LayoutOf returns the layout of some type.
func LayoutOf[T any]() Layout {
	var v T
	return Layout{
		Size:  int(unsafe.Sizeof(v)),
		Align: int(unsafe.Alignof(v)),
	}
}

// Index is like [unsafe.Add], but it operates on a typed pointer and scales the
// offset by that type's size, similar to pointer arithmetic in Rust or C.
//
// This function has the same safety caveats as [unsafe.Add].
//
//go:nosplit
func Index[T any](p *T, idx int) *T {
	raw := unsafe.Pointer(p)
	raw = unsafe.Add(raw, idx*LayoutOf[T]().Size)
	return (*T)(raw)
}

// Bitcast bit-casts a value of type From to a value of type To.
//
// This operation is very dangerous, because it can be used to break package
// export barriers, read uninitialized memory, and forge pointers in violation
// of [unsafe.Pointer]'s contract, resulting in memory errors in the GC.
//
// Panics if To and From have different sizes.
//
//go:nosplit
func Bitcast[To, From any](v From) To {
	// This function is correctly compiled down to a mov, as seen here:
	// https://godbolt.org/z/qvndcYYba
	//
	// With redundant code removed, stenciling Bitcast[float64, int64] produces
	// (as seen in the above Godbolt):
	//
	//   TEXT    unsafex.Bitcast[float64,int64]
	//   MOVQ    32(R14), R12
	//   TESTQ   R12, R12
	//   JNE     morestack
	//   XCHGL   AX, AX
	//   MOVQ    AX, X0
	//   RET

	if LayoutOf[To]().Size != LayoutOf[From]().Size {
		// This check will always be inlined away, because Bitcast is
		// manifestly inline-able.
		//
		// NOTE: This could potentially be replaced with a link error, by making
		// this call a function with no body (and then not defining that
		// function in a .s file; although, note we do need an empty.s to
		// silence a compiler error in that case).
		panic(badBitcast[To, From]{})
	}

	// To avoid an unaligned load below, we copy From into
	// a struct aligned to To.
	//
	// As seen in the Godbolt above, for cases where the alignment change
	// is redundant, this gets SROA'd out of existence.
	//
	// (SROA = scalar replacement of aggregates)
	aligned := struct {
		_ [0]To
		v From
	}{v: v}

	return *(*To)(unsafe.Pointer(&aligned))
}

type badBitcast[To, From any] struct{}

func (badBitcast[To, From]) Error() string {
	var to To
	var from From
	return fmt.Sprintf(
		"unsafex: %T and %T are of unequal size (%d != %d)",
		to, from,
		LayoutOf[To]().Size, LayoutOf[From]().Size,
	)
}

// StringData is like [unsafe.StringData], but it avoids a bug in Go 1.21 where
// calling the StringData intrinsic (which is *not* a generic function) with
// a generic type with a string constraint would be incorrectly diagnosed.
func StringData[S ~string](data S) *byte {
	return unsafe.StringData(string(data))
}

// SliceData is like [unsafe.SliceData], but it avoids a bug in Go 1.21 where
// calling the SliceData intrinsic (which is *not* a generic function) with
// a generic type with a slice constraint would be incorrectly diagnosed.
func SliceData[S ~[]E, E any](data S) *E {
	return unsafe.SliceData([]E(data))
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
//
//go:nosplit
func StringAlias[S ~[]E, E any](data S) string {
	return unsafe.String(
		Bitcast[*byte](SliceData(data)),
		len(data)*LayoutOf[E]().Size,
	)
}
