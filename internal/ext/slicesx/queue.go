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

package slicesx

import (
	"fmt"
	"iter"
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Set to true to enable debug printing for queues.
const debugQueue = false

// Queue is a ring buffer.
//
// Values can be pushed and popped from either the front or the back of the
// buffer, making it usable as a double-ended queue.
//
// A zero [Queue] is empty and ready to use.
type Queue[E any] struct {
	buf        []E // Invariant: cap(buf) is always a power of 2, or zero.
	start, end int
}

// NewQueue returns a [Queue] with the given capacity.
func NewQueue[E any](capacity int) *Queue[E] {
	if capacity == 0 {
		return &Queue[E]{}
	}

	// Buffer length must be capacity + 1 (one slot kept empty) and power of 2.
	bufLen := int(bitsx.MakePowerOfTwo(uint(capacity + 1)))
	return &Queue[E]{buf: make([]E, bufLen)}
}

// Len returns the number of elements currently in the buffer.
func (r *Queue[E]) Len() int {
	// The in-use part wraps around the end of the buffer.
	//
	// |xxx------xxxx|     len: 13
	//     ^end  ^start    start: 9
	//                     end: 3
	//
	// Len() = len - start + end = 13 - 9 + 3 = 7
	if r.start > r.end {
		return len(r.buf) - r.start + r.end
	}

	// The in-use part doesn't wrap around.
	//
	// |---xxxxxxx---|     len: 13
	//     ^start ^end     start: 3
	//                     end: 10
	//
	// Len() = end - start = 10 - 3 = 7
	return r.end - r.start
}

// Cap returns the capacity of the buffer, i.e., the number of elements it can
// hold before being resized.
func (r *Queue[E]) Cap() int {
	if len(r.buf) == 0 {
		return 0
	}
	return len(r.buf) - 1
}

// Reserve ensures that the capacity is large enough to push an additional n
// elements.
func (r *Queue[E]) Reserve(n int) {
	if r.Len()+n <= r.Cap() {
		return
	}
	// Buffer length must be capacity + 1 (one slot kept empty) and power of 2.
	newCap := r.Len() + n
	r.resize(int(bitsx.MakePowerOfTwo(uint(newCap + 1))))
}

// Front returns a pointer to the element at the front of the queue.
func (r *Queue[E]) Front() *E {
	if r.start == r.end {
		return nil
	}
	return &r.buf[r.start]
}

// Back returns a pointer to the element at the back of the queue.
func (r *Queue[E]) Back() *E {
	if r.start == r.end {
		return nil
	}
	return &r.buf[(r.end-1)&(len(r.buf)-1)]
}

// PushFront pushes elements to the front of the queue.
func (r *Queue[E]) PushFront(v ...E) {
	r.Reserve(len(v))

	end := r.start
	start := end - len(v)
	r.start = start & (len(r.buf) - 1)
	if start < r.start {
		// We overflowed, so we need to do two copies.
		count := copy(r.buf[r.start:], v)
		copy(r.buf, v[count:])
	} else {
		copy(r.buf[r.start:end], v)
	}
}

// PushBack pushes elements to the back of the queue.
func (r *Queue[E]) PushBack(v ...E) {
	r.Reserve(len(v))

	start := r.end
	end := start + len(v)
	r.end = end & (len(r.buf) - 1)
	if r.end < end {
		// We overflowed, so we need to do two copies.
		count := copy(r.buf[start:], v)
		copy(r.buf, v[count:])
	} else {
		copy(r.buf[start:r.end], v)
	}
}

// PopFront pops the element at the front of the queue.
func (r *Queue[E]) PopFront() (E, bool) {
	if r.start == r.end {
		var z E
		return z, false
	}
	v, _ := Take(r.buf, r.start)
	r.start++
	r.start &= len(r.buf) - 1
	return v, true
}

// PopBack pops the element at the back of the queue.
func (r *Queue[E]) PopBack() (E, bool) {
	if r.start == r.end {
		var z E
		return z, false
	}
	r.end--
	r.end &= len(r.buf) - 1
	return Take(r.buf, r.end)
}

// Values returns an iterator over the elements of the queue.
func (r *Queue[E]) Values() iter.Seq[E] {
	return func(yield func(E) bool) {
		if r.start <= r.end {
			for _, v := range r.buf[r.start:r.end] {
				if !yield(v) {
					return
				}
			}
			return
		}
		for _, v := range r.buf[r.start:] {
			if !yield(v) {
				return
			}
		}
		for _, v := range r.buf[:r.end] {
			if !yield(v) {
				return
			}
		}
	}
}

// Format implements [fmt.Formatter].
func (r Queue[E]) Format(out fmt.State, verb rune) {
	if out.Flag('#') {
		fmt.Fprintf(out, "%T{", r)
	} else {
		fmt.Fprint(out, "[")
	}

	if debugQueue {
		for i, v := range slices.All(r.buf) {
			if i > 0 {
				if out.Flag('#') {
					fmt.Fprint(out, ", ")
				} else {
					fmt.Fprint(out, " ")
				}
			}
			if i == r.start {
				fmt.Fprint(out, ">")
				if i == r.end {
					fmt.Fprint(out, "< ")
				}
			}
			fmt.Fprintf(out, fmt.FormatString(out, verb), v)
			if r.start != r.end && i == r.end-1 {
				fmt.Fprint(out, "<")
			}
		}
	} else {
		for i, v := range iterx.Enumerate(r.Values()) {
			if i > 0 {
				if out.Flag('#') {
					fmt.Fprint(out, ", ")
				} else {
					fmt.Fprint(out, " ")
				}
			}
			fmt.Fprintf(out, fmt.FormatString(out, verb), v)
		}
	}

	if out.Flag('#') {
		fmt.Fprint(out, "}", r)
	} else {
		fmt.Fprint(out, "]")
	}
}

// Clear clears the queue.
func (r *Queue[_]) Clear() {
	clear(r.buf)
	r.start, r.end = 0, 0
	r.buf = r.buf[:0]
}

func (r *Queue[E]) resize(n int) {
	var count int
	old := r.buf
	r.buf = make([]E, n)
	if r.start > r.end {
		count = copy(r.buf, old[r.start:])
		count += copy(r.buf[count:], old[:r.end])
	} else {
		count = copy(r.buf, old[r.start:r.end])
	}
	r.start = 0
	r.end = count
}
