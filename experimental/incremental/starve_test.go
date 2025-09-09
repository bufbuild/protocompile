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

func TestStarvation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	exec := incremental.New(
		// Force serialization to test to trigger starvation corner-cases.
		incremental.WithParallelism(1),
	)

	result, _, err := incremental.Run(ctx, exec,
		Fanout{Depth: 4},

		// This triggers a corner-case handled in (*task).waitUntilDone.
		Fanout{Depth: 4, Level: 4, Index: 3},
	)
	require.NoError(t, err)
	assert.Equal(t, 1*2*3*4, result[0].Value)
	assert.Equal(t, []string{
		"incremental_test.Fanout{Depth:4, Level:0, Index:0}",
		"incremental_test.Fanout{Depth:4, Level:1, Index:0}",
		"incremental_test.Fanout{Depth:4, Level:2, Index:0}",
		"incremental_test.Fanout{Depth:4, Level:2, Index:1}",
		"incremental_test.Fanout{Depth:4, Level:3, Index:0}",
		"incremental_test.Fanout{Depth:4, Level:3, Index:1}",
		"incremental_test.Fanout{Depth:4, Level:3, Index:2}",
		"incremental_test.Fanout{Depth:4, Level:4, Index:0}",
		"incremental_test.Fanout{Depth:4, Level:4, Index:1}",
		"incremental_test.Fanout{Depth:4, Level:4, Index:2}",
		"incremental_test.Fanout{Depth:4, Level:4, Index:3}",
	}, exec.Keys())
}

// Fanout is a query that spawns a wide fan-out of subqueries, quadratic in
// Depth.
type Fanout struct {
	Depth, Level, Index int
}

func (c Fanout) Key() any {
	return c
}

func (c Fanout) Execute(t *incremental.Task) (int, error) {
	if c.Depth == c.Level {
		return 1, nil
	}

	queries := make([]incremental.Query[int], c.Level+1)
	for i := range queries {
		queries[i] = Fanout{
			Depth: c.Depth,
			Level: c.Level + 1,
			Index: i,
		}
	}

	results, err := incremental.Resolve(t, queries...)
	if err != nil {
		return 0, err
	}

	var total int
	for _, r := range results {
		total += r.Value
	}

	return total, nil
}
