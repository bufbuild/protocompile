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

package incremental

import (
	"context"
	"runtime"
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
}

// Error adds errors to the current query, which will be propagated to all
// queries which depend on it.
//
// This will not cause the query to fail; see [Task.Fail] for that.
func (t *Task) Error(errs ...error) {
	t.result.Errors = append(t.result.Errors, errs...)
}

// Fail marks the current query as failed. This will cause all queries that
// depend on this one to also fail.
//
// This function will not return: it will cause a goroutine which calls it to
// exit. See [Resolve] for notes on what this entails.
func (t *Task) Fail(errs ...error) {
	t.Error(errs...)
	t.result.Failed = true
	close(t.result.done)
	runtime.Goexit()
}

// Resolve executes a set of queries in parallel. Each query is run on its own
// goroutine.
//
// If any of the queries fails, or if the [context.Context] passed to the
// [Run] call that spawned the [Task] is cancelled, this function calls
// [runtime.Goexit]. This is not something callers should be concerned about,
// because every call to a Query execution function is done in its own
// goroutine, and only occurs when the return value of [Query].Execute will be
// discarded.
//
// Errors for each result are only those that occurred as a direct result of
// query execution, and *does not* contain that query's transitive errors. This
// is unlike the behavior of [Run].
func Resolve[T any](caller Task, queries ...Query[T]) []Result[T] {
	results := make([]Result[T], len(queries))
	deps := make([]*task, len(queries))
	wg := semaphore.NewWeighted(int64(len(queries)))

	for i, q := range queries {
		i := i
		deps[i] = caller.exec.getTask(q.URL())

		run(caller, deps[i], q, wg, func() {
			r := deps[i].result.Load()
			if r.Value != nil {
				// This type assertion will always succeed, unless the user has
				// distinct queries with the same URL, which is a sufficiently
				// unrecoverable condition that a panic is acceptable.
				results[i].Value = r.Value.(T) //nolint:errcheck
			}

			results[i].Errors = r.Errors
			results[i].Failed = r.Failed
		})
	}

	// Update dependency links for each of our dependencies. This occurs in a
	// defer block so that it happens regardless of Goexit or panics.
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

	// This blocks until all of the asynchronous queries have completed, or
	// the context is cancelled. Note that if the context is cancelled, we
	// DO NOT call Fail, because that permanently marks the query as failed
	// until it or one of its dependencies is invalidated.
	if wg.Acquire(caller.ctx, int64(len(queries))) != nil {
		runtime.Goexit()
	}

	for _, result := range results {
		if result.Failed {
			caller.Fail()
		}
	}

	return results
}

// task is book-keeping information for a memoized Task in an Executor.
type task struct {
	deps       map[*task]struct{} // Transitive.
	downstream sync.Map           // [*task, struct{}]

	// If this task has not been started yet, this is nil.
	// Otherwise, if it is complete, result.done will be closed.
	result atomic.Pointer[result]
}

// Result is the Result of executing a query on an [Executor], either via
// [Run] or [Resolve].
type Result[T any] struct {
	Value  T
	Errors []error
	Failed bool
}

// result is a Result[any] with a completion channel appended to it.
type result struct {
	Result[any]
	done chan struct{}
}

// run executes a query in the context of some task and writes the result to
// out.
func run[T any](caller Task, task *task, q Query[T], wg *semaphore.Weighted, done func()) {
	// Common case for cached values; no need to spawn a separate goroutine.
	r := task.result.Load()
	if r != nil && closed(r.done) {
		done()
		return
	}

	if wg.Acquire(caller.ctx, 1) != nil {
		return
	}

	// Complete the rest of the computation asynchronously.
	go func() {
		defer wg.Release(1)

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
		}

		if caller.exec.sema.Acquire(caller.ctx, 1) != nil {
			// We were cancelled, reset this task to the "incomplete" state.
			task.result.Store(nil)
		}

		defer func() {
			caller.exec.sema.Release(1)
			if closed(r.done) {
				done()
			} else {
				// If the done channel is not closed, this means that we
				// are panicking or called Goexit outside of Task.Fail. Thus,
				// we should abandon whatever partially complete value we have
				// in the result by setting it to nil.
				task.result.Store(nil)
			}
		}()

		r.Value = q.Execute(callee)
		close(r.done)
	}()
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
