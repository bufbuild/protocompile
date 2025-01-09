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

import "cmp"

// Heap is a binary min-heap. This means that it is a complete binary tree that
// respects the heap invariant: each key is less than or equal to the keys of
// its children.
//
// This type resembles Go's [container/heap] package, but it uses generics
// instead of interface calls. Entries consist of a [cmp.Ordered] key, such
// as an integer, and additional data attached to that key.
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

// Insert adds an entry to the heap.
func (h *Heap[K, V]) Insert(k K, v V) {
	h.push(k, v)
	h.up(h.Len() - 1)
}

// Peek returns the entry with the least key, but does not pop it.
func (h *Heap[K, V]) Peek() (K, V) {
	return h.keys[0], h.vals[0]
}

// Pop removes and returns the entry with the least key from the heap.
func (h *Heap[K, V]) Pop() (K, V) {
	h.swap(0, h.Len()-1)
	k, v := h.pop()

	h.down(0)
	return k, v
}

// Update replaces the entry with the least key, as if by calling [Heap.Pop]
// followed by [Heap.Insert].
func (h *Heap[K, V]) Update(k K, v V) {
	h.keys[0] = k
	h.vals[0] = v

	// We know that we always need to be moving this entry down, because it
	// can't move up: it's at the top of the heap.
	h.down(0)
}

// up moves the element at i up the queue until it is greater than
// its parent.
func (h *Heap[K, V]) up(i int) {
	for {
		parent, root := heapParent(i)
		if root || !h.less(i, parent) {
			break
		}

		h.swap(parent, i)
		i = parent
	}
}

// down moves the element at i down the tree until it is greater than or equal
// to its parent.
func (h *Heap[K, V]) down(i int) {
	for {
		left, right, overflow := heapChildren(i)
		if overflow || left >= h.Len() {
			break
		}

		child := left
		if right < h.Len() && h.less(right, left) {
			child = right
		}

		if !h.less(child, i) {
			break
		}

		h.swap(i, child)
		i = child
	}
}

// less returns whether i's key is less than j's key.
func (h *Heap[K, V]) less(i, j int) bool {
	return h.keys[i] < h.keys[j]
}

// swap swaps the entries at i and j.
func (h *Heap[K, V]) swap(i, j int) {
	h.keys[i], h.keys[j] = h.keys[j], h.keys[i]
	h.vals[i], h.vals[j] = h.vals[j], h.vals[i]
}

// push pushes a value onto the backing slices.
func (h *Heap[K, V]) push(k K, v V) {
	h.keys = append(h.keys, k)
	h.vals = append(h.vals, v)
}

// pop removes the final element of the backing slices and returns it.
func (h *Heap[K, V]) pop() (k K, v V) {
	end := h.Len() - 1
	k, h.keys = h.keys[end], h.keys[:end]
	v, h.vals = h.vals[end], h.vals[:end]
	return k, v
}

// heapParent returns the heapParent index of i. Returns false if i is the root.
func heapParent(i int) (parent int, isRoot bool) {
	j := (i - 1) / 2
	return j, i == j
}

// heapChildren returns the child indices of i.
//
// Returns false on overflow.
func heapChildren(i int) (left, right int, overflow bool) {
	j := i*2 + 1
	return j, j + 1, j < 0
}
