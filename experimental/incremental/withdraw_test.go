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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
)

func TestCancelDoesNotPoisonCache(t *testing.T) {
	t.Parallel()

	exec := incremental.New(incremental.WithParallelism(4))
	query := FlakyOnce{
		Started: make(chan struct{}),
		Calls:   new(atomic.Int32),
	}

	ctx, cancel := context.WithCancel(t.Context())
	run1 := make(chan error)
	go func() {
		_, _, err := incremental.Run(ctx, exec, query)
		run1 <- err
	}()
	<-query.Started
	cancel()
	require.ErrorIs(t, <-run1, context.Canceled)

	// A fresh Run recomputes the query rather than hitting the cache.
	results, _, err := incremental.Run(t.Context(), exec, query)
	require.NoError(t, err)
	require.NoError(t, results[0].Fatal)
	assert.Equal(t, 42, results[0].Value)
	assert.True(t, results[0].Changed)
	assert.Equal(t, int32(2), query.Calls.Load())
}

func TestConcurrentRunSurvivesLeaderPanic(t *testing.T) {
	t.Parallel()

	exec := incremental.New(incremental.WithParallelism(4))
	query := PanicOnce{
		Started: make(chan struct{}),
		Release: make(chan struct{}),
		Calls:   new(atomic.Int32),
	}

	run1 := make(chan error)
	go func() {
		_, _, err := incremental.Run(t.Context(), exec, query)
		run1 <- err
	}()
	<-query.Started

	type run2Result struct {
		value int
		err   error
	}
	run2 := make(chan run2Result)
	go func() {
		results, _, err := incremental.Run(t.Context(), exec, query)
		if err != nil {
			run2 <- run2Result{err: err}
			return
		}
		run2 <- run2Result{value: results[0].Value, err: results[0].Fatal}
	}()

	// Give the second Run time to become a waiter on the pending attempt,
	// then let the leader panic.
	time.Sleep(50 * time.Millisecond)
	close(query.Release)

	var panicked *incremental.ErrPanic
	require.ErrorAs(t, <-run1, &panicked)

	select {
	case r := <-run2:
		require.NoError(t, r.err)
		assert.Equal(t, 42, r.value)
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent Run deadlocked after leader panicked")
	}
}

