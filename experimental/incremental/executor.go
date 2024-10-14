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
	"slices"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Executor is a caching executor for incremental queries.
//
// See [New], [Run], and [Invalidate].
type Executor struct {
	dirty sync.RWMutex
	tasks sync.Map // [string, *task]

	sema *semaphore.Weighted
}

// New constructs a new executor with the given maximum parallelism.
//
// Setting parallelism to zero or negative will default to GOMAXPROCS.
func New(parallelism int) *Executor {
	if parallelism <= 0 {
		parallelism = runtime.GOMAXPROCS(0)
	}

	return &Executor{
		sema: semaphore.NewWeighted(int64(parallelism)),
	}
}

// Queries returns a snapshot of the URLs of which queries are present (and
// memoized) in an Executor.
//
// The returned slice is sorted.
func (e *Executor) Queries() (urls []string) {
	e.tasks.Range(func(url, t any) bool {
		task := t.(*task) //nolint:errcheck // All values in this map are tasks.
		result := task.result.Load()
		if result == nil || !closed(result.done) {
			return true
		}
		urls = append(urls, url.(string))
		return true
	})

	slices.Sort(urls)
	return
}

// Run executes a set of queries on this executor in parallel.
//
// This function only returns an error if ctx is cancelled during execution,
// in which case it returns nil and the cause for cancellation.
//
// Errors that occur during each query are contained within the returned results.
// Unlike [Resolve], these contain the *transitive* errors for each query!
func Run[T any](ctx context.Context, e *Executor, queries ...Query[T]) (results []Result[T], cancelCause error) {
	e.dirty.RLock()
	defer e.dirty.RUnlock()

	root := Task{
		ctx:    ctx,
		exec:   e,
		result: &result{done: make(chan struct{})},
	}

	go func() {
		results = Resolve(root, queries...)
		close(root.result.done)
	}()
	select {
	case <-root.result.done:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}

	// Now, for each non-failed result, we need to walk their dependencies and
	// collect their errors.
	for i, query := range queries {
		task := e.getTask(query.URL())
		for dep := range task.deps {
			r := &results[i]
			r.Errors = append(r.Errors, dep.result.Load().Errors...)
		}
	}

	return results, nil
}

// Invalidate marks query URLs as invalid, requiring those queries, and their
// dependencies, to be recomputed. URLs that are not cached are ignored.
//
// This function cannot execute in parallel with calls to [Run], and will take
// an exclusive lock (note that [Run] calls themselves can be run in parallel).
func (e *Executor) Invalidate(urls ...string) {
	var queue []*task
	for _, url := range urls {
		if t, ok := e.tasks.Load(url); ok {
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

// getTask returns (and creates if necessary) a task pointer for the given URL.
func (e *Executor) getTask(url string) *task {
	// Avoid allocating a new task object in the common case.
	if t, ok := e.tasks.Load(url); ok {
		return t.(*task)
	}

	t, _ := e.tasks.LoadOrStore(url, new(task))
	return t.(*task)
}
