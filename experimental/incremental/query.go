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
	"fmt"
	"strings"
)

// Query represents an incremental compilation query.
//
// Types which implement Query can be executed by an [Executor], which
// automatically caches the results of a query.
//
// Nil query values will not panic the executor; instead, they will fail with
// [ErrNilQuery]. This allows nil queries to be processed in cases where we
// "don't care" about their result, so they can be used as a placeholder.
type Query[T any] interface {
	// Returns a unique key representing this query.
	//
	// This should be a comparable struct type unique to the query type. Failure
	// to do so may result in different queries with the same key, which may
	// result in incorrect results or panics.
	Key() any

	// Executes this query. This function will only be called if the result of
	// this query is not already in the [Executor]'s cache.
	//
	// The error return should only be used to signal if the query failed. For
	// non-fatal errors, you should record that information with [Task.NonFatal].
	//
	// Implementations of this function MUST NOT call [Run] on the executor that
	// is executing them. This will defeat correctness detection, and lead to
	// resource starvation (and potentially deadlocks).
	//
	// Panicking in execute is not interpreted as a fatal error that should be
	// memoized; instead, it is treated as cancellation of the context that
	// was passed to [Run].
	Execute(*Task) (value T, fatal error)
}

// ErrCycle is returned by [Resolve] if a cycle occurs during query execution.
type ErrCycle struct {
	// The offending cycle. The first and last queries will have the same URL.
	//
	// To inspect the concrete types of the cycle members, use [DowncastQuery],
	// which will automatically unwrap any calls to [AnyQuery].
	Cycle []*AnyQuery
}

// Error implements [error].
func (e *ErrCycle) Error() string {
	var buf strings.Builder
	buf.WriteString("cycle detected: ")
	for i, q := range e.Cycle {
		if i != 0 {
			buf.WriteString(" -> ")
		}
		fmt.Fprintf(&buf, "%#v", q.Key())
	}
	return buf.String()
}

// ErrPanic is returned by [Run] if any of the queries it executes panic.
// This error is used to cancel the [context.Context] that governs the call to
// [Run].
type ErrPanic struct {
	Query     *AnyQuery // The query that panicked.
	Panic     any       // The actual value passed to panic().
	Backtrace string    // A backtrace for the panic.
}

// Error implements [error].
func (e *ErrPanic) Error() string {
	return fmt.Sprintf(
		"call to Query.Execute (key: %#v) panicked: %v\n%s",
		e.Query.Key(), e.Panic, e.Backtrace,
	)
}

// AnyQuery is a [Query] that has been type-erased.
type AnyQuery struct {
	actual, key any
	execute     func(*Task) (any, error)
}

// AsAny type-erases a [Query].
//
// This is intended to be combined with [Resolve], for cases where queries
// of different types want to be run in parallel.
//
// If q is nil, returns nil.
func AsAny[T any](q Query[T]) *AnyQuery {
	if q == nil {
		return nil
	}

	if q, ok := any(q).(*AnyQuery); ok {
		return q
	}

	return &AnyQuery{
		actual:  q,
		key:     q.Key(),
		execute: func(t *Task) (any, error) { return q.Execute(t) },
	}
}

// Underlying returns the original, non-AnyQuery query this query was
// constructed with.
func (q *AnyQuery) Underlying() any {
	if q == nil {
		return nil
	}
	return q.actual
}

// Key implements [Query].
func (q *AnyQuery) Key() any { return q.key }

// Execute implements [Query].
func (q *AnyQuery) Execute(t *Task) (any, error) { return q.execute(t) }

// AsTyped undoes the effect of [AsAny].
//
// For some Query[any] values, you may be able to use ordinary Go type
// assertions, if the underlying type actually implements Query[any]. However,
// to downcast to a concrete Query[T] type, you must use this function.
func AsTyped[Q Query[T], T any](q Query[any]) (downcast Q, ok bool) {
	if downcast, ok := q.(Q); ok {
		return downcast, true
	}

	qAny, ok := q.(*AnyQuery)
	if !ok {
		var zero Q
		return zero, false
	}

	downcast, ok = qAny.actual.(Q)
	return downcast, ok
}
