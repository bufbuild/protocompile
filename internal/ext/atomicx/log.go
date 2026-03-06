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
	"math"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Log is a highly concurrent, append-only log.
// The zero value is valid and ready to use.
type Log[T any] struct {
	ptr atomic.Pointer[T]
	len atomic.Int32
	cap int32
	mu  sync.Mutex
}

// Load returns the value at the given index.
//
// This function may be called concurrently with Append and other Load calls.
func (s *Log[T]) Load(idx int) T {
	// Read len first. This ensures ordering such that after we load ptr, we
	// don't load a len value incremented by a different call to Append that
	// triggered a reallocation.
	len := s.len.Load()
	if uint(idx) >= uint(len) {
		panic("runtime error: index out of range")
	}

	ptr := s.ptr.Load()
	var elem T
	offset := uintptr(idx) * unsafe.Sizeof(elem)
	return *(*T)(unsafe.Add(unsafe.Pointer(ptr), offset))
}

// Append adds a new value to this log and returns the index.
//
// This function may be called concurrently with Load and other Append calls.
func (s *Log[T]) Append(v T) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.ptr.Load()
	l := s.len.Load()
	c := s.cap

	// Fast path, don't need to grow the slice.
	if uint32(l) < uint32(c) {
		slice := unsafe.Slice(p, c)
		slice[l] = v

		s.len.Store(l + 1)
		return int(l)
	}

	if l == math.MaxInt32 {
		panic("internal/atomicx: cannot allocate more than 2^32 elements")
	}

	// Grow a new slice.
	slice := append(unsafe.Slice(p, c), v)

	// Update the pointer, length, and capacity as appropriate.
	// Note that we update the length *after* the pointer, so an interleaved
	// call to Load will not see a longer length with an old pointer.
	s.ptr.Store(unsafe.SliceData(slice))
	s.len.Store(int32(len(slice)))
	s.cap = int32(cap(slice))

	return int(l)
}
