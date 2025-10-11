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
	"iter"

	"github.com/tidwall/btree"
)

// Nesting is a collection of intervals (and associated values) arranged in
// such a way that splits the collection into strictly nesting sets:
// a strictly nesting set of intervals is one such that no intervals in it
// overlap, except when one set is a strict subset of another.
//
// Inserting n intervals into this set is worst-case O(n^2 log n). Insertion
// order matters: larger intervals should be inserted first. To prevent nesting,
// insert *shorter* intervals first, instead.
type Nesting[K Endpoint, V any] struct {
	// Keys in each tree are the ends of the intervals.
	sets []*btree.Map[K, *Entry[K, V]]
}

// Clear resets this collection without discarding allocated memory
// (where possible).
func (n *Nesting[K, V]) Clear() {
	for _, set := range n.sets {
		set.Clear()
	}
}

// Entries returns an iterator over the nesting sets in this collection.
//
// Within each set, the order they are yielded in is unspecified.
func (n *Nesting[K, V]) Sets() iter.Seq[iter.Seq[Entry[K, V]]] {
	return func(yield func(iter.Seq[Entry[K, V]]) bool) {
		for _, set := range n.sets {
			if set.Len() == 0 {
				return
			}

			iter := func(yield func(Entry[K, V]) bool) {
				set.Scan(func(_ K, value *Entry[K, V]) bool { return yield(*value) })
			}

			if !yield(iter) {
				return
			}
		}
	}
}

// Insert adds a new interval to the collection.
func (n *Nesting[K, V]) Insert(start, end K, value V) {
	var found *btree.Map[K, *Entry[K, V]]
	for _, set := range n.sets {
		// Two cases under which we insert:
		//
		// 1. We do not intersect anything currently in the set.
		// 2. We overlap precisely one interval.

		iter := set.Iter()
		if !iter.Seek(end) {
			// This would be the greatest end in the set, so we need only
			// check we don't overlap with the greatest interval currently in
			// the set.
			if !iter.Last() || iter.Value().End < start {
				found = set
				break // We're done.
			}

			continue // Partial overlap with last.
		}

		// Check if we lie completely inside of the interval we found or
		// completely outside of it. If the found interval is [c, d], then
		// we want either a < b < c < d or c < a < b < d.
		//
		// Equivalently, the error condition is a <= c <= b
		if start <= iter.Value().Start && iter.Value().Start <= end {
			continue
		}

		// Finally, check that we don't overlap the previous interval. If
		// that interval is [c, d], then this is asking for c < d < a < b.
		//
		// Equivalently, the error condition is a <= d
		if iter.Prev() && start <= iter.Value().End {
			continue
		}

		found = set
		break // We're done.
	}

	if found == nil {
		found = new(btree.Map[K, *Entry[K, V]])
		n.sets = append(n.sets, found)
	}

	found.Set(end, &Entry[K, V]{Start: start, End: end, Value: value})
}
