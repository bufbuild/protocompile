// Copyright 2020-2026 Buf Technologies, Inc.
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

package synctestx_test

import (
	"sync/atomic"
	"testing"

	"github.com/bufbuild/protocompile/internal/ext/synctestx"
)

// Regression test for https://github.com/bufbuild/protocompile/issues/756:
// Hammer previously raised its start barrier inside the spawn loop, an Add
// from zero concurrent with Wait, which panics intermittently under -race.
// Repeated invocation makes the one-scheduling-decision window deterministic.
func TestHammerRepeated(t *testing.T) {
	t.Parallel()
	for i := range 2000 {
		var calls atomic.Int64
		synctestx.Hammer(4, func() { calls.Add(1) })
		if got := calls.Load(); got != 4 {
			t.Fatalf("iteration %d: f ran %d times, want 4", i, got)
		}
	}
}
