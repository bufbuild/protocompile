// Package fuzztesting contains helpers for reproducing issues found by
// fuzz tests configured with https://github.com/google/oss-fuzz. The
// resulting test cases also verify that we don't have regressions.
package fuzztesting

import (
	"context"
	"testing"
	"time"

	"github.com/bufbuild/protocompile/internal"
)

func RunWithFuzzerTimeout(t *testing.T, fn func(ctx context.Context)) {
	// Fuzz testing complains if 100 iterations takes longer than 60 seconds.
	// We're only running 3 iterations, so these tests aren't too slow.
	// So we can use a much tighter deadline.
	allowedDuration := 2 * time.Second
	if internal.IsRace {
		// We increase that threshold to 20 seconds when the race detector is enabled.
		// The race detector has been observed to make it take ~8x as long. If coverage
		// is *also* enabled, the test can take 19x as long(!!). Unfortunately, there
		// doesn't appear to be a way to easily detect if coverage is enabled, so we
		// always increase the timeout when race detector is enabled.
		allowedDuration = 20 * time.Second
		t.Logf("allowing %v since race detector is enabled", allowedDuration)
	}
	ctx, cancel := context.WithTimeout(context.Background(), allowedDuration)
	defer func() {
		if ctx.Err() != nil {
			t.Errorf("test took too long to execute (> %v)", allowedDuration)
		}
		cancel()
	}()
	for i := 0; i < 3; i++ {
		if ctx.Err() != nil {
			break
		}
		fn(ctx)
	}
}
