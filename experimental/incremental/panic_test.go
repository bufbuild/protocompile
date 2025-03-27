package incremental_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Panic bool

func (p Panic) Key() any {
	return p
}

func (p Panic) Execute(t *incremental.Task) (bool, error) {
	if p {
		panic("aaa!")
	}
	return bool(p), nil
}

func TestPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	_, _, err := incremental.Run(ctx, exec, Panic(false))
	require.NoError(t, err)

	_, _, err = incremental.Run(ctx, exec, Panic(true), Panic(false))
	var panicked *incremental.ErrPanic
	require.True(t, errors.As(err, &panicked))
	assert.Equal(t, panicked.Query, Panic(true))
	assert.Equal(t, panicked.Panic, "aaa!")

	_, _, err = incremental.Run(ctx, exec, Panic(false), Panic(true))
	require.True(t, errors.As(err, &panicked))
	assert.Equal(t, panicked.Query, Panic(true))
	assert.Equal(t, panicked.Panic, "aaa!")
}

type Abort bool

func (a Abort) Key() any {
	return a
}

func (a Abort) Execute(t *incremental.Task) (bool, error) {
	if a {
		incremental.Abort(t, a)
	}
	return bool(a), nil
}

func (a Abort) Error() string {
	return "aaa!"
}
func TestAbort(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	exec := incremental.New(
		incremental.WithParallelism(4),
	)

	_, _, err := incremental.Run(ctx, exec, Abort(false))
	require.NoError(t, err)

	assert.Panics(t, func() {
		incremental.Run(ctx, exec, Abort(true), Abort(false))
	})
	assert.Panics(t, func() {
		incremental.Run(ctx, exec, Abort(false), Abort(true))
	})
}
