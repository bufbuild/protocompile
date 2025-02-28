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

package benchmarks

import (
	"math/bits"
	"reflect"

	"github.com/igrmk/treemap/v2"
)

type measuringTape struct {
	bst   *treemap.TreeMap[uintptr, uint64]
	other uint64
}

func newMeasuringTape() *measuringTape {
	return &measuringTape{
		bst: treemap.New[uintptr, uint64](),
	}
}

func (t *measuringTape) insert(start uintptr, length uint64) bool {
	if start == 0 {
		// nil ptr
		return false
	}
	end := start + uintptr(length)
	iter := t.bst.LowerBound(start)
	if !iter.Valid() {
		// tree is empty or all entries are too low to overlap
		t.bst.Set(end, length)
		return true
	}
	entryEnd := iter.Key()
	entryStart := entryEnd - uintptr(iter.Value())
	if entryStart > end {
		// range does not exist; add it
		t.bst.Set(end, length)
		return true
	}
	if entryStart <= start && entryEnd >= end {
		// range is entirely encompassed in existing entry
		return false
	}

	// navigate back to find the first overlapping range and push
	// start out if needed to encompass all overlaps
	first := t.bst.Iterator().Key()
	for entryStart > start {
		if iter.Key() == first {
			// can go no further
			break
		}
		iter.Prev()
		if iter.Key() < start {
			// gone back too far
			break
		}
		entryStart = iter.Key() - uintptr(iter.Value())
	}
	if entryStart < start {
		start = entryStart
	}

	// find last overlapping range
	if entryEnd < end {
		for entryEnd < end {
			// remove overlaps that will be replaced with
			// new, larger, encompassing range
			t.bst.Del(entryEnd)

			// Iterator doesn't like concurrent removal of node. So after
			// Del above, we can't call Next; we have to re-search the tree
			// for the next node.
			iter = t.bst.LowerBound(entryEnd)
			if !iter.Valid() {
				// can go no further
				break
			}
			st := iter.Key() - uintptr(iter.Value())
			if st > end {
				// gone too far
				break
			}
			entryEnd = iter.Key()
		}
	}
	if entryEnd > end {
		end = entryEnd
	}

	t.bst.Set(end, uint64(end-start))
	return true
}

func (t *measuringTape) memoryUsed() uint64 {
	iter := t.bst.Iterator()
	var total uint64
	for iter.Valid() {
		total += iter.Value()
		iter.Next()
	}
	return total + t.other
}

func (t *measuringTape) measure(value reflect.Value) {
	// We only need to measure outbound references. So we don't care about the size of the pointer itself
	// if value is a pointer, since that is either passed by value (not on heap) or accounted for in the
	// type that contains the pointer (which we'll have already measured).

	switch value.Kind() {
	case reflect.Pointer:
		if !t.insert(value.Pointer(), uint64(value.Type().Elem().Size())) {
			return
		}
		t.measure(value.Elem())

	case reflect.Slice:
		if !t.insert(value.Pointer(), uint64(value.Cap())*uint64(value.Type().Elem().Size())) {
			return
		}
		for i := range value.Len() {
			t.measure(value.Index(i))
		}

	case reflect.Chan:
		if !t.insert(value.Pointer(), uint64(value.Cap())*uint64(value.Type().Elem().Size())) {
			return
		}
		// no way to query for objects in the channel's buffer :(

	case reflect.Map:
		const mapHdrSz = 48 // estimate based on struct hmap in runtime/map.go
		if !t.insert(value.Pointer(), mapHdrSz) {
			return
		}

		// Can't really get pointers to bucket arrays,
		// so we estimate their size and add them via t.other.
		buckets := numBuckets(value.Len())
		// estimate based on struct bmap in runtime/map.go
		bucketSz := uint64(8 * (value.Type().Key().Size() + value.Type().Elem().Size() + 1))
		t.other += uint64(buckets) * bucketSz

		for iter := value.MapRange(); iter.Next(); {
			t.measure(iter.Key())
			t.measure(iter.Value())
		}

	case reflect.Interface:
		v := value.Elem()
		if v.IsValid() {
			if !isReference(v.Kind()) {
				t.other += uint64(v.Type().Size())
			}
			t.measure(v)
		}

	case reflect.String:
		t.insert(value.Pointer(), uint64(value.Len()))

	case reflect.Struct:
		for i := range value.NumField() {
			t.measure(value.Field(i))
		}

	default:
		// nothing to do
	}
}

func numBuckets(mapSize int) int {
	// each bucket holds 8 entries
	buckets := mapSize / 8
	if mapSize > buckets*8 {
		buckets++
	}
	// Number of buckets is a power of two (map doubles each
	// time it grows).
	highestBit := 63 - bits.LeadingZeros64(uint64(buckets))
	if highestBit >= 0 {
		powerOf2 := 1 << highestBit
		if buckets > powerOf2 {
			powerOf2 <<= 1
		}
		buckets = powerOf2
	}
	return buckets
}

func isReference(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Chan, reflect.Map, reflect.Func:
		return true
	default:
		return false
	}
}
