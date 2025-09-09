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

package incremental_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

func TestCyclic(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := t.Context()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	result, _, err := incremental.Run(ctx, exec, Cyclic{Mod: 5, Step: 3})
	require.NoError(t, err)
	assert.Equal(
		`cycle detected: `+
			`incremental_test.Cyclic{Mod:5, Step:3} -> `+
			`incremental_test.Cyclic{Mod:5, Step:4} -> `+
			`incremental_test.Cyclic{Mod:5, Step:0} -> `+
			`incremental_test.Cyclic{Mod:5, Step:1} -> `+
			`incremental_test.Cyclic{Mod:5, Step:2} -> `+
			`incremental_test.Cyclic{Mod:5, Step:3}`,
		result[0].Fatal.Error(),
	)
}

// Cyclic is a query that queries itself, for triggering cycle detection.
type Cyclic struct {
	Mod, Step int
}

func (c Cyclic) Key() any {
	return c
}

func (c Cyclic) Execute(t *incremental.Task) (int, error) {
	next, err := incremental.Resolve(t, Cyclic{
		Mod:  c.Mod,
		Step: (c.Step + 1) % c.Mod,
	})
	if err != nil {
		return 0, err
	}

	// NOTE: This call is a regression check against a case where calling
	// Report() after a cyclic error would incorrectly treat the cycle point
	// as having been completed.
	t.Report().Remarkf("squaring: %d", next[0].Value)
	return next[0].Value * next[0].Value, next[0].Fatal
}
