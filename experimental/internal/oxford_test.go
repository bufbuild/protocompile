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

package internal_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/internal"
)

func TestOxford(t *testing.T) {
	t.Parallel()

	join := func(s ...string) string {
		return fmt.Sprint(internal.Oxford[string]{Conjunction: "and", Elements: s})
	}

	assert.Equal(t, "", join())
	assert.Equal(t, "foo", join("foo"))
	assert.Equal(t, "foo and bar", join("foo", "bar"))
	assert.Equal(t, "foo, bar, and baz", join("foo", "bar", "baz"))
	assert.Equal(t, "foo, bar, baz, and bang", join("foo", "bar", "baz", "bang"))
}
