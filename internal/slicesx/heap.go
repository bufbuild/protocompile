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

package slicesx

import "cmp"

// Heap is a port of Go's [container/heap] package to use generics instead of
// interface calls. Instead, entries consist of an ordered key and an arbitrary
// value.ß
//
// A zero heap is empty and ready to use.
type Heap[K cmp.Ordered, V any] struct {
	keys []K
	vals []V
}

// NewHeap returns a new heap with the given pre-allocated capacity.
//
//nolint:revive,predeclared // cap used as a variable.
func NewHeap[K cmp.Ordered, V any](cap int) *Heap[K, V] {
	return &Heap[K, V]{
		keys: make([]K, 0, cap),
		vals: make([]V, 0, cap),
	}
}

// Len returns the number of elements in the heap.
func (h *Heap[K, V]) Len() int {
	return len(h.keys)
}

// Push pushes the element x onto the heap.
func (h *Heap[K, V]) Push(k K, v V) {
	h.push(k, v)
	h.up(h.Len() - 1)
}

// Pop removes and returns the entry with the least key from the heap.
//
// Pop is equivalent to [Heap.Remove](0).
func (h *Heap[K, V]) Pop() (K, V) {
	n := h.Len() - 1
	h.swap(0, n)
	h.down(0, n)
	return h.pop()
}

func (h *Heap[K, V]) up(j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !h.less(j, i) {
			break
		}
		h.swap(i, j)
		j = i
	}
}

func (h *Heap[K, V]) down(i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 after int overflow
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && h.less(j2, j1) {
			j = j2 // = 2*i + 2  // right child
		}
		if !h.less(j, i) {
			break
		}
		h.swap(i, j)
		i = j
	}
	return i > i0
}

func (h *Heap[K, V]) less(i, j int) bool {
	return h.keys[i] < h.keys[j]
}

func (h *Heap[K, V]) swap(i, j int) {
	h.keys[i], h.keys[j] = h.keys[j], h.keys[i]
	h.vals[i], h.vals[j] = h.vals[j], h.vals[i]
}

func (h *Heap[K, V]) push(k K, v V) {
	h.keys = append(h.keys, k)
	h.vals = append(h.vals, v)
}

func (h *Heap[K, V]) pop() (k K, v V) {
	end := h.Len() - 1
	k, h.keys = h.keys[end], h.keys[:end]
	v, h.vals = h.vals[end], h.vals[:end]
	return k, v
}
