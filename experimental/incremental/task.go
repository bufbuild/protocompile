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

package incremental

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var (
	errBadAcquire = errors.New("called acquire() while holding the semaphore")
	errBadRelease = errors.New("called release() without holding the semaphore")
)

// Task represents a query that is currently being executed.
//
// Values of type Task are passed to [Query]. The main use of a Task is to
// be passed to [Resolve] to resolve dependencies.
type Task struct {
	// We need all of the contexts for a call to [Run] to be the same, so to
	// avoid user implementations of Query making this mistake (or inserting
	// inappropriate timeouts), we pass the context as part of the task context.
	ctx    context.Context //nolint:containedctx
	cancel func(error)

	exec   *Executor
	task   *task
	result *result
	runID  uint64

	// Set if we're currently holding the executor's semaphore. This exists to
	// ensure that we do not violate concurrency assumptions, and is never
	// itself mutated concurrently.
	holding bool
	// True if this task is intended to execute on the goroutine that called [Run].
	onRootGoroutine bool
}

// Context returns the cancellation context for this task.
func (t *Task) Context() context.Context {
	t.checkDone()
	return t.ctx
}

// Report returns the diagnostic report for this task.
func (t *Task) Report() *report.Report {
	t.checkDone()
	return &t.task.report
}

// acquire acquires a hold on the global semaphore.
//
// Returns false if the underlying context has timed out.
func (t *Task) acquire() bool {
	if t.holding {
		t.abort(errBadAcquire)
	}

	t.holding = t.exec.sema.Acquire(t.ctx, 1) == nil
	t.log("acquire", "%[1]v %[2]T/%[2]v", t.holding, t.task.underlying())

	return t.holding
}

// release releases a hold on the global semaphore.
func (t *Task) release() {
	t.log("release", "%[1]T/%[1]v", t.task.underlying())

	if !t.holding {
		if context.Cause(t.ctx) != nil {
			// This context was cancelled, so acquires prior to this release
			// may have failed, in which case we do nothing instead of panic.
			return
		}

		t.abort(errBadRelease)
	}

	t.exec.sema.Release(1)
	t.holding = false
}

// transferFrom acquires a hold on the global semaphore from the given task.
func (t *Task) transferFrom(that *Task) {
	if t.holding || !that.holding {
		t.abort(errBadAcquire)
	}

	t.holding, that.holding = that.holding, t.holding

	t.log("acquireFrom", "%[1]T/%[1]v -> %[2]T/%[2]v",
		that.task.underlying(),
		t.task.underlying())
}

// log is used for printf debugging in the task scheduling code.
func (t *Task) log(what string, format string, args ...any) {
	internal.DebugLog(
		[]any{"%p/%d", t.exec, t.runID},
		what, format, args...)
}

type errAbort struct {
	err error
}

func (e *errAbort) Unwrap() error {
	return e.err
}

func (e *errAbort) Error() string {
	return fmt.Sprintf(
		"incremental: internal error: %v (this is a bug in protocompile)", e.err,
	)
}

// abort aborts the current computation due to an unrecoverable error.
//
// This will cause the outer call to Run() to immediately wake up and panic.
func (t *Task) abort(err error) {
	t.log("abort", "%[1]T/%[1]v, %[2]v", t.task.underlying(), err)

	if prev := t.aborted(); prev != nil {
		// Prevent multiple errors from cascading and getting spammed all over
		// the place.
		err = prev
	} else {
		err = &errAbort{err}
		t.cancel(err)
	}

	// Destroy the current task, it's in a broken state.
	panic(err)
}

// aborted returns the error passed to [Task.abort] by some task in the current
// Run call.
//
// Returns nil if there is no such error.
func (t *Task) aborted() error {
	err, ok := context.Cause(t.ctx).(*errAbort)
	if !ok {
		return nil
	}
	return err
}

