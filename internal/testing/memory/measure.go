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

package memory

import (
	"reflect"

	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/ext/reflectx"
	"github.com/bufbuild/protocompile/internal/interval"
)

// MeasuringTape measures how much memory a particular value used.
type MeasuringTape struct {
	// Which memory regions have already been measured. This allows us to
	// detect cycles in a robust manner.
	heap    interval.Intersect[uintptr, struct{}]
	visited map[[2]uintptr]struct{}

	// Extra memory that we cannot get an actual pointer to.
	extra uint64
}

// Usage returns the total number of bytes used.
func (t *MeasuringTape) Usage() uint64 {
	var total uint64
	for entry := range t.heap.Contiguous(false) {
		total += uint64(entry.End - entry.Start + 1)
	}
	return total + t.extra
}

// Measure records the memory transitively reachable through v.
func (t *MeasuringTape) Measure(v any) {
	t.measure(reflect.ValueOf(v))
}

func (t *MeasuringTape) measure(v reflect.Value) {
	insert := func(start uintptr, bytes int) bool {
		if bytes == 0 {
			return false
		}
		end := start + uintptr(bytes)
		if _, ok := t.visited[[2]uintptr{start, end}]; ok {
			return false
		}
		if t.visited == nil {
			t.visited = make(map[[2]uintptr]struct{})
		}
		t.visited[[2]uintptr{start, end}] = struct{}{}

		t.heap.Insert(start, end-1, struct{}{})
		return true
	}

	// We only need to measure outbound references. So we don't care about the
	// size of the pointer itself if value is a pointer, since that is either
	// passed by value (not on heap) or accounted for in the type that contains
	// the pointer (which we'll have already measured).
	//
	// Note that we cannot handle unsafe.Pointer, because reflection cannot
	// tell us how large the pointee is.

	switch v.Kind() {
	case reflect.Pointer:
		if !insert(v.Pointer(), int(v.Type().Elem().Size())) {
			return
		}
		t.measure(v.Elem())

	case reflect.Slice:
		if !insert(v.Pointer(), v.Cap()*int(v.Type().Elem().Size())) {
			return
		}
		for i := range v.Len() {
			t.measure(v.Index(i))
		}

	case reflect.Chan:
		if !insert(v.Pointer(), v.Cap()*int(v.Type().Elem().Size())) {
			return
		}
		// no way to query for objects in the channel's buffer :(

	case reflect.Map:
		const header = 8 * 6 // See internal/maps.Map in maps/map.go.
		if !insert(v.Pointer(), header) {
			return
		}

		t.extra += uint64(estimateMapSize(v))
		for iter := v.MapRange(); iter.Next(); {
			t.measure(iter.Key())
			t.measure(iter.Value())
		}

	case reflect.Interface:
		v := v.Elem()
		if v.IsValid() {
			inner := reflectx.UnwrapStruct(v)
			switch inner.Kind() {
			case reflect.Pointer, reflect.Chan, reflect.Map, reflect.Func:
			default:
				t.extra += uint64(v.Type().Size())
			}
			t.measure(v)
		}

	case reflect.String:
		insert(v.Pointer(), v.Len())

	case reflect.Array:
		for i := range v.Len() {
			t.measure(v.Index(i))
		}

	case reflect.Struct:
		for i := range v.NumField() {
			t.measure(v.Field(i))
		}

	default:
		// nothing to do
	}
}

//nolint:revive,predeclared
func estimateMapSize(m reflect.Value) int {
	const table = 8 * 4 // See internal/maps.table in maps/table.go.
	const groupSize = 8

	// Map size must be a power of two.
	// Note that if len is a power of two, the cap must be the next power of
	// two, because SwissTable requires a load factor of ~7/8.
	cap := bitsx.NextPowerOfTwo(uint(m.Len()))

	// Approximation: this is missing padding.
	group := groupSize + groupSize*(m.Type().Key().Size()+m.Type().Elem().Size())

	// We assume that the internal map directory has exactly one entry in it.
	return table + int(cap/groupSize)*int(group)
}
