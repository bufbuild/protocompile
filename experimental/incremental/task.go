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

package incremental

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
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
	exec   *Executor
	task   *task
	result *result

	// As long as this is false, this task has an associated hold on exec.sema.
	// This is to ensure that calls to Resolve can temporarily release this
	// hold while waiting for their dependencies to finish.
	done bool

	// Intrusive linked list node for cycle detection.
	path path
}

// Error adds errors to the current query, which will be propagated to all
// queries which depend on it.
//
// This will not cause the query to fail; instead, [Query].Execute should
// return false for the ok value to signal failure.
func (t *Task) NonFatal(errs ...error) {
	t.checkDone()
	t.result.NonFatal = append(t.result.NonFatal, errs...)
}

// Resolve executes a set of queries in parallel. Each query is run on its own
// goroutine.
//
// If the context passed [Executor] expires, this will return [context.Cause].
// The caller MUST immediately propagate this error to ensure the whole query
// graph exits quickly. Failure to do so can result in deadlock or other
// concurrent mayhem.
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
func Resolve[T any](caller Task, queries ...Query[T]) (results []Result[T], expired error) {
	caller.checkDone()

	results = make([]Result[T], len(queries))
	deps := make([]*task, len(queries))
	wg := semaphore.NewWeighted(int64(len(queries))) // Used like a sync.WaitGroup.

	// TODO: A potential optimization is to make the current goroutine
	// execute the zeroth query, which saves on allocating a fresh g for
	// *every* query.
	async := false
	for i, q := range queries {
		i := i // Avoid pesky capture-by-ref of loop induction variables.

		q := AnyQuery(q) // This will also cache the result of q.URL() for us.
		deps[i] = caller.exec.getTask(q.URL())
		async = run(caller, deps[i], q, wg, func() {
			r := deps[i].result.Load()
			if r.Value != nil {
				// This type assertion will always succeed, unless the user has
				// distinct queries with the same URL, which is a sufficiently
				// unrecoverable condition that a panic is acceptable.
				results[i].Value = r.Value.(T) //nolint:errcheck
			}

			results[i].NonFatal = r.NonFatal
			results[i].Fatal = r.Fatal
		}) || async // Need to avoid short-circuiting here!
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

	if async {
		// Release our current hold on the global semaphore, since we're about to
		// go to sleep. This avoids potential resource starvation for deeply-nested
		// queries on low parallelism settings.
		caller.exec.sema.Release(1)

		// This blocks until all of the asynchronous queries have completed, or
		// the context is cancelled.
		if wg.Acquire(caller.ctx, int64(len(queries))) != nil ||
			// Also, re-acquire from the global semaphore before returning, so
			// execution of the calling task may resume.
			caller.exec.sema.Acquire(caller.ctx, 1) != nil {
			return nil, context.Cause(caller.ctx)
		}
	}

	return results, nil
}

// checkDone panics if this task is completed. This is to avoid shenanigans with
// tasks that escape their scope.
func (t *Task) checkDone() {
	if t.done {
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
}

// Result is the Result of executing a query on an [Executor], either via
// [Run] or [Resolve].
type Result[T any] struct {
	Value    T
	NonFatal []error
	Fatal    error
}

// result is a Result[any] with a completion channel appended to it.
type result struct {
	Result[any]
	done chan struct{}
}

// run executes a query in the context of some task and writes the result to
// out.
func run(caller Task, task *task, q Query[any], wg *semaphore.Weighted, done func()) (async bool) {
	// Common case for cached values; no need to spawn a separate goroutine.
	r := task.result.Load()
	if r != nil && closed(r.done) {
		done()
		return false
	}

	if wg.Acquire(caller.ctx, 1) != nil {
		return false
	}

	// Complete the rest of the computation asynchronously.
	go func() {
		defer wg.Release(1)

		// Check for a potential cycle.
		node := &caller.path
		url := q.URL()
		fmt.Println(url)
		for node.Query != nil {
			fmt.Println(node.Query.URL(), url)
			if node.Query.URL() == url {
				err := new(ErrCycle)

				// Re-walk the list to collect the cycle itself.
				node2 := &caller.path
				for {
					err.Cycle = append(err.Cycle, node2.Query)
					if node2 == node {
						break
					}

					node2 = node2.Prev
				}
				// Reverse the list so that dependency arrows point to the
				// right (i.e., Cycle[n] depends on Cycle[n+1]).
				slices.Reverse(err.Cycle)
				// Insert a copy of the current query to complete the cycle.
				err.Cycle = append(err.Cycle, AnyQuery(q))

				r.Fatal = err
				close(r.done)
				done()
				return
			}
			node = node.Prev
		}

		// Try to become the task responsible for computing the result.
		r := &result{done: make(chan struct{})}
		if !task.result.CompareAndSwap(nil, r) {
			// We failed to become the executor, so we're gonna go to sleep
			// until it's done.
			r = task.result.Load()
			select {
			case <-r.done:
				done()
			case <-caller.ctx.Done():
			}
			return
		}

		callee := Task{
			ctx:    caller.ctx,
			exec:   caller.exec,
			task:   task,
			result: r,
			path: path{
				Query: q,
				Prev:  &caller.path,
			},
		}

		if caller.exec.sema.Acquire(caller.ctx, 1) != nil {
			// We were cancelled, reset this task to the "incomplete" state.
			task.result.Store(nil)
		}

		defer func() {
			callee.done = true
			caller.exec.sema.Release(1)
			if closed(r.done) {
				done()
			} else {
				// If the done channel is not closed, this means that we
				// are panicking. Thus, we should abandon whatever partially
				// complete value we have in the result by setting it to nil.
				task.result.Store(nil)
				close(r.done)
			}
		}()

		r.Value, r.Fatal = q.Execute(callee)
		if !closed(r.done) {
			// if a cycle was detected, r.done may have already been closed.
			// Thus, we need to avoid a double-close, which will panic.
			close(r.done)
		}
	}()

	return true
}

// path is a linked list node for tracking cycles in query dependencies.
type path struct {
	Query Query[any]
	Prev  *path
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
