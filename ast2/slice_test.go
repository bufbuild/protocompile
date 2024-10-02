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

package ast2

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPointers(t *testing.T) {
	assert := assert.New(t)

	var p pointers[int]
	assert.Equal(0, p.Len())

	p.Append(5)
	assert.Equal(1, p.Len())
	assert.Equal(5, *p.At(0))

	for i := range 16 {
		p.Append(i + 5)
	}
	assert.Equal(17, p.Len())
	assert.Equal(19, *p.At(15))
	assert.Equal(20, *p.At(16))

	for i := range 32 {
		p.Append(i + 21)
	}
	fmt.Println(p)
	assert.Equal(49, p.Len())
	assert.Equal(51, *p.At(47))
	assert.Equal(52, *p.At(48))

	assert.Equal("[5 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19|20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51|52]", p.String())
}
