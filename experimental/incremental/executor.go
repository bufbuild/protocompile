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
	"runtime"
	"slices"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Executor is a caching executor for incremental queries.
//
// See [New], [Run], and [Invalidate].
type Executor struct {
	dirty sync.RWMutex

	// TODO: Evaluate alternatives. sync.Map is pretty bad at having predictable
	// performance, and we may want to add eviction to keep memoization costs
	// in a long-running process (like, say, a language server) down.
	// See https://github.com/dgraph-io/ristretto as a potential alternative.
	tasks sync.Map // [any, *task]

	sema *semaphore.Weighted
}

// ExecutorOption is an option func for [New].
type ExecutorOption func(*Executor)

// New constructs a new executor with the given maximum parallelism.
func New(options ...ExecutorOption) *Executor {
	exec := &Executor{
		sema: semaphore.NewWeighted(int64(runtime.GOMAXPROCS(0))),
	}

	for _, opt := range options {
		opt(exec)
	}

	return exec
}

// WithParallelism sets the maximum number of queries that can execute in
// parallel. Defaults to GOMAXPROCS if not set explicitly.
func WithParallelism(n int64) ExecutorOption {
	return func(e *Executor) { e.sema = semaphore.NewWeighted(n) }
}

// Keys returns a snapshot of the keys of which queries are present (and
// memoized) in an Executor.
//
// The returned slice is sorted.
func (e *Executor) Keys() (keys []string) {
	e.tasks.Range(func(k, t any) bool {
		task := t.(*task) //nolint:errcheck // All values in this map are tasks.
		result := task.result.Load()
		if result == nil || !closed(result.done) {
			return true
		}
		keys = append(keys, fmt.Sprintf("%#v", k))
		return true
	})

	slices.Sort(keys)
	return
}

var runExecutorKey byte

// Run executes a set of queries on this executor in parallel.
//
// This function only returns an error if ctx expires during execution,
// in which case it returns nil and [context.Cause].
//
// Errors that occur during each query are contained within the returned results.
// Unlike [Resolve], these contain the *transitive* errors for each query!
//
// Implementations of [Query].Execute MUST NOT UNDER ANY CIRCUMSTANCES call
// Run. This will result in potential resource starvation or deadlocking, and
// defeats other correctness verification (such as cycle detection). Instead,
// use [Resolve], which takes a [Task] instead of an [Executor].
//
// Note: this function really wants to be a method of [Executor], but it isn't
// because it's generic.
func Run[T any](ctx context.Context, e *Executor, queries ...Query[T]) (results []Result[T], expired error) {
	e.dirty.RLock()
	defer e.dirty.RUnlock()

	// Verify we haven't reëntrantly called Run.
	if callers, ok := ctx.Value(&runExecutorKey).(*[]*Executor); ok {
		if slices.Contains(*callers, e) {
			panic("protocompile/incremental: reentrant call to Run")
		}
		*callers = append(*callers, e)
	} else {
		ctx = context.WithValue(ctx, &runExecutorKey, &[]*Executor{e})
	}
	ctx, cancel := context.WithCancelCause(ctx)

	// Need to acquire a hold on the global semaphore to represent the root
	// task we're about to execute.
	if e.sema.Acquire(ctx, 1) != nil {
		return nil, context.Cause(ctx)
	}
	defer e.sema.Release(1)

	root := Task{
		ctx:    ctx,
		cancel: cancel,
		exec:   e,
		result: &result{done: make(chan struct{})},
	}

	results, expired = Resolve(root, queries...)
	if expired != nil {
		return nil, expired
	}

	// Now, for each result, we need to walk their dependencies and collect
	// their dependencies' non-fatal errors.
	for i, query := range queries {
		task := e.getTask(query.Key())
		for dep := range task.deps {
			r := &results[i]
			r.NonFatal = append(r.NonFatal, dep.result.Load().NonFatal...)
		}
	}

	return results, nil
}

// Evict marks query keys as invalid, requiring those queries, and their
// dependencies, to be recomputed. keys that are not cached are ignored.
//
// This function cannot execute in parallel with calls to [Run], and will take
// an exclusive lock (note that [Run] calls themselves can be run in parallel).
func (e *Executor) Evict(keys ...any) {
	var queue []*task
	for _, key := range keys {
		if t, ok := e.tasks.Load(key); ok {
			queue = append(queue, t.(*task))
		} else {
			return
		}
	}
	if len(queue) == 0 {
		return
	}

	e.dirty.Lock()
	defer e.dirty.Unlock()

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		next.downstream.Range(func(k, _ any) bool {
			queue = append(queue, k.(*task))
			return true
		})

		// Clear everything. We don't need to synchronize here because we have
		// unique ownership of the task.
		*next = task{}
	}
}

// getTask returns (and creates if necessary) a task pointer for the given key.
func (e *Executor) getTask(key any) *task {
	// Avoid allocating a new task object in the common case.
	if t, ok := e.tasks.Load(key); ok {
		return t.(*task)
	}

	t, _ := e.tasks.LoadOrStore(key, new(task))
	return t.(*task)
}