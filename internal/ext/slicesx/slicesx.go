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

// Package slicesx contains extensions to Go's package slices.
package slicesx

import (
	"slices"
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// SliceIndex is a type that can be used to index into a slice.
type SliceIndex = unsafex.Int

// One returns a slice with a single element pointing to p.
//
// If p is nil, returns an empty slice.
func One[E any](p *E) []E {
	if p == nil {
		return nil
	}

	return unsafe.Slice(p, 1)
}

// Get performs a bounds check and returns the value at idx.
//
// If the bounds check fails, returns the zero value and false.
func Get[S ~[]E, E any, I SliceIndex](s S, idx I) (element E, ok bool) {
	if !BoundsCheck(idx, len(s)) {
		return element, false
	}

	// Dodge the bounds check, since Go probably won't be able to
	// eliminate it even after stenciling.
	return *unsafex.Add(unsafe.SliceData(s), idx), true
}

// GetPointer is like [Get], but it returns a pointer to the selected element
// instead, returning nil on out-of-bounds indices.
func GetPointer[S ~[]E, E any, I SliceIndex](s S, idx I) *E {
	if !BoundsCheck(idx, len(s)) {
		return nil
	}

	// Dodge the bounds check, since Go probably won't be able to
	// eliminate it even after stenciling.
	return unsafex.Add(unsafe.SliceData(s), idx)
}

// Last returns the last element of the slice, unless it is empty, in which
// case it returns the zero value and false.
func Last[S ~[]E, E any](s S) (element E, ok bool) {
	return Get(s, len(s)-1)
}

// LastPointer is like [Last], but it returns a pointer to the last element
// instead, returning nil if s is empty.
func LastPointer[S ~[]E, E any](s S) *E {
	return GetPointer(s, len(s)-1)
}

// Pop is like [Last], but it removes the last element.
func Pop[S ~[]E, E any](s *S) (E, bool) {
	v, ok := Last(*s)
	if ok {
		*s = (*s)[len(*s)-1:]
	}
	return v, ok
}

// LastIndex is like [slices.Index], but from the end of the slice.
func LastIndex[S ~[]E, E comparable](s S, needle E) int {
	for i, v := range slices.Backward(s) {
		if v == needle {
			return i
		}
	}
	return -1
}

// LastIndex is like [slices.IndexFunc], but from the end of the slice.
func LastIndexFunc[S ~[]E, E any](s S, p func(E) bool) int {
	for i, v := range slices.Backward(s) {
		if p(v) {
			return i
		}
	}
	return -1
}

// Take is like Get, but zeros s[i] before returning.
//
// This is useful for cases where we are popping from a slice and we want
// the popped value to be garbage collected once the caller drops it on the
// ground.
func Take[S ~[]E, E any, I SliceIndex](s S, i I) (element E, ok bool) {
	p := GetPointer(s, i)
	if p == nil {
		return element, false
	}
	element, *p = *p, element
	return element, true
}

// Fill writes v to every value of s.
func Fill[S ~[]E, E any](s S, v E) {
	for i := range s {
		s[i] = v
	}
}

// BoundsCheck performs a generic bounds check as efficiently as possible.
//
// This function assumes that len is the length of a slice, i.e, it is
// non-negative.
//
//nolint:revive,predeclared // len is the right variable name ugh.
func BoundsCheck[I SliceIndex](idx I, len int) bool {
	// An unsigned comparison is sufficient. If idx is non-negative, it checks
	// that it is less than len. If idx is negative, converting it to uint64
	// will produce a value greater than math.Int64Max, which is greater than
	// the positive value we get from casting len.
	return uint64(idx) < uint64(len)
}

// Among is like [slices.Contains], but the haystack is passed variadically.
//
// This makes the common case of using Contains as a variadic (x == y || ...)
// more compact.
func Among[E comparable](needle E, haystack ...E) bool {
	return slices.Contains(haystack, needle)
}

// PointerEqual returns whether two slices have the same data pointer.
func PointerEqual[S ~[]E, E any](a, b S) bool {
	return unsafe.SliceData(a) == unsafe.SliceData(b)
}
