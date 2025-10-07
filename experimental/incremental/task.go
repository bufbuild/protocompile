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
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal"
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

	exec  *Executor
	task  *task
	runID uint64

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

	var underlying any
	if t.task != nil {
		underlying = t.task.query.Underlying()
	}

	t.log("waiting", "%T/%v", underlying, underlying)
	t.holding = t.exec.sema.Acquire(t.ctx, 1) == nil
	t.log("acquire", "%v %T/%v", t.holding, underlying, underlying)

	return t.holding
}

// release releases a hold on the global semaphore.
func (t *Task) release() {
	var underlying any
	if t.task != nil {
		underlying = t.task.query.Underlying()
	}
	t.log("release", "%T/%v", underlying, underlying)

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

	var underlying, thatUnderlying any
	if t.task != nil {
		underlying = t.task.query.Underlying()
	}
	if that.task != nil {
		thatUnderlying = that.task.query.Underlying()
	}
	t.log("acquireFrom", "%T/%v -> %T/%v",
		thatUnderlying, thatUnderlying, underlying, underlying)
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
	t.log("abort", "%T/%v, %v",
		t.task.query.Underlying(), t.task.query.Underlying(), err)

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
	results, _, err := resolve(caller, queries...)
	return results, err
}

// resolve executes the queries returning the results plus the tasks for report
// generation.
func resolve[T any](caller *Task, queries ...Query[T]) ([]Result[T], []*task, error) {
	if len(queries) == 0 {
		return nil, nil, nil
	}
	results := make([]Result[T], len(queries))
	tasks := make([]*task, len(queries))

	// We use a semaphore here instead of a WaitGroup so that when we block
	// on it later in this function, we can bail if caller.ctx is cancelled.
	join := semaphore.NewWeighted(int64(len(queries)))
	join.TryAcquire(int64(len(queries))) // Always succeeds because there are no waiters.

	// Schedule all but the first query to run asynchronously.
	var needWait bool
	for i := len(queries) - 1; i >= 0; i-- {
		query := AsAny(queries[i]) // This will also cache the result of q.Key() for us.
		if query == nil {
			var underlying any
			if caller.task != nil {
				underlying = caller.task.query.Underlying()
			}
			return nil, nil, fmt.Errorf("protocompile/incremental: nil query at index %d while resolving from %T/%v", i, underlying, underlying)
		}
		sync := i == 0
		async := caller.start(query, sync, func(t *task) {
			tasks[i] = t
			var value T
			if t.value != nil {
				// This type assertion will always succeed, unless the user has
				// distinct queries with the same key, which is a sufficiently
				// unrecoverable condition that a panic is acceptable.
				value = t.value.(T) //nolint:errcheck
			}
			results[i] = Result[T]{
				Value:   value,
				Fatal:   t.fatal,
				Changed: t.runID == caller.runID,
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
			return nil, nil, context.Cause(caller.ctx)
		}

		// Reacquire from the global semaphore before returning, so
		// execution of the calling task may resume.
		if !caller.acquire() {
			return nil, nil, context.Cause(caller.ctx)
		}
	}
	return results, tasks, context.Cause(caller.ctx)
}

// checkDone returns an error if this task is completed. This is to avoid shenanigans with
// tasks that escape their scope.
func (t *Task) checkDone() {
	if t.task.done.Load() {
		panic("protocompile/incremental: use of Task after the associated Query.Execute call returned")
	}
}

// task is book-keeping information for a memoized Task in an Executor.
type task struct {
	// The query that executed this task.
	query *AnyQuery

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

	// The deps and parents are protected by the executor.lock.
	// deps is used for cycle detection and transitive error collection.
	// parents is used for cache invalidation to transitively evict dependent tasks.
	deps    map[*task]struct{}
	parents map[*task]struct{}

	// The wait group protects the results. All readers must wait on wg.
	// The done atomic provides a fast-path check to avoid blocking on wg.Wait()
	// when the result is not required. It is set immediately after wg.Done()
	// by the executing goroutine.
	wg     sync.WaitGroup
	value  any
	fatal  error
	report report.Report
	done   atomic.Bool
}

func (t *task) String() string {
	return fmt.Sprintf("%[1]T/%[1]v", t.query.Underlying())
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

// start executes a query in the context of some task and records the result by
// calling done.
//
// If sync is false, the computation will occur asynchronously. Returns whether
// the computation is in fact executing asynchronously as a result.
//
// # Leader Selection
//
// This function implements leader selection for query execution. When multiple
// goroutines race to execute the same query (by key), the first to acquire
// exec.lock becomes the "leader" that executes the query, while others become
// "waiters" that block on the leader's task.wg until completion.
//
// The critical section protected by exec.lock includes:
//   - Checking and updating the tasks map
//   - Adding dependency edges between tasks
//   - Cycle detection via dependency BFS traversal
//
// Query execution (the actual expensive work) happens OUTSIDE the lock, making
// the critical section very short (~100ns). This is why a simple mutex was
// chosen over more complex lock-free approaches like sync.Map.LoadOrStore.
// Zero allocations for the common case.
//
// # Execution Paths
//
// Three paths exist based on the tasks map state:
//
//  1. Cache hit (done): Task completed, return cached result immediately
//  2. Cache hit (in progress): Task executing, wait on task.wg for completion
//  3. Cache miss: Become leader, create task, execute query
//
// Path 2 also performs cycle detection. If a cycle is detected, the task is
// marked with ErrCycle and waiters return the error without blocking.
func (t *Task) start(query *AnyQuery, sync bool, done func(*task)) (async bool) {
	t.exec.lock.Lock()
	if t.exec.tasks == nil {
		t.exec.tasks = make(map[any]*task)
	}
	key := query.Key()
	if c, ok := t.exec.tasks[key]; ok {
		t.addDependencyWithLock(c)
		if c.done.Load() {
			t.exec.lock.Unlock()
			t.log("cache hit fast", "%T/%v", query.Underlying(), query.Underlying())
			done(c) // fast path
			return false
		}
		t.log("cache hit slow", "%T/%v", query.Underlying(), query.Underlying())
		if err := t.hasCycleWithLock(c, query); err != nil {
			// Safe to modify the task as run is waiting for this dependency to complete.
			c.fatal = err
			t.exec.lock.Unlock()
			done(c)
			// Cyclic key is evixted by the run task.
			return false
		}
		t.exec.lock.Unlock()
		if !sync {
			go func() {
				t.wait(c, sync)
				done(c)
			}()
			return true
		}
		t.wait(c, sync)
		done(c)
		return false
	}
	task := new(task)
	task.query = query
	task.runID = t.runID
	task.wg.Add(1)
	t.exec.tasks[key] = task
	t.addDependencyWithLock(task)
	t.exec.lock.Unlock()

	if !sync {
		go func() {
			defer done(task)
			t.run(task, sync)
		}()
		return true
	}
	defer done(task)
	t.run(task, sync)
	return false
}

// addDependencyWithLock links the callee task to the parent caller, building
// the dependency graph used for cycle detection and cache invalidation.
//
// Must be called with t.exec.lock held.
//
// The dependency graph (task.deps and task.parents) is shared mutable state
// that grows as queries are resolved. Multiple goroutines concurrently resolving
// different queries will race to:
//
//  1. Initialize the deps/parents maps (lazy initialization on first use)
//  2. Insert edges into these maps
//  3. Read the graph during cycle detection (hasCycleWithLock)
//  4. Read the graph during cache eviction (Executor.Evict)
//  5. Read the graph during report generation (Executor.generateReport)
//
// Without the lock, these concurrent map operations would cause data races.
// The exec.lock ensures that:
//
//   - Map initialization is atomic (no lost edges from concurrent nil checks)
//   - Map insertions are serialized (no concurrent writes)
//   - Readers see a consistent snapshot of the dependency graph
//
// The dependency graph is built incrementally as queries discover dependencies,
// so edges are added from start() each time a query resolves a sub-query.
func (t *Task) addDependencyWithLock(child *task) {
	parent := t.task
	if parent == nil {
		return // Root task.
	}
	if child.parents == nil {
		child.parents = make(map[*task]struct{})
	}
	if parent.deps == nil {
		parent.deps = make(map[*task]struct{})
	}
	parent.deps[child] = struct{}{}
	child.parents[parent] = struct{}{}
}

// hasCycleWithLock checks if waiting on target would create a cycle.
// Must be called with t.exec.lock held.
// targetQuery is the query being resolved (with import info), which may differ from target.query.
func (t *Task) hasCycleWithLock(target *task, targetQuery *AnyQuery) error {
	if t.task == nil || target == nil {
		return nil
	}

	// Check if a cycle exists using BFS to find if target depends on t.task.
	// We use BFS to find the shortest path for better error messages.
	dependencies := queue[*task]{}
	dependencies.push(target)
	parent := make(map[*task]*task)
	hasCycle := false

	for current := range dependencies.items() {
		if current == t.task {
			hasCycle = true
			break
		}
		for dep := range current.deps {
			if _, ok := parent[dep]; !ok {
				parent[dep] = current
				dependencies.push(dep)
			}
		}
	}

	if !hasCycle {
		return nil
	}

	// Reconstruct the cycle path from t.task back to target.
	var cycle []*AnyQuery
	cycle = append(cycle, t.task.query)
	for current := parent[t.task]; current != nil && current != target; current = parent[current] {
		cycle = append(cycle, current.query)
	}
	cycle = append(cycle, target.query)

	// Reverse to get the correct dependency order (target -> ... -> t.task).
	slices.Reverse(cycle)

	// Add targetQuery at the end to complete the cycle (target -> ... -> t.task -> targetQuery).
	// We use targetQuery instead of target.query because it has the import request info.
	cycle = append(cycle, targetQuery)

	return &ErrCycle{Cycle: cycle}
}

// wait on the query results of the task. The run func is called by another Task.
func (t *Task) wait(task *task, sync bool) {
	// If this task is being executed synchronously with its caller, we need to
	// drop our semaphore hold, otherwise we will deadlock: this caller will
	// be waiting for the leader of this task to complete, but that one
	// may be waiting on a semaphore hold, which it will not acquire due to
	// tasks waiting for it to complete holding the semaphore in this function.
	//
	// If the task is being executed asynchronously, this function is not
	// called while the semaphore is being held, which avoids the above
	// deadlock scenario.
	if sync {
		t.release()
		defer t.acquire()
	}
	t.log("waiting", "%[1]T/%[1]v", task.query.Underlying())
	task.wg.Wait()
}

// run actually executes the query passed to start.
func (t *Task) run(task *task, sync bool) {
	callee := &Task{
		ctx:             t.ctx,
		cancel:          t.cancel,
		exec:            t.exec,
		task:            task,
		runID:           t.runID,
		onRootGoroutine: t.onRootGoroutine && sync,
	}
	defer func() {
		task.wg.Done()
		task.done.Store(true)
		if t.aborted() != nil {
			t.log("aborted", "%[1]T/%[1]v, %[2]v", task.query.Underlying(), t.aborted())
			if !callee.onRootGoroutine {
				_ = recover()
				runtime.Goexit()
			}
			t.exec.Evict(task.query.Key())
			return
		}
		if panicked := recover(); panicked != nil {
			t.log("panic", "%[1]T/%[1]v, %[2]v", task.query.Underlying(), panicked)
			t.cancel(&ErrPanic{
				Query:     task.query,
				Panic:     panicked,
				Backtrace: string(debug.Stack()),
			})
			t.exec.Evict(task.query.Key())
			return
		}
		if t.ctx.Err() != nil {
			t.exec.Evict(task.query.Key())
		} else {
			callee.log("done", "%[1]T/%[1]v", task.query.Underlying())
		}
	}()

	if !sync {
		// If asynchronous, acquire a new semaphore hold.
		if !callee.acquire() {
			return
		}
		defer callee.release()
	} else {
		// Steal our caller's semaphore hold.
		callee.transferFrom(t)
		defer t.transferFrom(callee)
	}

	callee.log("executing", "%[1]T/%[1]v", task.query.Underlying())
	task.value, task.fatal = task.query.Execute(callee)
	callee.log("returning", "%[1]T/%[1]v", task.query.Underlying())
}

type queue[T any] struct {
	head, tail []T
}

func (q *queue[_]) len() int {
	return len(q.head) + len(q.tail)
}
func (q *queue[T]) push(value T) {
	q.head = append(q.head, value)
}
func (q *queue[T]) pop() T {
	if len(q.tail) == 0 {
		q.head, q.tail = q.tail, q.head
		slices.Reverse(q.tail)
	}
	value := q.tail[len(q.tail)-1]
	q.tail = q.tail[:len(q.tail)-1]
	return value
}
func (q *queue[T]) items() iter.Seq[T] {
	return func(yield func(T) bool) {
		for q.len() > 0 {
			value := q.pop()
			if !yield(value) {
				return
			}
		}
	}
}
