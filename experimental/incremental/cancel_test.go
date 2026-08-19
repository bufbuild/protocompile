// Copyright 2020-2026 Buf Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

func TestCancelWhileLeaderExecutes(t *testing.T) {
	t.Parallel()

	// The interleaving is timing-dependent; repeat to give the race detector
	// a chance to observe conflicting accesses.
	for range 10 {
		exec := incremental.New(incremental.WithParallelism(4))
		blocking := Blocking{
			Started: make(chan struct{}),
			Release: make(chan struct{}),
		}

		var (
			leaderResults []incremental.Result[int]
			leaderErr     error
			leaderDone    = make(chan struct{})
		)
		go func() {
			defer close(leaderDone)
			leaderResults, _, leaderErr = incremental.Run(t.Context(), exec, incremental.Query[int](blocking))
		}()
		<-blocking.Started

		ctx, cancel := context.WithCancel(t.Context())
		signal := Signal{Executed: make(chan struct{})}
		waiterErr := make(chan error)
		go func() {
			_, _, err := incremental.Run(ctx, exec, incremental.Query[int](signal), incremental.Query[int](blocking))
			waiterErr <- err
		}()
		<-signal.Executed

		cancel()
		close(blocking.Release)
		require.ErrorIs(t, <-waiterErr, context.Canceled)
		<-leaderDone

		require.NoError(t, leaderErr)
		assert.Equal(t, 42, leaderResults[0].Value)
	}
}

func TestCancelWhileLeaderPanics(t *testing.T) {
	t.Parallel()

	// The waiter may instead re-execute the query as a new leader (see
	// PanicBlocking); repeat so the pending-waiter path is exercised.
	for range 10 {
		exec := incremental.New(incremental.WithParallelism(4))
		blocking := PanicBlocking{
			Started: make(chan struct{}),
			Release: make(chan struct{}),
		}

		leaderDone := make(chan struct{})
		go func() {
			defer close(leaderDone)
			_, _, err := incremental.Run(t.Context(), exec, incremental.Query[int](blocking))
			var panicked *incremental.ErrPanic
			assert.ErrorAs(t, err, &panicked)
		}()
		<-blocking.Started

		ctx, cancel := context.WithCancel(t.Context())
		signal := Signal{Executed: make(chan struct{})}
		waiterErr := make(chan error)
		go func() {
			_, _, err := incremental.Run(ctx, exec, incremental.Query[int](signal), incremental.Query[int](blocking))
			waiterErr <- err
		}()
		<-signal.Executed

		// The panic resets the shared result to nil without closing its done
		// channel; cancel only afterwards so the waiter wakes to the nil
		// result.
		close(blocking.Release)
		<-leaderDone
		cancel()
		require.ErrorIs(t, <-waiterErr, context.Canceled)
	}
}

// Blocking signals when it starts executing and then blocks until released.
type Blocking struct {
	Started chan struct{}
	Release chan struct{}
}

func (b Blocking) Key() any {
	return b
}

func (b Blocking) Execute(_ *incremental.Task) (int, error) {
	close(b.Started)
	<-b.Release
	return 42, nil
}

// PanicBlocking is like Blocking, but panics once released.
type PanicBlocking struct {
	Started chan struct{}
	Release chan struct{}
}

func (b PanicBlocking) Key() any {
	return b
}

func (b PanicBlocking) Execute(task *incremental.Task) (int, error) {
	select {
	case <-b.Started:
		// Re-executed as the new leader after the panic below wiped the
		// result; block so the caller still observes cancellation.
		<-task.Context().Done()
		return 0, task.Context().Err()
	default:
	}
	close(b.Started)
	<-b.Release
	panic("boom")
}

// Signal closes Executed when it runs. Resolve spawns goroutines for all
// queries after the first before executing the first synchronously, so once a
// leading Signal executes, later queries in the same Run are in flight.
type Signal struct {
	Executed chan struct{}
}

func (s Signal) Key() any {
	return s
}

func (s Signal) Execute(_ *incremental.Task) (int, error) {
	close(s.Executed)
	return 0, nil
}
