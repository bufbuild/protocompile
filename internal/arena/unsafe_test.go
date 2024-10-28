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

package arena

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	x := []int{1, 2, 3, 4}
	assert.Equal(-1, pointerIndex[int](nil, nil))
	assert.Equal(-1, pointerIndex(nil, x))
	assert.Equal(-1, pointerIndex(new(int), x))
	assert.Equal(0, pointerIndex(&x[0], x))
	assert.Equal(1, pointerIndex(&x[1], x))
	assert.Equal(2, pointerIndex(&x[2], x))
	assert.Equal(3, pointerIndex(&x[3], x))
	assert.Equal(-1, pointerIndex(&x[0], x[1:]))
	assert.Equal(-1, pointerIndex(&x[3], x[:2]))
	assert.Equal(0, pointerIndex(&x[2], x[2:]))
}
