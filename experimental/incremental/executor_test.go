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
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

type Root struct{}

func (r Root) Key() any {
	return r
}

func (Root) Execute(_ incremental.Task) (struct{}, error) {
	time.Sleep(100 * time.Millisecond)
	return struct{}{}, nil
}

type ParseInt struct {
	Input string
}

func (i ParseInt) Key() any {
	return i
}

func (i ParseInt) Execute(t incremental.Task) (int, error) {
	// This tests that a thundering stampede of queries all waiting on the same
	// query (as in a diamond-shaped graph) do not cause any issues.
	_, err := incremental.Resolve(t, Root{})
	if err != nil {
		return 0, err
	}

	v, err := strconv.Atoi(i.Input)
	if err != nil {
		t.NonFatal(err)
	}
	if v < 0 {
		return 0, fmt.Errorf("negative value: %v", v)
	}
	return v, nil
}

type Sum struct {
	Input string
}

func (s Sum) Key() any {
	return s
}

func (s Sum) Execute(t incremental.Task) (int, error) {
	var queries []incremental.Query[int] //nolint:prealloc
	for _, s := range strings.Split(s.Input, ",") {
		queries = append(queries, ParseInt{s})
	}

	ints, err := incremental.Resolve(t, queries...)
	if err != nil {
		return 0, err
	}

	var v int
	for _, i := range ints {
		if i.Fatal != nil {
			return 0, i.Fatal
		}

		v += i.Value
	}
	return v, nil
}

type Cyclic struct {
	Mod, Step int
}

func (c Cyclic) Key() any {
	return c
}

func (c Cyclic) Execute(t incremental.Task) (int, error) {
	next, err := incremental.Resolve(t, Cyclic{
		Mod:  c.Mod,
		Step: (c.Step + 1) % c.Mod,
	})
	if err != nil {
		return 0, err
	}
	if next[0].Fatal != nil {
		return 0, next[0].Fatal
	}

	return next[0].Value * next[0].Value, nil
}

func TestSum(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	result, err := incremental.Run(ctx, exec, Sum{"1,2,2,3,4"})
	require.NoError(t, err)
	assert.Equal(12, result[0].Value)
	assert.Empty(result[0].NonFatal)
	assert.Equal([]string{
		`incremental_test.ParseInt{Input:"1"}`,
		`incremental_test.ParseInt{Input:"2"}`,
		`incremental_test.ParseInt{Input:"3"}`,
		`incremental_test.ParseInt{Input:"4"}`,
		`incremental_test.Root{}`,
		`incremental_test.Sum{Input:"1,2,2,3,4"}`,
	}, exec.Keys())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,oops,4"})
	require.NoError(t, err)
	assert.Equal(9, result[0].Value)
	assert.Len(result[0].NonFatal, 1)
	assert.Equal([]string{
		`incremental_test.ParseInt{Input:"1"}`,
		`incremental_test.ParseInt{Input:"2"}`,
		`incremental_test.ParseInt{Input:"3"}`,
		`incremental_test.ParseInt{Input:"4"}`,
		`incremental_test.ParseInt{Input:"oops"}`,
		`incremental_test.Root{}`,
		`incremental_test.Sum{Input:"1,2,2,3,4"}`,
		`incremental_test.Sum{Input:"1,2,2,oops,4"}`,
	}, exec.Keys())

	exec.Evict(ParseInt{"4"})
	assert.Equal([]string{
		`incremental_test.ParseInt{Input:"1"}`,
		`incremental_test.ParseInt{Input:"2"}`,
		`incremental_test.ParseInt{Input:"3"}`,
		`incremental_test.ParseInt{Input:"oops"}`,
		`incremental_test.Root{}`,
	}, exec.Keys())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,3,4"})
	require.NoError(t, err)
	assert.Equal(12, result[0].Value)
	assert.Empty(result[0].NonFatal)
	assert.Equal([]string{
		`incremental_test.ParseInt{Input:"1"}`,
		`incremental_test.ParseInt{Input:"2"}`,
		`incremental_test.ParseInt{Input:"3"}`,
		`incremental_test.ParseInt{Input:"4"}`,
		`incremental_test.ParseInt{Input:"oops"}`,
		`incremental_test.Root{}`,
		`incremental_test.Sum{Input:"1,2,2,3,4"}`,
	}, exec.Keys())
}

func TestFatal(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	result, err := incremental.Run(ctx, exec, Sum{"1,2,-3,-4"})
	require.NoError(t, err)
	// NOTE: This error is deterministic, because it's chosen by Sum.Execute.
	assert.Equal("negative value: -3", result[0].Fatal.Error())
	assert.Equal([]string{
		`incremental_test.ParseInt{Input:"-3"}`,
		`incremental_test.ParseInt{Input:"-4"}`,
		`incremental_test.ParseInt{Input:"1"}`,
		`incremental_test.ParseInt{Input:"2"}`,
		`incremental_test.Root{}`,
		`incremental_test.Sum{Input:"1,2,-3,-4"}`,
	}, exec.Keys())
}

func TestCyclic(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	result, err := incremental.Run(ctx, exec, Cyclic{Mod: 5, Step: 3})
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

func TestUnchanged(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	queries := make([]incremental.Query[int], 16)
	for i := range queries {
		queries[i] = ParseInt{"42"}
	}

	// Hammer the same query many, many times to ensure consistency across
	// parallelized and serialized calls.
	const (
		runs     = 4
		gsPerRun = 16
	)

	for i := 0; i < runs; i++ {
		exec.Evict(ParseInt{"42"})
		results, _ := incremental.Run(ctx, exec, queries...)
		for j, r := range results[1:] {
			// All calls after an eviction should return true for Changed.
			assert.True(r.Changed, "%d", j)
		}

		var (
			wg, barrier sync.WaitGroup
			changed     atomic.Int32
		)

		exec.Evict(ParseInt{"42"})
		barrier.Add(1)
		for i := 0; i < gsPerRun; i++ {
			wg.Add(1)
			i := i
			go func() {
				barrier.Wait() // Ensure all goroutines start together.
				defer wg.Done()

				results, _ := incremental.Run(ctx, exec, queries...)
				for j, r := range results {
					// We don't know who the winning g that gets to do the
					// computation will be be, so just require that all of the
					// results within one run agree.
					assert.Equal(results[0].Changed, r.Changed, "%d:%d", i, j)
				}

				if results[0].Changed {
					changed.Add(1)
				}
			}()
		}
		barrier.Done()
		wg.Wait()

		// Exactly one of the gs should have seen a change.
		assert.Equal(int32(1), changed.Load())

		results, _ = incremental.Run(ctx, exec, queries...)
		for j, r := range results[1:] {
			// All calls after computation should return false for Changed.
			assert.False(r.Changed, "%d", j)
		}
	}
}
