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
	"strings"
)

// Query represents an incremental compilation query.
//
// Types which implement Query can be executed by an [Executor], which
// automatically caches the results of a query.
type Query[T any] interface {
	// Returns a unique URL representing this query. Some conventional examples:
	//
	// file:///absolute/path.proto
	//
	// ast://my/proto/file.proto
	//
	// The incremental framework does not attempt to interpret these URLs, and
	// only uses them as keys for memoization.
	//
	// If two distinct queries have the same URL, the incremental framework may
	// produce incorrect results, or panic.
	URL() string

	// Executes this query. This function will only be called if the result of
	// this query is not already in the [Executor]'s cache.
	//
	// The error return should only be used to signal if the query failed. For
	// non-fatal errors, you should record that information with [Task.NonFatal].
	Execute(Task) (value T, fatal error)
}

// ErrCycle is returned by [Resolve] if a cycle occurs during query execution.
type ErrCycle struct {
	// The offending cycle. The first and last queries will have the same URL.
	//
	// To inspect the concrete types of the cycle members, use [DowncastQuery],
	// which will automatically unwrap any calls to [AnyQuery].
	Cycle []Query[any]
}

// Error implements [error].
func (e *ErrCycle) Error() string {
	var buf strings.Builder
	buf.WriteString("cycle detected: ")
	for i, q := range e.Cycle {
		if i != 0 {
			buf.WriteString(" -> ")
		}
		buf.WriteString(q.URL())
	}
	return buf.String()
}

// AnyQuery type-erases a [Query].
//
// This is intended to be combined with [Resolve], for cases where queries
// of different types want to be run in parallel.
func AnyQuery[T any](q Query[T]) Query[any] {
	if q, ok := any(q).(anyQuery); ok {
		return q
	}

	return anyQuery{
		url:     q.URL(),
		execute: func(t Task) (any, error) { return q.Execute(t) },
	}
}

// DowncastQuery undoes the effect of [AnyQuery].
//
// For some Query[any] values, you may be able to use ordinary Go type
// assertions, if the underlying type actually implements Query[any]. However,
// to downcast to a concrete Query[T] type, you must use this function.
func DowncastQuery[Q Query[T], T any](q Query[any]) (downcast Q, ok bool) {
	if downcast, ok = q.(Q); ok {
		return
	}

	qAny, ok := q.(anyQuery)
	if !ok {
		return
	}

	downcast, ok = qAny.actual.(Q)
	return
}

type anyQuery struct {
	actual  any
	url     string
	execute func(Task) (any, error)
}

func (q anyQuery) URL() string                 { return q.url }
func (q anyQuery) Execute(t Task) (any, error) { return q.execute(t) }
