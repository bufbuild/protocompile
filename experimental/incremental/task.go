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
	"iter"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/bufbuild/protocompile/experimental/report"
)

// ErrNilQuery is set as the Fatal error for the result of executing a nil
// query.
var ErrNilQuery = errors.New("incremental: nil query value")

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

	// Intrusive linked list node for cycle detection.
	path path

	// Set if we're currently holding the executor's semaphore. This exists to
	// ensure that we do not violate concurrency assumptions, and is never
	// itself mutated concurrently.
	holding bool
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
		panic("incremental: called acquire() while holding the semaphore; this is a bug")
	}
	err := t.exec.sema.Acquire(t.ctx, 1)
	if debugIncremental {
		fmt.Fprintf(os.Stderr,
			"incremental: acquire: %p/%d, %T/%v\n",
			t.exec, t.runID, t.path.Query.Underlying(), t.path.Query.Underlying())
	}
	t.holding = err == nil
	return t.holding
}

// release releases a hold on the global semaphore.
func (t *Task) release() {
	if debugIncremental {
		fmt.Fprintf(os.Stderr,
			"incremental: release: %p/%d, %T/%v\n",
			t.exec, t.runID, t.path.Query.Underlying(), t.path.Query.Underlying())
	}
	if t.holding {
		t.exec.sema.Release(1)
	}
	t.holding = false
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

	results = make([]Result[T], len(queries))
	deps := make([]*task, len(queries))

	var wg sync.WaitGroup
	wg.Add(len(queries))

	// TODO: A potential optimization is to make the current goroutine
	// execute the zeroth query, which saves on allocating a fresh g for
	// *every* query.
	anyAsync := false
	for i, q := range queries {
		if q == nil {
			results[i].Fatal = ErrNilQuery
			continue
		}

		q := AsAny(q) // This will also cache the result of q.Key() for us.
		deps[i] = caller.exec.getTask(q.Key())

		async := deps[i].start(caller, q, func(r *result) {
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

			wg.Done()
		})

		anyAsync = anyAsync || async
	}

	// Update dependency links for each of our dependencies. This occurs in a
	// defer block so that it happens regardless of panicking.
	defer func() {
		if caller.task == nil {
			return
		}
		for _, dep := range deps {
			if dep == nil {
				continue
			}

			if caller.task.deps == nil {
				caller.task.deps = map[*task]struct{}{}
			}

			caller.task.deps[dep] = struct{}{}
			for dep := range dep.deps {
				caller.task.deps[dep] = struct{}{}
			}
			if caller.task != nil {
				dep.downstream.Store(caller.task, struct{}{})
			}
		}
	}()

	if anyAsync {
		// Release our current hold on the global semaphore, since we're about to
		// go to sleep. This avoids potential resource starvation for deeply-nested
		// queries on low parallelism settings.
		caller.release()
		wg.Wait()

		// Reacquire from the global semaphore before returning, so
		// execution of the calling task may resume.
		if !caller.acquire() {
			return nil, context.Cause(caller.ctx)
		}
	}

	return results, nil
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
	deps map[*task]struct{} // Transitive.

	// TODO: See the comment on Executor.tasks.
	downstream sync.Map // [*task, struct{}]

	// If this task has not been started yet, this is nil.
	// Otherwise, if it is complete, result.done will be closed.
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

// path is a linked list node for tracking cycles in query dependencies.
type path struct {
	Query *AnyQuery
	Prev  *path
}

// Walk returns an iterator over the linked list.
func (p *path) Walk() iter.Seq[*path] {
	return func(yield func(*path) bool) {
		for node := p; node.Query != nil; node = node.Prev {
			if !yield(node) {
				return
			}
		}
	}
}

// start executes a query in the context of some task and records the result by
// calling done.
func (t *task) start(caller *Task, q *AnyQuery, done func(*result)) (async bool) {
	// Common case for cached values; no need to spawn a separate goroutine.
	r := t.result.Load()
	if r != nil && closed(r.done) {
		done(r)
		return false
	}

	// Complete the rest of the computation asynchronously.
	go func() {
		done(t.run(caller, q))
	}()
	return true
}

// run actually executes the query passed to start. It is called on its own
// goroutine.
func (t *task) run(caller *Task, q *AnyQuery) (output *result) {
	output = t.result.Load()

	defer func() {
		if panicked := recover(); panicked != nil {
			output = nil
			caller.cancel(&ErrPanic{
				Query:     q,
				Panic:     panicked,
				Backtrace: string(debug.Stack()),
			})
		}

		if output != nil && !closed(output.done) {
			if _, ok := output.Fatal.(*ErrCycle); ok {
				// Do not mark completion on cycles: we want the original call of this
				// query to continue executing and handle the cycle in some way.
				return
			}

			if debugIncremental {
				fmt.Fprintf(os.Stderr,
					"incremental: done: %p/%d, %T/%v\n",
					caller.exec, caller.runID, q.Underlying(), q.Underlying())
			}
			close(output.done)
		}
	}()

	// Check for a potential cycle.
	// TODO: use a map for this check.
	var cycle *ErrCycle
	for node := range caller.path.Walk() {
		if node.Query.Key() != q.Key() {
			continue
		}

		cycle = new(ErrCycle)

		// Re-walk the list to collect the cycle itself.
		for node2 := range caller.path.Walk() {
			cycle.Cycle = append(cycle.Cycle, node2.Query)
			if node2 == node {
				break
			}
		}

		// Reverse the list so that dependency arrows point to the
		// right (i.e., Cycle[n] depends on Cycle[n+1]).
		slices.Reverse(cycle.Cycle)

		// Insert a copy of the current query to complete the cycle.
		cycle.Cycle = append(cycle.Cycle, AsAny(q))
		break
	}
	if cycle != nil {
		output.Fatal = cycle
		return output
	}

	// Try to become the leader (the task responsible for computing the result).
	output = &result{done: make(chan struct{})}
	if !t.result.CompareAndSwap(nil, output) {
		// We failed to become the executor, so we're gonna go to sleep
		// until it's done.
		select {
		case <-t.result.Load().done:
		case <-caller.ctx.Done():
		}

		// Reload the result pointer. This is needed if the leader panics,
		// because the result will be set to nil.
		return t.result.Load()
	}

	callee := Task{
		ctx:    caller.ctx,
		exec:   caller.exec,
		runID:  caller.runID,
		task:   t,
		result: output,
		path: path{
			Query: q,
			Prev:  &caller.path,
		},
	}

	if !callee.acquire() {
		return nil
	}
	defer callee.release()

	if debugIncremental {
		fmt.Fprintf(os.Stderr,
			"incremental: executing: %p/%d, %T/%v\n",
			caller.exec, caller.runID, q.Underlying(), q.Underlying())
	}

	output.Value, output.Fatal = q.Execute(&callee)
	output.runID = callee.runID

	if debugIncremental {
		fmt.Fprintf(os.Stderr,
			"incremental: returning: %p/%d, %T/%v\n",
			caller.exec, caller.runID, q.Underlying(), q.Underlying())
	}

	return output
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
