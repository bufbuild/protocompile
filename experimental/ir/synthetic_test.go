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

package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/intern"
)

func TestSyntheticNames(t *testing.T) {
	t.Parallel()

	table := new(intern.Table)
	names := syntheticNames(mapsx.Set(
		table.Intern("foo"),
		table.Intern("bar"),
		table.Intern("baz"),
		table.Intern("X_baz"),
	))

	assert.Equal(t, "_foo", names.generateIn("_foo", table))
	assert.Equal(t, "X_foo", names.generateIn("_foo", table))
	assert.Equal(t, "_baz", names.generateIn("baz", table))
	assert.Equal(t, "XX_baz", names.generateIn("_baz", table))
}
