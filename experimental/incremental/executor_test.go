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

package incremental_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

type ParseInt string

func (i ParseInt) URL() string {
	return incremental.URLBuilder{
		Scheme: "int",
		Opaque: string(i),
	}.Build()
}

func (i ParseInt) Execute(t incremental.Task) (int, error) {
	v, err := strconv.Atoi(string(i))
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

func (s Sum) URL() string {
	return incremental.URLBuilder{
		Scheme: "sum",
		Opaque: s.Input,
	}.Build()
}

func (s Sum) Execute(t incremental.Task) (int, error) {
	var queries []incremental.Query[int] //nolint:prealloc
	for _, s := range strings.Split(s.Input, ",") {
		queries = append(queries, ParseInt(s))
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

func (c Cyclic) URL() string {
	return incremental.URLBuilder{
		Scheme:  "cyclic",
		Opaque:  strconv.Itoa(c.Mod),
		Queries: [][2]string{{"step", strconv.Itoa(c.Step)}},
	}.Build()
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
		"int:1",
		"int:2",
		"int:3",
		"int:4",
		"sum:1,2,2,3,4",
	}, exec.URLs())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,oops,4"})
	require.NoError(t, err)
	assert.Equal(9, result[0].Value)
	assert.Len(result[0].NonFatal, 1)
	assert.Equal([]string{
		"int:1",
		"int:2",
		"int:3",
		"int:4",
		"int:oops",
		"sum:1,2,2,3,4",
		"sum:1,2,2,oops,4",
	}, exec.URLs())

	exec.Evict("int:4")
	assert.Equal([]string{
		"int:1",
		"int:2",
		"int:3",
		"int:oops",
	}, exec.URLs())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,3,4"})
	require.NoError(t, err)
	assert.Equal(12, result[0].Value)
	assert.Empty(result[0].NonFatal)
	assert.Equal([]string{
		"int:1",
		"int:2",
		"int:3",
		"int:4",
		"int:oops",
		"sum:1,2,2,3,4",
	}, exec.URLs())
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
		"int:-3",
		"int:-4",
		"int:1",
		"int:2",
		"sum:1,2,-3,-4",
	}, exec.URLs())
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
		"cycle detected: cyclic:5?step=3 -> cyclic:5?step=4 -> cyclic:5?step=0 -> cyclic:5?step=1 -> cyclic:5?step=2 -> cyclic:5?step=3",
		result[0].Fatal.Error(),
	)
}
