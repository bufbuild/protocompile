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

	q.PushFront(1, 2, 3, 4, 5)
	q.Clear()
	q.PushBack(-1)
	x, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, -1, x)
	_, ok = q.PopFront()
	assert.False(t, ok)
}

// TestQueueWrapAround tests operations when the queue wraps around the buffer.
func TestQueueWrapAround(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](4)
	q.PushBack(1, 2, 3, 4)
	q.PopFront()
	q.PopFront()
	q.PushBack(5, 6)

	assert.Equal(t, []int{3, 4, 5, 6}, slices.Collect(q.Values()))

	v, ok := q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 3, v)

	v, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 4, v)

	v, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 5, v)

	v, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 6, v)

	_, ok = q.PopFront()
	assert.False(t, ok)
}

// TestQueueBackWithWrapping tests Back() when end wraps to 0.
func TestQueueBackWithWrapping(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](4)
	q.PushBack(1, 2, 3, 4)

	q.PopFront()
	q.PopFront()
	q.PopFront()
	q.PopFront()

	q.PushBack(5, 6, 7, 8)

	back := q.Back()
	assert.NotNil(t, back)
	assert.Equal(t, 8, *back)

	v, ok := q.PopBack()
	assert.True(t, ok)
	assert.Equal(t, 8, v)
}

// TestQueueResizeWithWrapping tests that resize works correctly when buffer wraps.
func TestQueueResizeWithWrapping(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](2)
	q.PushBack(1, 2)
	q.PopFront()
	q.PushBack(3)

	assert.Equal(t, []int{2, 3}, slices.Collect(q.Values()))

	q.PushBack(4, 5, 6, 7, 8)

	expected := []int{2, 3, 4, 5, 6, 7, 8}
	assert.Equal(t, expected, slices.Collect(q.Values()))

	// Verify PopFront returns correct values, not zeros
	for i, want := range expected {
		v, ok := q.PopFront()
		assert.True(t, ok, "PopFront at index %d should succeed", i)
		assert.Equal(t, want, v, "PopFront at index %d should return %d", i, want)
	}
}

// TestQueueMixedOperations tests complex mixed push/pop scenarios.
func TestQueueMixedOperations(t *testing.T) {
	t.Parallel()

	var q slicesx.Queue[int]

	q.PushBack(1, 2, 3)
	q.PushFront(0)
	q.PushBack(4, 5)

	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, slices.Collect(q.Values()))

	v, ok := q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 0, v)

	v, ok = q.PopBack()
	assert.True(t, ok)
	assert.Equal(t, 5, v)

	v, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	assert.Equal(t, []int{2, 3, 4}, slices.Collect(q.Values()))
}

// TestQueueLargeSequence tests a large sequence that causes multiple wraps and resizes.
func TestQueueLargeSequence(t *testing.T) {
	t.Parallel()

	var q slicesx.Queue[int]

	for i := range 1000 {
		q.PushBack(i)
	}

	for i := range 500 {
		v, ok := q.PopFront()
		assert.True(t, ok)
		assert.Equal(t, i, v)
	}

	for i := 1000; i < 1500; i++ {
		q.PushBack(i)
	}

	expected := make([]int, 1000)
	for i := range 1000 {
		expected[i] = 500 + i
	}
	assert.Equal(t, expected, slices.Collect(q.Values()))

	for i := 500; i < 1500; i++ {
		v, ok := q.PopFront()
		assert.True(t, ok, "PopFront should succeed at %d", i)
		assert.Equal(t, i, v, "Expected %d, got %d", i, v)
	}
}

// TestQueueFrontAndBackPointers tests Front() and Back() pointer operations.
func TestQueueFrontAndBackPointers(t *testing.T) {
	t.Parallel()

	var q slicesx.Queue[int]

	assert.Nil(t, q.Front())
	assert.Nil(t, q.Back())

	q.PushBack(42)
	assert.Equal(t, 42, *q.Front())
	assert.Equal(t, 42, *q.Back())

	q.PushBack(43, 44)
	assert.Equal(t, 42, *q.Front())
	assert.Equal(t, 44, *q.Back())

	// After wrapping
	q.PopFront()
	q.PushBack(45)
	assert.Equal(t, 43, *q.Front())
	assert.Equal(t, 45, *q.Back())
}

