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

func TestPanic(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	_, _, err := incremental.Run(ctx, exec, Panic(false))
	require.NoError(t, err)

	_, _, err = incremental.Run(ctx, exec, Panic(true), Panic(false))
	var panicked *incremental.ErrPanic
	require.ErrorAs(t, err, &panicked)
	assert.Equal(t, panicked.Query.Underlying(), Panic(true))
	assert.Equal(t, "aaa!", panicked.Panic)

	_, _, err = incremental.Run(ctx, exec, Panic(false), Panic(true))
	require.ErrorAs(t, err, &panicked)
	assert.Equal(t, panicked.Query.Underlying(), Panic(true))
	assert.Equal(t, "aaa!", panicked.Panic)
}

func TestAbort(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	_, _, err := incremental.Run(ctx, exec, Abort(false))
	require.NoError(t, err)

	assert.Panics(t, func() {
		_, _, _ = incremental.Run(ctx, exec, Abort(true), Abort(false))
	})
	assert.Panics(t, func() {
		_, _, _ = incremental.Run(ctx, exec, Abort(false), Abort(true))
	})
}

// Panic is a query that conditionally panics, for testing that panic
// propagation works correctly.
type Panic bool

func (p Panic) Key() any {
	return p
}

func (p Panic) Execute(_ *incremental.Task) (bool, error) {
	if p {
		panic("aaa!")
	}
	return bool(p), nil
}

// Abort is a query that conditionally triggers a task abort, for testing that
// task aborts produce a panic on the root goroutine reliably.
type Abort bool

func (a Abort) Key() any {
	return a
}

func (a Abort) Execute(t *incremental.Task) (bool, error) {
	if a {
		t.Abort(a)
	}
	return bool(a), nil
}

func (a Abort) Error() string {
	return "aaa!"
}
