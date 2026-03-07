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

package atomicx_test

import (
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/atomicx"
)

func TestLog(t *testing.T) {
	t.Parallel()

	const trials = 1000

	log := new(atomicx.Log[int])

	start := new(sync.WaitGroup)
	end := new(sync.WaitGroup)

	for i := range runtime.GOMAXPROCS(0) {
		start.Add(1)
		end.Add(1)
		go func() {
			defer end.Done()

			// This ensures that we have a thundering herd situation: all of
			// these goroutines wake up and hammer the intern table at the
			// same time.
			start.Done()
			start.Wait()

			for j := range trials {
				n := i*trials + j
				i := log.Append(n)
				assert.Equal(t, n, log.Load(i))
			}
		}()
	}

	end.Wait()
}
