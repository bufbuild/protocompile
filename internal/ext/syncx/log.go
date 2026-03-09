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
	"fmt"
	"math"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// Log is an append-only log.
//
// Loading and append operations may happen concurrently with each other.
type Log[T any] struct {
	// Protected by the lock bit in cap. This means that cap must be loaded
	// before loading this value, to ensure that prior writes to it are seen,
	// and cap must be stored after storing this value, to ensure the write is
	// published.
	//
	// This value is atomic because otherwise a data race occurs. Go guarantees
	// this is not actually a data race, because in Go all pointer loads/stores
	// are relaxed atomic, but this is essentially free, because we're already
	// loading cap.
	ptr atomic.Pointer[T]

	// Array of bits with length equal to cap divided by the bit size of uintptr,
	// rounded up.
	bits atomic.Pointer[atomic.Uintptr]

	next atomic.Int32 // The next index to fill.
	cap  atomic.Int32 // Top bit is used as a spinlock.
}

const (
	lockbit  = math.MinInt32
	wordBits = int(unsafe.Sizeof(uintptr(0)) * 8)
)

// Load returns the value at the given index.
//
// Panics if no value is at that index.
func (s *Log[T]) Load(idx int) T {
	// Read cap first, which is required before we can read s.ptr.
	cap := s.cap.Load() &^ lockbit
	ptr := s.ptr.Load()

	bits := unsafe.Slice(s.bits.Load(), logBits(int(cap)))
	if n := idx / wordBits; n >= len(bits) || bits[n].Load()&(1<<(idx%wordBits)) == 0 {
		panic(fmt.Errorf("internal/syncx: index out of bounds [%v]", idx))
	}

	return unsafe.Slice(ptr, cap)[idx]
}

// Append adds a new value to this slice.
//
// Returns the index of the appended element, which can be looked up with
// [Log.Load].
func (s *Log[T]) Append(v T) int {
	i := s.next.Add(1)
	if i < 0 {
		panic(errors.New("internal/syncx: cannot allocate more than 2^32 elements"))
	}
	i--

again:
	// Load cap first. See comment in [Load].
	c := s.cap.Load()
	if c&lockbit != 0 {
		runtime.Gosched()
		goto again
	}

	p := s.ptr.Load()
	b := s.bits.Load()
	slice := unsafe.Slice(p, c)
	bits := unsafe.Slice(b, logBits(int(c)))
	if uint32(i) < uint32(c) { // Don't need to grow the slice.
		i := int(i)

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

		// Mark this index as claimed.
		bits[i/wordBits].Or(1 << (i % wordBits))

		// If the value was potentially torn, it would have resulted in c
		// changing, meaning we need to try again.
		//
		// A very important property is that this value never returns to the
		// same value after a resize begins, preventing an ABA problem. If the
		// capacity does not change across a store, it means that store
		// succeeded
		//
		// To ensure that the value we just wrote above is visible to other
		// goroutines, in particular a goroutine that wants to perform a resize,
		// We need to store to the capacity. The easiest way to achieve both
		// things at once is with the following CAS.
		if !s.cap.CompareAndSwap(c, c) {
			runtime.Gosched()
			goto again
		}

		return i
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
	//
	// Getting preempted by the call into the allocator would be... non-ideal,
	// but there isn't really a way to prevent that.
	slice = append(slice, make([]T, max(i+1, 16)-c)...)
	slice[i] = v

	// Now, grow the bit array.
	bits2 := make([]atomic.Uintptr, logBits(cap(slice)))
	for i := range bits {
		bits2[i].Store(bits[i].Load())
	}
	bits = bits2

	s.ptr.Store(unsafe.SliceData(slice))
	s.bits.Store(unsafe.SliceData(bits))
	s.cap.Store(int32(cap(slice))) // Drop the lock.

	// Mark this index as claimed.
	bits[int(i)/wordBits].Or(1 << (int(i) % wordBits))

	return int(i)
}

//go:norace
func storeNoRace[T any](p *T, v T) {
	*p = v
}

func logBits(cap int) int {
	return (cap + wordBits - 1) / wordBits
}