// Resolve executes a set of queries in parallel. Each query is run on its own
// goroutine.
//
// If the context passed [Executor] expires, this will return [context.Cause].
// The caller must propagate this error to ensure the whole query graph exits
// quickly. Failure to propagate the error will result in incorrect query
// results.
//
// If a cycle is detected for a given query, the query will automatically fail
// and produce an [ErrCycle] for its fatal error. If the call to [Query].Execute
// returns an error, that will be placed into the fatal error value, instead.
//
// Callers should make sure to check each result's fatal error before using
// its value.
//
// Non-fatal errors for each result are only those that occurred as a direct
// result of query execution, and *does not* contain that query's transitive
// errors. This is unlike the behavior of [Run], which only collects errors at
// the very end. This ensures that errors are not duplicated, something that
// is not possible to do mid-query.
//
// Note: this function really wants to be a method of [Task], but it isn't
// because it's generic.
func Resolve[T any](caller *Task, queries ...Query[T]) (results []Result[T], expired error) {
	caller.checkDone()
	if len(queries) == 0 {
		return nil, nil
	}

	results = make([]Result[T], len(queries))
	anyQueries := make([]*AnyQuery, len(queries))
	deps := make([]*task, len(queries))

	// We use a semaphore here instead of a WaitGroup so that when we block
	// on it later in this function, we can bail if caller.ctx is cancelled.
	join := semaphore.NewWeighted(int64(len(queries)))
	join.TryAcquire(int64(len(queries))) // Always succeeds because there are no waiters.

	var needWait bool
	for i, qt := range queries {
		q := AsAny(qt) // This will also cache the result of q.Key() for us.
		if q == nil {
			return nil, fmt.Errorf(
				"protocompile/incremental: nil query at index %[1]d while resolving from %[2]T/%[2]v",
				i, caller.task.underlying(),
			)
		}

		anyQueries[i] = q
		dep := caller.exec.getOrCreateTask(q)
		deps[i] = dep

		// Update dependency graph.
		parent := caller.task
		if parent == nil {
			continue // Root.
		}
		parent.deps.Store(dep, struct{}{})
		dep.parents.Store(parent, struct{}{})
	}

	// Schedule all but the first query to run asynchronously.
	for i, dep := range slices.Backward(deps) {
		q := anyQueries[i]
		// Run the zeroth query synchronously, inheriting this task's semaphore hold.
		runNow := i == 0

		async := dep.start(caller, q, runNow, func(r *result) {
			if r != nil {
				if r.Value != nil {
					// This type assertion will always succeed, unless the user has
					// distinct queries with the same key, which is a sufficiently
					// unrecoverable condition that a panic is acceptable.
					results[i].Value = r.Value.(T) //nolint:errcheck
				}

				results[i].Fatal = r.Fatal
				results[i].Changed = r.runID == caller.runID
			}

			join.Release(1)
		})

		needWait = needWait || async
	}

	if needWait {
		// Release our current hold on the global semaphore, since we're about to
		// go to sleep. This avoids potential resource starvation for deeply-nested
		// queries on low parallelism settings.
		caller.release()
		if join.Acquire(caller.ctx, int64(len(queries))) != nil {
			return nil, context.Cause(caller.ctx)
		}

		// Reacquire from the global semaphore before returning, so
		// execution of the calling task may resume.
		if !caller.acquire() {
			return nil, context.Cause(caller.ctx)
		}
	}

	return results, context.Cause(caller.ctx)
}

// checkDone returns an error if this task is completed. This is to avoid shenanigans with
// tasks that escape their scope.
func (t *Task) checkDone() {
	if closed(t.result.done) {
		panic("protocompile/incremental: use of Task after the associated Query.Execute call returned")
	}
}

// task is book-keeping information for a memoized Task in an Executor.
type task struct {
	// The query that executed this task.
	query *AnyQuery

	// Direct dependencies. Tasks that this task depends on.
	// Written on setup in Resolve. May be concurrent on invalid cyclic structures.
	// TODO: See the comment on Executor.tasks.
	deps sync.Map // map[*task]struct{}
	// Inverse of deps. Contains all tasks that directly depend on this task.
	// Written by multiple tasks concurrently.
	// TODO: See the comment on Executor.tasks.
	parents sync.Map // [*task]struct{}

	// If this task has not been started yet, this is nil.
	// Otherwise, if it is complete, result.done will be closed.
	//
	// In other words, if result is non-nil and result.done is not closed, this
	// task is pending.
	result atomic.Pointer[result]
	report report.Report
}

