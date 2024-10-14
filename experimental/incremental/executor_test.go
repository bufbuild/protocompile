// Copyright 2020-2024 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package incremental_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

type ParseInt string

func (i ParseInt) URL() string {
	return "int://" + string(i)
}

func (i ParseInt) Execute(t incremental.Task) int {
	v, err := strconv.Atoi(string(i))
	if err != nil {
		t.Error(err)
	}
	return v
}

type Sum struct {
	Input string
}

func (s Sum) URL() string {
	return "sum://" + s.Input
}

func (s Sum) Execute(t incremental.Task) (v int) {
	var queries []incremental.Query[int] //nolint:prealloc
	for _, s := range strings.Split(s.Input, ",") {
		queries = append(queries, ParseInt(s))
	}

	ints := incremental.Resolve(t, queries...)
	for _, i := range ints {
		v += i.Value
	}
	return
}

func TestSum(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := context.Background()
	exec := incremental.New(4)

	result, err := incremental.Run(ctx, exec, Sum{"1,2,2,3,4"})
	assert.Equal(12, result[0].Value)
	assert.Empty(result[0].Errors)
	assert.NoError(err) //nolint:testifylint // We want assert, not require.
	assert.Equal([]string{
		"int://1",
		"int://2",
		"int://3",
		"int://4",
		"sum://1,2,2,3,4",
	}, exec.Queries())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,oops,4"})
	assert.Equal(9, result[0].Value)
	assert.Len(result[0].Errors, 1)
	assert.NoError(err) //nolint:testifylint // We want assert, not require.
	assert.Equal([]string{
		"int://1",
		"int://2",
		"int://3",
		"int://4",
		"int://oops",
		"sum://1,2,2,3,4",
		"sum://1,2,2,oops,4",
	}, exec.Queries())

	exec.Invalidate("int://4")
	assert.Equal([]string{
		"int://1",
		"int://2",
		"int://3",
		"int://oops",
	}, exec.Queries())

	result, err = incremental.Run(ctx, exec, Sum{"1,2,2,3,4"})
	assert.Equal(12, result[0].Value)
	assert.Empty(result[0].Errors)
	assert.NoError(err) //nolint:testifylint // We want assert, not require.
	assert.Equal([]string{
		"int://1",
		"int://2",
		"int://3",
		"int://4",
		"int://oops",
		"sum://1,2,2,3,4",
	}, exec.Queries())
}
