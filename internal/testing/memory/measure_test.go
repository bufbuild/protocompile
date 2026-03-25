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

package memory_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/internal/testing/memory"
)

func TestMeasuringTapeMeasure(t *testing.T) {
	t.Parallel()

	mt := new(memory.MeasuringTape)
	bytes := make([]byte, 1000000)
	mt.Measure(bytes)
	require.Equal(t, 1000000, int(mt.Usage()))
	// these do nothing since they are part of already-measured slice
	mt.Measure(bytes[0:10])
	mt.Measure(bytes[1000:10000])
	require.Equal(t, 1000000, int(mt.Usage()))

	int64s := make([]int64, 1000000)
	mt.Measure(int64s)
	require.Equal(t, 9000000, int(mt.Usage()))

	int64ptrs := make([]*int64, 1000000)
	for i := range int64ptrs {
		int64ptrs[i] = &int64s[i]
	}
	mt.Measure(int64ptrs)
	// increase is only the size of slice, not pointed-to values, since all pointers
	// point to locations in already-measured slice above
	ptrBytes := len(int64ptrs) * int(reflect.TypeOf((*int64)(nil)).Size())
	require.Equal(t, 9000000+ptrBytes, int(mt.Usage()))
}