// TestQueuePopFromEmpty tests popping from an empty queue.
func TestQueuePopFromEmpty(t *testing.T) {
	t.Parallel()

	var q slicesx.Queue[int]

	v, ok := q.PopFront()
	assert.False(t, ok)
	assert.Equal(t, 0, v)

	v, ok = q.PopBack()
	assert.False(t, ok)
	assert.Equal(t, 0, v)

	// After clearing queue
	q.PushBack(1)
	q.PopFront()

	v, ok = q.PopFront()
	assert.False(t, ok)
	assert.Equal(t, 0, v)
}

// TestQueueResizeWhileWrapped tests resize when buffer has wrapped around.
func TestQueueResizeWhileWrapped(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](4)
	q.PushBack(1, 2, 3, 4)
	q.PopFront()
	q.PopFront()
	q.PushBack(5, 6)

	assert.Equal(t, []int{3, 4, 5, 6}, slices.Collect(q.Values()))

	q.PushBack(7, 8, 9)

	expected := []int{3, 4, 5, 6, 7, 8, 9}
	assert.Equal(t, expected, slices.Collect(q.Values()))

	for i, want := range expected {
		v, ok := q.PopFront()
		assert.True(t, ok, "PopFront at index %d should succeed", i)
		assert.Equal(t, want, v, "PopFront at index %d should return %d, not zero", i, want)
	}
}

// TestQueuePushFrontWithWrapping tests PushFront when buffer wraps.
func TestQueuePushFrontWithWrapping(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](4)
	q.PushBack(1, 2, 3, 4)
	q.PopBack()
	q.PopBack()

	// PushFront with slice maintains slice order at front
	q.PushFront(0, -1)

	assert.Equal(t, []int{0, -1, 1, 2}, slices.Collect(q.Values()))

	v, ok := q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, 0, v)

	v, ok = q.PopFront()
	assert.True(t, ok)
	assert.Equal(t, -1, v)
}

// TestQueueStressTest performs many operations to catch edge cases.
func TestQueueStressTest(t *testing.T) {
	t.Parallel()

	var q slicesx.Queue[int]
	var expected []int

	for i := range 100 {
		switch i % 7 {
		case 0, 1:
			q.PushBack(i)
			expected = append(expected, i)
		case 2:
			q.PushFront(i)
			expected = append([]int{i}, expected...)
		case 3, 4:
			if len(expected) > 0 {
				v, ok := q.PopFront()
				assert.True(t, ok)
				assert.Equal(t, expected[0], v)
				expected = expected[1:]
			}
		case 5:
			if len(expected) > 0 {
				v, ok := q.PopBack()
				assert.True(t, ok)
				assert.Equal(t, expected[len(expected)-1], v)
				expected = expected[:len(expected)-1]
			}
		case 6:
			if len(expected) == 0 {
				assert.Equal(t, 0, q.Len())
			} else {
				assert.Equal(t, expected, slices.Collect(q.Values()))
			}
		}
	}

	assert.Equal(t, len(expected), q.Len())
	assert.Equal(t, expected, slices.Collect(q.Values()))
}

// TestQueueResizeWithGap specifically targets the resize bug with wrapped buffer.
func TestQueueResizeWithGap(t *testing.T) {
	t.Parallel()

	q := slicesx.NewQueue[int](3)
	q.PushBack(10, 20, 30)

	v1, _ := q.PopFront()
	v2, _ := q.PopFront()
	assert.Equal(t, 10, v1)
	assert.Equal(t, 20, v2)

	q.PushBack(40, 50, 60)

	assert.Equal(t, []int{30, 40, 50, 60}, slices.Collect(q.Values()))

	q.PushBack(70, 80, 90)

	expected := []int{30, 40, 50, 60, 70, 80, 90}
	assert.Equal(t, expected, slices.Collect(q.Values()))

	for i, want := range expected {
		v, ok := q.PopFront()
		assert.True(t, ok, "PopFront at index %d should succeed", i)
		assert.Equal(t, want, v, "PopFront at index %d: expected %d, got %d (zero value means resize bug)", i, want, v)
	}
}
