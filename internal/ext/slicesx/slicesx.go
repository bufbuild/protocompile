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

// Get performs a bounds check and returns the value at idx.
//
// If the bounds check fails, returns the zero value and false.
func Get[S ~[]E, E any, I SliceIndex](s S, idx I) (element E, ok bool) {
	if idx < 0 {
		return element, false
	}
	if uint64(idx) >= uint64(len(s)) {
		return element, false
	}

	// Dodge the bounds check, since Go probably won't be able to
	// eliminate it even after stenciling.
	return *unsafex.Add(unsafe.SliceData(s), idx), true
}

// GetPointer is like [Get], but it returns a pointer to the selected element
// instead, returning nil on out-of-bounds indices.
func GetPointer[S ~[]E, E any, I SliceIndex](s S, idx I) *E {
	if idx < 0 {
		return nil
	}
	if uint64(idx) >= uint64(len(s)) {
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

// Among is like [slices.Contains], but the haystack is passed variadically.
//
// This makes the common case of using Contains as a variadic (x == y || ...)
// more compact.
func Among[E comparable](needle E, haystack ...E) bool {
	return slices.Contains(haystack, needle)
}
