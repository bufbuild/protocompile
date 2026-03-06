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

//nolint:revive,predeclared
package atomicx

import (
	"errors"
	"math"
	"sync/atomic"
	"unsafe"
)

// Log is an append-only log. Loading operations may happen concurrently with
// append operations, but append operations may not be concurrent with each
// other.
type Log[T any] struct {
	ptr atomic.Pointer[T]
	len atomic.Int32
	cap int32 // Only modified by Append, does not need synchronization.
}

// Load returns the value at the given index.
//
// This function may be called concurrently with [Log.Append].
func (s *Log[T]) Load(idx int) T {
	// Read len first. This ensures ordering such that after we load ptr, we
	// don't load a len value incremented by a different call to Append that
	// triggered a reallocation.
	len := s.len.Load()
	ptr := s.ptr.Load()

	return unsafe.Slice(ptr, len)[idx]
}

// Append adds a new value to this slice.
//
// Append may be called concurrently with [Log.Load], but must *not* be called
// concurrently with itself. An external mutex should be used to protect calls
// to Append.
//
// Returns the new length of the log.
func (s *Log[T]) Append(v T) int {
	p := s.ptr.Load()
	l := s.len.Load()
	c := s.cap

	if l == math.MaxInt32 {
		panic(errors.New("internal/atomicx: cannot allocate more than 2^32 elements"))
	}

	slice := unsafe.Slice(p, c)

	if l < c { // Don't need to grow the slice.
		// Write the value first, and *then* make it visible to Load by
		// incrementing the length.
		slice[l] = v
		return int(s.len.Add(1))
	}

	// Grow a new slice.
	slice = append(slice, v)

	// Update the pointer, length, and capacity as appropriate.
	// Note that we update the length *after* the pointer, so an interleaved
	// call to Load will not see a longer length with an old pointer.
	s.ptr.Store(unsafe.SliceData(slice))
	s.len.Store(int32(len(slice)))
	s.cap = int32(cap(slice))

	return len(slice)
}
