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

package slicesx_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	type p struct {
		v  int
		ok bool
	}
	pack := func(v int, ok bool) p { return p{v, ok} }

	var q slicesx.Queue[int]
	q.PushBack(1)
	assert.Equal(t, []int{1}, slices.Collect(q.Values()))
	x, ok := q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 1, x)
	_, ok = q.PopFront()
	assert.False(t, ok)

	q.PushBack(1, 2, 3)
	assert.Equal(t, []int{1, 2, 3}, slices.Collect(q.Values()))
	assert.Equal(t, 3, *q.Back())
	assert.Equal(t, p{1, true}, pack(q.PopFront()))
	assert.Equal(t, []int{2, 3}, slices.Collect(q.Values()))
	assert.Equal(t, p{3, true}, pack(q.PopBack()))

	q.PushFront(1, 2)
	assert.Equal(t, []int{1, 2, 2}, slices.Collect(q.Values()))
	assert.Equal(t, 2, *q.Back())
	assert.Equal(t, p{1, true}, pack(q.PopFront()))
	assert.Equal(t, []int{2, 2}, slices.Collect(q.Values()))
	assert.Equal(t, p{2, true}, pack(q.PopBack()))
	assert.Equal(t, p{2, true}, pack(q.PopBack()))
}
