// Copyright 2020-2023 Buf Technologies, Inc.
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
	"testing"

	"github.com/igrmk/treemap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasuringTapeInsert(t *testing.T) {
	t.Parallel()

	mt := newMeasuringTape()
	assert.True(t, mt.insert(100, 300)) // 100 -> 400
	verifyMap(t, mt.bst, 100, 400)

	// wholly contained
	assert.False(t, mt.insert(100, 300))
	assert.False(t, mt.insert(150, 200))

	// extends range start
	assert.True(t, mt.insert(50, 300)) // 50 -> 350
	verifyMap(t, mt.bst, 50, 400)

	// extends range end
	assert.True(t, mt.insert(300, 175)) // 300 -> 475
	verifyMap(t, mt.bst, 50, 475)

	// new range above
	assert.True(t, mt.insert(1500, 100)) // 1500 -> 1600
	verifyMap(t, mt.bst, 50, 475, 1500, 1600)

	// new range below
	assert.True(t, mt.insert(10, 10)) // 10 -> 20
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 1600)

	// new range above
	assert.True(t, mt.insert(25000, 50000)) // 25,000 -> 75,000
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 1600, 25000, 75000)

	// new interior range
	assert.True(t, mt.insert(1700, 300)) // 1700 -> 2000
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 1600, 1700, 2000, 25000, 75000)

	// new interior range
	assert.True(t, mt.insert(2100, 300)) // 2100 -> 2400
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 1600, 1700, 2000, 2100, 2400, 25000, 75000)

	// matches range boundary, extends end
	assert.True(t, mt.insert(2400, 100)) // 2400 -> 2500
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 1600, 1700, 2000, 2100, 2500, 25000, 75000)

	// matches both adjacent range boundaries, collapses
	assert.True(t, mt.insert(1600, 100)) // 1600 -> 1700
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 2000, 2100, 2500, 25000, 75000)

	// matches range boundary, extends start
	assert.True(t, mt.insert(24000, 1000)) // 24,000 -> 25,000
	verifyMap(t, mt.bst, 10, 20, 50, 475, 1500, 2000, 2100, 2500, 24000, 75000)

	// encompasses many ranges, collapses
	assert.True(t, mt.insert(10, 3000)) // 10 -> 3010
	verifyMap(t, mt.bst, 10, 3010, 24000, 75000)

	// wholly contained
	assert.False(t, mt.insert(1500, 1510)) // 1500 -> 3010

	mt.other = 99
	assert.Equal(t, 54099, int(mt.memoryUsed()))
}

func TestMeasuringTapeMeasure(t *testing.T) {
	t.Parallel()

	mt := newMeasuringTape()
	bytes := make([]byte, 1000000)
	mt.measure(reflect.ValueOf(bytes))
	require.Equal(t, uint64(1000000), mt.memoryUsed())
	// these do nothing since they are part of already-measured slice
	mt.measure(reflect.ValueOf(bytes[0:10]))
	mt.measure(reflect.ValueOf(bytes[1000:10000]))
	require.Equal(t, uint64(1000000), mt.memoryUsed())

	int64s := make([]int64, 1000000)
	mt.measure(reflect.ValueOf(int64s))
	require.Equal(t, uint64(9000000), mt.memoryUsed())

	int64ptrs := make([]*int64, 1000000)
	for i := range int64ptrs {
		int64ptrs[i] = &int64s[i]
	}
	mt.measure(reflect.ValueOf(int64ptrs))
	// increase is only the size of slice, not pointed-to values, since all pointers
	// point to locations in already-measured slice above
	ptrsSz := uint64(1000000 * reflect.TypeOf(uintptr(0)).Size())
	require.Equal(t, 9000000+ptrsSz, mt.memoryUsed())
}

func verifyMap(t *testing.T, tree *treemap.TreeMap[uintptr, uint64], ranges ...uintptr) {
	t.Helper()
	require.Equal(t, 0, len(ranges)%2, "ranges must be even number of values")

	iter := tree.Iterator()
	for i := 0; i < len(ranges); i += 2 {
		require.True(t, iter.Valid())
		entryEnd := iter.Key()
		entryStart := entryEnd - uintptr(iter.Value())
		type pair struct {
			start, end uintptr
		}
		expected := pair{ranges[i], ranges[i+1]}
		actual := pair{entryStart, entryEnd}
		require.Equal(t, expected, actual)
		iter.Next()
	}
}

func TestNumBuckets(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, numBuckets(0))
	assert.Equal(t, 1, numBuckets(8))
	assert.Equal(t, 2, numBuckets(9))
	assert.Equal(t, 2, numBuckets(16))
	assert.Equal(t, 4, numBuckets(17))
	assert.Equal(t, 4, numBuckets(32))
	assert.Equal(t, 8, numBuckets(33))

	check := func(sz int) {
		b := numBuckets(sz)
		// power of 2
		assert.Equal(t, 1, bits.OnesCount(uint(b)))
		// that fits given size (each bucket holds 8 entries)
		assert.Less(t, b*4, sz)
		assert.GreaterOrEqual(t, b*8, sz)
	}
	check(7364)
	check(1234567)
	check(918373645623)
}