// Result is the Result of executing a query on an [Executor], either via
// [Run] or [Resolve].
type Result[T any] struct {
	Value T // Value is unspecified if Fatal is non-nil.

	Fatal error

	// Set if this result has possibly changed since the last time [Run] call in
	// which this query was computed.
	//
	// This has important semantics wrt to calls to [Run]. If *any* call to
	// [Resolve] downstream of a particular call to [Run] returns a true value
	// for Changed for a particular query, all such calls to [Resolve] will.
	// This ensures that the value of Changed is deterministic regardless of
	// the order in which queries are actually scheduled.
	//
	// This flag can be used to implement partial caching of a query. If a query
	// calculates the result of merging several queries, it can use its own
	// cached result (provided by the caller of [Run] in some way) and the value
	// of [Changed] to only perform a partial mutation instead of a complete
	// merge of the queries.
	Changed bool
}

// result is a Result[any] with a completion channel appended to it.
type result struct {
	Result[any]

	// This is the sequence ID of the Run call that caused this result to be
	// computed. If it is equal to the ID of the current Run, it was computed
	// during the current call. Otherwise, it is cached from a previous Run.
	//
	// Proof of correctness. As long as any Runs are ongoing, it is not possible
	// for queries to be evicted, so once a query is calculated, its runID is
	// fixed. Suppose two Runs race the same query. One of them will win as the
	// leader, and the other will wait until it's done. The leader will mark it
	// with its run ID, so the leader sees Changed and the loser sees !Changed.
	// Any other queries from the same or other Runs racing this query will see
	// the same result.
	//
	// Note that runID itself does not require synchronization, because loads of
	// runID are synchronized-after the done channel being closed.
	runID uint64
	done  chan struct{}
}

// start executes a query in the context of some task and records the result by
// calling done.
//
// If sync is false, the computation will occur asynchronously. Returns whether
// the computation is in fact executing asynchronously as a result.
func (t *task) start(caller *Task, q *AnyQuery, sync bool, done func(*result)) (async bool) {
	// Common case for cached values; no need to spawn a separate goroutine.
	r := t.result.Load()
	if r != nil && closed(r.done) {
		caller.log("cache hit", "%[1]T/%[1]v", q.Underlying())
		done(r)
		return false
	}

	if sync {
		done(t.run(caller, q, false))
		return false
	}

	// Complete the rest of the computation asynchronously.
	go func() {
		done(t.run(caller, q, true))
	}()
	return true
}

// checkCycle checks for a potential cycle. This is only possible if output is
// pending; if it isn't, it can't be in our history path.
func (t *task) checkCycle(caller *Task, q *AnyQuery) error {
	deps := slicesx.NewQueue[*task](1)
	deps.PushFront(t)
	parent := make(map[*task]*task)
	hasCycle := false

	for node, ok := deps.PopFront(); ok; node, ok = deps.PopFront() {
		if node == caller.task {
			hasCycle = true
			break
		}
		node.deps.Range(func(depAny any, _ any) bool {
			dep := depAny.(*task) //nolint:errcheck
			if _, ok := parent[dep]; !ok {
				parent[dep] = node
				deps.PushBack(dep)
			}
			return true
		})
	}

	if !hasCycle {
		return nil
	}

	// Reconstruct the cycle path from t.task back to target.
	var cycle []*AnyQuery
	cycle = append(cycle, caller.task.query)
	for current := parent[caller.task]; current != nil && current != t; current = parent[current] {
		cycle = append(cycle, current.query)
	}
	cycle = append(cycle, t.query)

	// Reverse to get the correct dependency order (target -> ... -> t.task).
	slices.Reverse(cycle)

	// Add q at the end to complete the cycle (target -> ... -> t.task -> targetQuery).
	// We use q instead of t.query because it has the import request info.
	cycle = append(cycle, q)

	return &ErrCycle{Cycle: cycle}
}

