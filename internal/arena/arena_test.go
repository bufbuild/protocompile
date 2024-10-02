// Copyright 2020-2024 Buf Technologies, Inc.
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

	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/stretchr/testify/assert"
)

func TestPointers(t *testing.T) {
	assert := assert.New(t)

	var a arena.Arena[int]

	p1 := a.New(5)
	p2 := p1.In(&a)
	assert.Equal(5, *p1.In(&a))

	for i := 0; i < 16; i++ {
		a.New(i + 5)
	}
	assert.Equal(19, *arena.Pointer[int](16).In(&a))
	assert.Equal(20, *arena.Pointer[int](17).In(&a))
	assert.True(p1.In(&a) == p2)

	for i := 0; i < 32; i++ {
		a.New(i + 21)
	}
	assert.Equal(51, *arena.Pointer[int](48).In(&a))
	assert.Equal(52, *arena.Pointer[int](49).In(&a))
	assert.True(p1.In(&a) == p2)

	assert.Equal("[5 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19|20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51|52]", a.String())
}
