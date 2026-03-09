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

package timex

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStopwatch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		sw := new(Stopwatch)
		assert.Equal(t, time.Duration(0), sw.Elapsed())

		sw.Start()
		time.Sleep(time.Second)
		assert.Equal(t, time.Second, sw.Elapsed())
		time.Sleep(time.Second)
		assert.Equal(t, 2*time.Second, sw.Stop())
		time.Sleep(time.Second)
		assert.Equal(t, 2*time.Second, sw.Elapsed())
		sw.Start()
		time.Sleep(time.Second)
		assert.Equal(t, 3*time.Second, sw.Elapsed())
		sw.Reset()
		time.Sleep(time.Second)
		assert.Equal(t, time.Second, sw.Elapsed())
	})
}
