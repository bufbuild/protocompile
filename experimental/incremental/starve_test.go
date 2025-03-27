package incremental_test

import (
	"context"
	"testing"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestFanout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	exec := incremental.New(
		// Very low parallelism to ensure we avoid starvation.
		incremental.WithParallelism(2),
	)

	result, _, err := incremental.Run(ctx, exec, Fanout{Depth: 4})
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
