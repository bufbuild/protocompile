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

package interval

import (
	"fmt"
	"iter"
	"slices"

	"github.com/tidwall/btree"
	"golang.org/x/exp/constraints" //nolint:exptostd // Tries to replace w/ cmp.
)

// Intersection is an interval intersection map: a collection of intervals,
// such that given a point in K, one can query for the intersection of all
// intervals in the collection which contain it, along with the values
// associated with each of those intervals.
//
// A zero value is ready to use.
type Intersect[K Endpoint, V any] struct {
	// Keys in this map are the ends of intervals in the map.
	tree    btree.Map[K, *Entry[K, []V]]
	pending []*Entry[K, []V] // Scratch space for Insert().
}

// Endpoint is a type that may be used as an interval endpoint.
type Endpoint = constraints.Integer

// Entry is an entry in a [Intersect]. This means that it is the intersection
// of all intervals which contain a particular point.
type Entry[K Endpoint, V any] struct {
	Start, End K // The interval range, inclusive.
	Value      V
}

// Contains returns whether an entry contains a given point.
func (e Entry[K, V]) Contains(point K) bool {
	return e.Start <= point && point <= e.End
}

// Get returns the intersection of all intervals which contain point.
//
// If no such interval exists, the [Entry].Values will be nil.
func (m *Intersect[K, V]) Get(point K) Entry[K, []V] {
	iter := m.tree.Iter()
	found := iter.Seek(point)

	if !found || point < iter.Value().Start {
		// Check that the interval actually contains key. It is implicit
		// already that key <= end.
		return Entry[K, []V]{}
	}

	return *iter.Value()
}

// Entries returns an iterator over the entries in this map.
//
// There exists one entry per maximal subset of the map with non-empty
// intersection. Entries are yielded in order, and are pairwise disjoint.
func (m *Intersect[K, V]) Entries() iter.Seq[Entry[K, []V]] {
	return func(yield func(Entry[K, []V]) bool) {
		iter := m.tree.Iter()
		for more := iter.First(); more; more = iter.Next() {
			if !yield(*iter.Value()) {
				return
			}
		}
	}
}

// Insert inserts a new interval into this map, with the given associated value.
// Both endpoints are inclusive.
//
// Returns true if the interval was disjoint from all others in the set.
func (m *Intersect[K, V]) Insert(start, end K, value V) (disjoint bool) {
	if start > end {
		panic(fmt.Sprintf("interval: start (%#v) > end (%#v)", start, end))
	}

	var prev *Entry[K, []V]
	for entry := range m.intersect(start, end) {
		if prev == nil && start < entry.Start {
			// Need to insert an extra entry for the stuff between start and the
			// first interval.
			m.pending = append(m.pending, &Entry[K, []V]{
				Start: start,
				End:   entry.Start - 1,
				Value: []V{value},
			})
		}

		// Make a copy of entry.Values; entry.Values may get modified in a way
		// where appending to it results in value appearing twice.
		//
		// NB: the values array may be shared across different entries where one
		// is a prefix of the other.
		orig := entry.Value

		// If the entry contains end, we need to split it at end.
		if entry.Contains(end) && end < entry.End {
			next := &Entry[K, []V]{
				Start: entry.Start,
				End:   end,
				Value: append(slices.Clip(orig), value),
			}

			// Shorten the existing entry.
			entry.Start = end + 1

			// Add next to the pending queue and use it as the entry here
			// onwards.
			m.pending = append(m.pending, next)
			entry = next
		}

		// If the entry contains start, we also need to split it.
		if entry.Contains(start) && entry.Start < start {
			next := &Entry[K, []V]{
				Start: entry.Start,
				End:   start - 1,
				Value: orig,
			}

			// Add next to the pending queue, but *don't* use it as entry,
			// because it does not overlap!
			m.pending = append(m.pending, next)

			// Shorten the existing entry (this one overlaps [a, b]).
			entry.Start = start
		}

		// Add the value to this overlap.
		//nolint:gocritic // Slice assignment false positive.
		entry.Value = append(orig, value)

		if prev != nil && prev.End < entry.Start {
			// Add a new interval in between this one and the previous.
			m.pending = append(m.pending, &Entry[K, []V]{
				Start: prev.End + 1,
				End:   entry.Start - 1,
				Value: []V{value},
			})
		}

		prev = entry
	}

	if prev != nil && prev.End < end {
		// Need to insert an extra entry for the stuff between the
		// last interval and end.
		m.pending = append(m.pending, &Entry[K, []V]{
			Start: prev.End + 1,
			End:   end,
			Value: []V{value},
		})
	}

	for _, entry := range m.pending {
		m.tree.Set(entry.End, entry)
	}
	m.pending = m.pending[:0]

	if prev == nil {
		m.tree.Set(end, &Entry[K, []V]{
			Start: start,
			End:   end,
			Value: []V{value},
		})
	}

	return prev == nil
}

// Format implements [fmt.Formatter].
func (m *Intersect[K, V]) Format(s fmt.State, v rune) {
	fmt.Fprint(s, "{")
	first := true
	m.tree.Scan(func(end K, entry *Entry[K, []V]) bool {
		if !first {
			fmt.Fprint(s, ", ")
		}
		first = false

		if entry.Start == end {
			fmt.Fprintf(s, "%#v: ", entry.Start)
		} else {
			fmt.Fprintf(s, "[%#v, %#v]: ", entry.Start, end)
		}
		fmt.Fprintf(s, fmt.FormatString(s, v), entry.Value)

		return true
	})
	fmt.Fprint(s, "}")
}

// intersect returns an iterator over the intervals that intersect [start, end].
func (m *Intersect[K, V]) intersect(start, end K) iter.Seq[*Entry[K, []V]] {
	return func(yield func(*Entry[K, []V]) bool) {
		// Here, [a, b] is the query interval, and [c, d] is the current
		// interval we're looking at.
		a, b := start, end
		iter := m.tree.Iter()

		// We need to walk the tree forwards, finding overlapping intervals,
		// until we find an interval that contains b, is greater than b, or we
		// reach the end of the tree.
		//
		// This logic conveniently handles the case where the map is empty,
		// and when [a, b] is greater than all other intervals, because Seek()
		// will return false.
		for more := iter.Seek(a); more; more = iter.Next() {
			c, _ := iter.Value().Start, iter.Value().End

			// By construction, we already know that a <= d, so if [c, d]
			// is less than b or contains it, it intersects [a, b].
			//
			// If b is less than [c, d], we're already done, so we don't
			// need to yield this value. yield() is only called when !(b < c),
			// i.e. c <= b.
			if b < c || !yield(iter.Value()) {
				return
			}
		}
	}
}
