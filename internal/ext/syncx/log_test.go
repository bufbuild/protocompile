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

package syncx_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/synctestx"
	"github.com/bufbuild/protocompile/internal/ext/syncx"
)

func TestLog(t *testing.T) {
	t.Parallel()

	const trials = 1000

	log := new(syncx.Log[int])
	synctestx.Hammer(0, func() {
		for range trials {
			n := rand.Int()
			i := log.Append(n)
			assert.Equal(t, n, log.Load(i))
		}
	})

	// Verify that mis-using an index panics.
	i := log.Append(0)
	assert.Panics(t, func() { log.Load(i + 1) })
}

func TestExhaust(t *testing.T) {
	t.Parallel()

	log := new(syncx.Log[int])
	log.SetFull()
	assert.Panics(t, func() { log.Append(0) })
}
