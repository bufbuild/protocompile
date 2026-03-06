package atomicx

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"
)

const debugLog = false

var capacities sync.Map

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
	// Order doesn't matter here. Len is always updated after ptr, so ptr will
	// always be valid for len elements.
	len := s.len.Load()
	ptr := s.ptr.Load()

	if debugLog {
		v, _ := capacities.Load(unsafe.Pointer(ptr))
		cap := v.(int)
		if cap < int(len) {
			panic(fmt.Errorf("atomicx.Log: loaded %p with cap=%d < len=%d", ptr, cap, len))
		}
	}

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

	if debugLog {
		capacities.Store(unsafe.Pointer(unsafe.SliceData(slice)), cap(slice))
	}

	// Update the pointer, length, and capacity as appropriate.
	// Note that we update the length *after* the pointer, so an interleaved
	// call to Load will not see a longer length with an old pointer.
	s.ptr.Store(unsafe.SliceData(slice))
	s.len.Store(int32(len(slice)))
	s.cap = int32(cap(slice))

	return len(slice)
}
