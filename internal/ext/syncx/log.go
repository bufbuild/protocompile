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
package syncx

import (
	"errors"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// Log is an append-only log.
//
// Loading and append operations may happen concurrently with each other.
// Can hold at most 2^31 elements.
type Log[T any] struct {
	ptr atomic.Pointer[T]

	next, len atomic.Int32 // The next index to fill.
	cap       atomic.Int32 // Top bit is used as a spinlock.
}

// Load returns the value at the given index.
//
// Panics if no value is at that index.
func (s *Log[T]) Load(idx int) T {
	// Read cap first, which is required before we can read s.ptr.
	len := s.len.Load()
	ptr := s.ptr.Load()

	return unsafe.Slice(ptr, len)[idx]
}

// Append adds a new value to this slice.
//
// Returns the index of the appended element, which can be looked up with
// [Log.Load].
func (s *Log[T]) Append(v T) int {
	i := s.next.Add(1)
	if i < 0 {
		panic(errors.New("internal/syncx: cannot allocate more than 2^31 elements"))
	}
	i--

	// Wait for the capacity to be large enough for our index, or for us to
	// be responsible for growing it (i == c).
	c := s.cap.Load()
	for i > c {
		runtime.Gosched()
		c = s.cap.Load()
	}

	// Fast path (i < c): slice is already large enough.
	if i < c {
		p := s.ptr.Load()
		unsafe.Slice(p, c)[i] = v

		s.len.Add(1)
		for s.len.Load() <= i {
			// Make sure that every index before us also completes, to ensure
			// that Load does not panic.
			runtime.Gosched()
		}

		return int(i)
	}

	// Slow path (i == c): we are responsible for growing the slice. Need to
	// wait until all fast-path writers to finish. Any further writers will
	// spin in the i > c loop.
	for s.len.Load() != c {
		runtime.Gosched()
	}

	// Grow the slice.
	// i == c, so we are appending exactly one element right now.
	p := s.ptr.Load()
	slice := append(unsafe.Slice(p, c), v)

	// Publish the new slice to readers and waiting writers.
	// Pointer must be stored before capacity to prevent out-of-bounds panics in Load.
	s.ptr.Store(unsafe.SliceData(slice))
	s.cap.Store(int32(cap(slice)))

	s.len.Add(1)
	return int(i)
}
