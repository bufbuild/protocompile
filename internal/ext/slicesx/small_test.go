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

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

func TestSmall(t *testing.T) {
	t.Parallel()

	s := slicesx.NewSmall[int32](nil)
	assert.Nil(t, s.Slice())

	s = slicesx.NewSmall[int32]([]int32{1, 2, 3, 4})
	assert.Equal(t, []int32{1, 2, 3, 4}, s.Slice())
}
