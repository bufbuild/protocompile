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
	"runtime"
	"sync/atomic"
	"unsafe"
)

// Log is an append-only log.
//
// Loading and append operations may happen concurrently with each other,
// with the caveat that loading indices not yet appended may produce garbage
// values.
type Log[T any] struct {
	// Protected by the lock bit in cap. This means that cap must be loaded
	// before loading this value, to ensure that prior writes to it are seen,
	// and cap must be stored after storing this value, to ensure the write is
	// published.
	ptr *T

	next atomic.Int32 // The next index to fill.
	cap  atomic.Int32 // Top bit is used as a spinlock.
}

const lockbit = math.MinInt32

// Load returns the value at the given index. This index must have been
// previously returned by [Log.Append].
func (s *Log[T]) Load(idx int) T {
	// Read cap first, which is required before we can read s.ptr.
	cap := s.cap.Load()

	return unsafe.Slice(s.ptr, cap&^lockbit)[idx]
}

// Append adds a new value to this slice.
//
// Returns the index of the appended element, which can be looked up with
// [Log.Load].
func (s *Log[T]) Append(v T) int {
	i := s.next.Add(1)
	if i == 0 {
		panic(errors.New("internal/atomicx: cannot allocate more than 2^32 elements"))
	}
	i--

again:
	// Load cap first. See comment in [Load].
	c := s.cap.Load()
	if c&lockbit != 0 {
		runtime.Gosched()
		goto again
	}

	slice := unsafe.Slice(s.ptr, c)
	if uint32(i) < uint32(c) { // Don't need to grow the slice.
		// This is a data race. However, it's fine, because this slot is not
		// valid yet, so tearing it is fine.
		//
		// This is, in fact, a benign race. So long as this value is not read
		// at before this function returns i, no memory corruption is possible.
		// In particular, Go promises to never tear pointers, so we can't make
		// the GC freak out about broken pointers.
		//
		// See https://go.dev/ref/mem#restrictions
		//
		// This store is also the slowest part of this function, due to
		// significant cache thrashing if the slice is resized from under us.
		storeNoRace(&slice[i], v)

		if s.cap.Load() != c {
			// If the value was potentially torn, it would have resulted in c
			// changing, meaning we need to try again.
			runtime.Gosched()
			goto again
		}

		return int(i)
	}

	// Need to grow a slice. Lock the slice by setting the sign bit of the
	// capacity.
	//
	// This lock is necessary so that updating ptr always happens together with
	// cap.
	if !s.cap.CompareAndSwap(c, c|lockbit) {
		goto again
	}

	// Grow the slice enough to insert our value.
	// Getting preempted by the call into the allocator would be... non-ideal,
	// but there isn't really a way to prevent that.
	//
	// To try to tame down the number of times we need to grow the slice, since
	// that cause significant cache thrash due to racing reads and writes, we
	// grow the underlying buffer a little faster than O(2^n).
	slice = append(slice, make([]T, i+1-c)...)
	slice[i] = v

	// Drop the lock on the slice.
	s.ptr = unsafe.SliceData(slice)
	s.cap.Store(int32(cap(slice)))

	return int(i)
}

//go:norace
func storeNoRace[T any](p *T, v T) {
	*p = v
}
