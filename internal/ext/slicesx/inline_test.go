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
	"testing"

	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/stretchr/testify/assert"
)

func TestInline8(t *testing.T) {
	t.Parallel()

	var s slicesx.Inline[int8]
	assert.Equal(t, []int8{}, s.Slice())
	assert.Equal(t, 16, s.Cap())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 1, 2, 3)
	assert.Equal(t, []int8{1, 2, 3}, s.Slice())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 2, 3, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7)
	assert.Equal(t, []int8{1, 2, 3, 2, 3, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Delete(0)
	s.Delete(0)
	assert.Equal(t, []int8{3, 2, 3, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Compact()
	assert.Equal(t, []int8{3, 2, 3, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7}, s.Slice())
	assert.True(t, s.IsInlined())

	s.Delete(1)
	assert.Equal(t, []int8{3, 3, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7}, s.Slice())
	assert.True(t, s.IsInlined())
}

func TestInline32(t *testing.T) {
	t.Parallel()

	var s slicesx.Inline[int32]
	assert.Equal(t, []int32{}, s.Slice())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 1, 2, 3)
	assert.Equal(t, []int32{1, 2, 3}, s.Slice())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 2, 3)
	assert.Equal(t, []int32{1, 2, 3, 2, 3}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Delete(0)
	s.Delete(0)
	assert.Equal(t, []int32{3, 2, 3}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Compact()
	assert.Equal(t, []int32{3, 2, 3}, s.Slice())
	assert.True(t, s.IsInlined())

	s.Delete(1)
	assert.Equal(t, []int32{3, 3}, s.Slice())
	assert.True(t, s.IsInlined())
}

func TestInline64(t *testing.T) {
	t.Parallel()

	var s slicesx.Inline[int64]
	assert.Equal(t, []int64{}, s.Slice())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 1, 2)
	assert.Equal(t, []int64{1, 2}, s.Slice())
	assert.True(t, s.IsInlined())

	seq.Append(&s, 3)
	assert.Equal(t, []int64{1, 2, 3}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Delete(0)
	s.Delete(0)
	assert.Equal(t, []int64{3}, s.Slice())
	assert.False(t, s.IsInlined())

	s.Compact()
	assert.Equal(t, []int64{3}, s.Slice())
	assert.True(t, s.IsInlined())

	s.Delete(0)
	assert.Equal(t, []int64{}, s.Slice())
	assert.True(t, s.IsInlined())
}