// run actually executes the query passed to start. It is called on its own
// goroutine.
func (t *task) run(caller *Task, q *AnyQuery, async bool) (output *result) {
	output = t.result.Load()
	if output != nil {
		if closed(output.done) {
			return output
		}
		return t.waitUntilDone(caller, output, q, async)
	}

	// Try to become the leader (the task responsible for computing the result).
	output = &result{done: make(chan struct{})}
	if !t.result.CompareAndSwap(nil, output) {
		// We failed to become the executor, so we're gonna go to sleep
		// until it's done.
		output := t.result.Load()
		if output == nil {
			return nil // Leader panicked but we did see a result.
		}
		return t.waitUntilDone(caller, output, q, async)
	}

	callee := &Task{
		ctx:    caller.ctx,
		cancel: caller.cancel,
		exec:   caller.exec,
		runID:  caller.runID,
		task:   t,
		result: output,

		onRootGoroutine: caller.onRootGoroutine && !async,
	}

	defer func() {
		if caller.aborted() == nil {
			if panicked := recover(); panicked != nil {
				caller.log("panic", "%[1]T/%[1]v, %[2]v", q.Underlying(), panicked)

				t.result.CompareAndSwap(output, nil)
				output = nil

				caller.cancel(&ErrPanic{
					Query:     q,
					Panic:     panicked,
					Backtrace: string(debug.Stack()),
				})
			}
		} else {
			// If this task is pending and we're the leader, do not allow it to
			// stick around. This will cause future calls to the same failed
			// query to hit the cache.
			t.result.CompareAndSwap(output, nil)
			output = nil

			if !callee.onRootGoroutine {
				// For Gs spawned by the executor, we just kill them here without
				// panicking, so we don't blow up the whole process. The root G for
				// this Run call will panic when it exits Resolve.
				_ = recover()
				runtime.Goexit()
			}
		}

		if output != nil && !closed(output.done) {
			callee.log("done", "%[1]T/%[1]v", q.Underlying())
			close(output.done)
		}
	}()

	if async {
		// If synchronous, this is executing under the hold of the caller query.
		if !callee.acquire() {
			return nil
		}
		defer callee.release()
	} else {
		// Steal our caller's semaphore hold.
		callee.transferFrom(caller)
		defer caller.transferFrom(callee)
	}

	callee.log("executing", "%[1]T/%[1]v", q.Underlying())
	output.Value, output.Fatal = t.query.Execute(callee)
	output.runID = callee.runID
	callee.log("returning", "%[1]T/%[1]v", q.Underlying())

	return output
}

// waitUntilDone waits for this task to be completed by another goroutine.
func (t *task) waitUntilDone(caller *Task, output *result, q *AnyQuery, async bool) *result {
	if err := t.checkCycle(caller, q); err != nil {
		output.Fatal = err
		return output
	}

	// If this task is being executed synchronously with its caller, we need to
	// drop our semaphore hold, otherwise we will deadlock: this caller will
	// be waiting for the leader of this task to complete, but that one
	// may be waiting on a semaphore hold, which it will not acquire due to
	// tasks waiting for it to complete holding the semaphore in this function.
	//
	// If the task is being executed asynchronously, this function is not
	// called while the semaphore is being held, which avoids the above
	// deadlock scenario.
	if !async {
		caller.release()
	}

	select {
	case <-output.done:
	case <-caller.ctx.Done():
	}

	if !async && !caller.acquire() {
		return nil
	}

	// Reload the result pointer. This is needed if the leader panics,
	// because the result will be set to nil.
	return t.result.Load()
}

// underlying returns the tasks query underlying key.
func (t *task) underlying() any {
	if t != nil {
		return t.query.Underlying()
	}
	return nil
}

// closed checks if ch is closed. This may return false negatives, in that it
// may return false for a channel which is closed immediately after this
// function returns.
func closed[T any](ch <-chan T) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}