func TestCancelledAcquireDoesNotWedgeTask(t *testing.T) {
	t.Parallel()

	// Parallelism 1 so that the second query's leader queues on the
	// semaphore behind the first.
	exec := incremental.New(incremental.WithParallelism(1))
	gate := Gate{
		Started: make(chan struct{}),
		Release: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(t.Context())
	run1 := make(chan error)
	go func() {
		_, _, err := incremental.Run(ctx, exec, GatedPair{Gate: gate})
		run1 <- err
	}()

	// The gate holds the only semaphore slot; cancelling now makes
	// WedgeTarget's queued acquisition fail.
	<-gate.Started
	cancel()
	close(gate.Release)
	require.ErrorIs(t, <-run1, context.Canceled)

	results, _, err := incremental.Run(t.Context(), exec, WedgeTarget{})
	require.NoError(t, err)
	require.NoError(t, results[0].Fatal)
	assert.Equal(t, 7, results[0].Value)
}

func TestRetryDoesNotDuplicateDiagnostics(t *testing.T) {
	t.Parallel()

	exec := incremental.New(incremental.WithParallelism(4))
	query := FlakyOnce{
		Started:  make(chan struct{}),
		Calls:    new(atomic.Int32),
		Diagnose: true,
	}

	ctx, cancel := context.WithCancel(t.Context())
	run1 := make(chan error)
	go func() {
		_, _, err := incremental.Run(ctx, exec, query)
		run1 <- err
	}()
	<-query.Started
	cancel()
	require.ErrorIs(t, <-run1, context.Canceled)

	_, report, err := incremental.Run(t.Context(), exec, query)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Len(t, report.Diagnostics, 1)
	assert.Equal(t, int32(2), query.Calls.Load())
}

func TestNoFalseCycleAfterResolvedCycleError(t *testing.T) {
	t.Parallel()

	shared := &falseCycleState{
		detected: make(chan struct{}),
		release:  make(chan struct{}),
	}

	exec := incremental.New(incremental.WithParallelism(4))
	results, _, err := incremental.Run(t.Context(), exec, FalseCycleA{state: shared})
	require.NoError(t, err)
	require.NoError(t, results[0].Fatal)

	// B saw a genuine cycle error for its request of A; C, an innocent
	// bystander waiting on B, must not.
	errB, _ := shared.cycleSeenByB.Load().(error)
	var cycle *incremental.ErrCycle
	require.ErrorAs(t, errB, &cycle)
	errC, _ := shared.errSeenByC.Load().(error)
	require.NoError(t, errC, "C must not observe a cycle error")
	assert.Equal(t, 1, results[0].Value)
}

// FlakyOnce blocks until cancelled on its first execution, and succeeds on
// subsequent ones. If Diagnose is set, it reports a diagnostic each time.
type FlakyOnce struct {
	Started  chan struct{}
	Calls    *atomic.Int32
	Diagnose bool
}

func (f FlakyOnce) Key() any {
	return "flaky-once"
}

func (f FlakyOnce) Execute(t *incremental.Task) (int, error) {
	if f.Diagnose {
		t.Report().Errorf("flaky query executed")
	}
	if f.Calls.Add(1) == 1 {
		close(f.Started)
		<-t.Context().Done()
		return 0, context.Cause(t.Context())
	}
	return 42, nil
}

// PanicOnce blocks until released and panics on its first execution, and
// succeeds on subsequent ones.
type PanicOnce struct {
	Started chan struct{}
	Release chan struct{}
	Calls   *atomic.Int32
}

func (p PanicOnce) Key() any {
	return "panic-once"
}

func (p PanicOnce) Execute(_ *incremental.Task) (int, error) {
	if p.Calls.Add(1) == 1 {
		close(p.Started)
		<-p.Release
		panic("boom")
	}
	return 42, nil
}

// Gate signals when it starts executing and then blocks until released.
type Gate struct {
	Started chan struct{}
	Release chan struct{}
}

func (g Gate) Key() any {
	return "gate"
}

func (g Gate) Execute(_ *incremental.Task) (int, error) {
	close(g.Started)
	<-g.Release
	return 1, nil
}

// GatedPair resolves a Gate and a WedgeTarget together, so that the latter's
// leader queues on the executor semaphore behind the former.
type GatedPair struct {
	Gate Gate
}

func (p GatedPair) Key() any {
	return "gated-pair"
}

func (p GatedPair) Execute(t *incremental.Task) (int, error) {
	_, err := incremental.Resolve(t, p.Gate, WedgeTarget{})
	return 0, err
}

// WedgeTarget is a trivial query used to check that a key does not become
// permanently pending.
type WedgeTarget struct{}

func (WedgeTarget) Key() any {
	return "wedge-target"
}

func (WedgeTarget) Execute(_ *incremental.Task) (int, error) {
	return 7, nil
}

// falseCycleState orchestrates TestNoFalseCycleAfterResolvedCycleError:
// A resolves B and C together; B resolves A and receives a genuine cycle
// error; C then resolves B, which is still pending. Following B's stale
// B -> A edge would fabricate the cycle C -> B -> A -> C.
type falseCycleState struct {
	detected     chan struct{}
	release      chan struct{}
	releaseOnce  sync.Once
	cycleSeenByB atomic.Value // error
	errSeenByC   atomic.Value // error
}

// releaseB unblocks FalseCycleB, at most once.
func (s *falseCycleState) releaseB() {
	s.releaseOnce.Do(func() { close(s.release) })
}

// FalseCycleA resolves both FalseCycleB and FalseCycleC.
type FalseCycleA struct {
	state *falseCycleState
}

func (a FalseCycleA) Key() any {
	return "false-cycle-a"
}

func (a FalseCycleA) Execute(t *incremental.Task) (int, error) {
	results, err := incremental.Resolve(t,
		FalseCycleB(a),
		FalseCycleC(a),
	)
	if err != nil {
		return 0, err
	}
	return results[1].Value, results[1].Fatal
}

// FalseCycleB requests FalseCycleA, expecting a cycle error, and then blocks
// until released.
type FalseCycleB struct {
	state *falseCycleState
}

func (b FalseCycleB) Key() any {
	return "false-cycle-b"
}

func (b FalseCycleB) Execute(t *incremental.Task) (int, error) {
	results, err := incremental.Resolve(t, FalseCycleA(b))
	if err != nil {
		return 0, err
	}
	if fatal := results[0].Fatal; fatal != nil {
		b.state.cycleSeenByB.Store(fatal)
	}
	close(b.state.detected)
	<-b.state.release
	return 1, nil
}

// FalseCycleC waits for FalseCycleB to have observed its cycle error and then
// requests it, which must not produce a cycle error.
type FalseCycleC struct {
	state *falseCycleState
}

func (c FalseCycleC) Key() any {
	return "false-cycle-c"
}

func (c FalseCycleC) Execute(t *incremental.Task) (int, error) {
	<-c.state.detected

	// Release B only once we are (very likely) already waiting on it, and
	// again after Resolve returns so that a spurious cycle error fails the
	// test instead of deadlocking it.
	timer := time.AfterFunc(50*time.Millisecond, c.state.releaseB)
	defer timer.Stop()
	defer c.state.releaseB()

	results, err := incremental.Resolve(t, FalseCycleB(c))
	if err != nil {
		return 0, err
	}
	if fatal := results[0].Fatal; fatal != nil {
		c.state.errSeenByC.Store(fatal)
		return 0, fatal
	}
	return results[0].Value, nil
}
