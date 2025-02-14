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

package arena_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/arena"
)

func TestPointers(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	var a arena.Arena[int]

	p1 := a.NewCompressed(5)
	p2 := a.Deref(p1)
	assert.Equal(5, *a.Deref(p1))

	for i := range 16 {
		a.New(i + 5)
	}
	assert.Equal(19, *a.Deref(16))
	assert.Equal(20, *a.Deref(17))
	assert.Same(a.Deref(p1), p2)

	for i := range 32 {
		a.New(i + 21)
	}
	assert.Equal(51, *a.Deref(48))
	assert.Equal(52, *a.Deref(49))
	assert.Same(a.Deref(p1), p2)

	assert.Equal("[5 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19|20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51|52]", a.String())
}

func TestCompress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	var a arena.Arena[int]

	x := a.NewCompressed(5)
	y := a.Deref(x)
	assert.Equal(x, a.Compress(y))

	assert.Equal(arena.Pointer[int](0), a.Compress(nil))
	assert.Equal(arena.Pointer[int](0), a.Compress(new(int)))

	for i := range 16 {
		a.New(i + 5)
	}
	x = a.NewCompressed(5)
	y = a.Deref(x)
	assert.Equal(x, a.Compress(y))
}
