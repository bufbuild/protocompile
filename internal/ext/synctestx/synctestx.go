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

package synctestx

import (
	"runtime"
	"sync"
)

// Hammer runs f across count goroutines, ensuring that f is called
// simultaneously, simulating a thundering herd. Returns once all spawned
// goroutines have exited.
//
// If count is zero, uses GOMAXPROCS instead.
func Hammer(count int, f func()) {
	if count == 0 {
		count = runtime.GOMAXPROCS(0)
	}

	start := new(sync.WaitGroup)
	end := new(sync.WaitGroup)
	for range count {
		start.Add(1)
		end.Add(1)
		go func() {
			defer end.Done()

			// This ensures that we have a thundering herd situation: all of
			// these goroutines wake up and hammer f() at the same time.
			start.Done()
			start.Wait()

			f()
		}()
	}

	end.Wait()
}
