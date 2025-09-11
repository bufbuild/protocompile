package interval

import (
	"cmp"
	"fmt"
	"iter"

	"github.com/tidwall/btree"
)

// Map is an interval map, which maps closed intervals with endpoints in K
// to values of type V.
//
// A zero value is ready to use.
type Map[K cmp.Ordered, V any] struct {
	// Keys in this map are the ends of intervals in the map.
	tree btree.Map[K, *entry[K, V]]
}

// Interval is an entry returned by [Map.Insert].
type Interval[K cmp.Ordered, V any] struct {
	// The range for this interval.
	Start, End K

	// The value associated with it.
	Value *V
}

// Get looks up the interval which contains key, if one exists.
//
// If no such interval exists, the Value of the returned [Interval] will be
// nil.
func (m *Map[K, V]) Get(key K) Interval[K, V] {
	iter := m.tree.Iter()
	found := iter.Seek(key)

	if !found || key < iter.Value().start {
		// Check that the interval actually contains key. It is implicit
		// already that key <= end.
		return Interval[K, V]{}
	}

	return Interval[K, V]{
		Start: iter.Value().start,
		End:   iter.Key(),
		Value: &iter.Value().value,
	}
}

// Intervals returns an iterator over the intervals in this map.
func (m *Map[K, V]) Intervals() iter.Seq[Interval[K, V]] {
	return func(yield func(Interval[K, V]) bool) {
		iter := m.tree.Iter()
		more := iter.First()
		for more {
			if !yield(Interval[K, V]{
				Start: iter.Value().start,
				End:   iter.Key(),
				Value: &iter.Value().value,
			}) {
				return
			}
			more = iter.Next()
		}
	}
}

// Insert inserts a new interval into this map, with the given associated value.
// Both endpoints are inclusive.
//
// If [start, end] overlaps any interval present in this map, this function will
// return the interval with the least start that overlaps with it. This case is
// distinguished by overlap.Value != nil.
func (m *Map[K, V]) Insert(start, end K, value V) (overlap Interval[K, V]) {
	if start > end {
		panic(fmt.Sprintf("interval: start (%#v) > end (%#v)", start, end))
	}

	// We need to deal with five cases. Let start and end be a and b here.
	//
	// 1. [a, b] does not overlap any intervals.
	// 2. [a, b] is a subset of an interval.
	// 3. [a, b] intersects the greatest interval before it.
	// 4. [a, b] intersects the least interval after it.
	// 5. [a, b] contains an interval.

	iter := m.tree.Iter()
	if !iter.Seek(start) {
		// Either the map is empty, or there is no interval with a <= d, which
		// means that c <= d < a <= b for all intervals. This is a degenerate
		// version of case (1).
		m.tree.Set(end, &entry[K, V]{
			start: start,
			value: value,
		})
		return Interval[K, V]{}
	}

	switch {
	case end < iter.Value().start:
		// We have that a <= b < c <= d, where [c, d] is the least interval
		// with a <= d. his is case (1).
		m.tree.Set(end, &entry[K, V]{
			start: start,
			value: value,
		})
		return Interval[K, V]{}

	case end <= iter.Key():
		// We instead have that c <= a <= b <= d. This is case (2).
		return Interval[K, V]{
			Start: iter.Value().start,
			End:   iter.Key(),
			Value: &iter.Value().value,
		}
	}

	// To check for case (3), we need c <= a <= d <= b, where [c, d) is the
	// greatest interval with d <= b.
	iter.Seek(end)
	notFirst := iter.Prev()
	// Need to check if start lies within this interval.
	if notFirst {
		if start <= iter.Key() {
			// This is case (3).
			// This is also case (5), which is a <= c <= d <= b.
			return Interval[K, V]{
				Start: iter.Value().start,
				End:   iter.Key(),
				Value: &iter.Value().value,
			}
		}
	}

	// To check for case (4), we need a <= c <= b <= d, where [c, d) is
	// the least interval with b <= d.
	if notFirst {
		iter.Next() // Undo the iter.Prev() above, if it succeeded.
	}

	// By process of elimination, this must be case (4).
	return Interval[K, V]{
		Start: iter.Value().start,
		End:   iter.Key(),
		Value: &iter.Value().value,
	}
}

// Format implements [fmt.Formatter].
func (m *Map[K, V]) Format(s fmt.State, v rune) {
	fmt.Fprint(s, "{")
	first := true
	m.tree.Scan(func(end K, entry *entry[K, V]) bool {
		if !first {
			fmt.Fprint(s, ", ")
		}
		first = false

		if entry.start == end {
			fmt.Fprintf(s, "%#v: ", entry.start)
		} else {
			fmt.Fprintf(s, "[%#v, %#v]: ", entry.start, end)
		}
		fmt.Fprintf(s, fmt.FormatString(s, v), entry.value)

		return true
	})
	fmt.Fprint(s, "}")
}

type entry[K cmp.Ordered, V any] struct {
	start K
	value V
}
