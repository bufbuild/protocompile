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
	URL() string

	// Executes this query. This function will only be called if the result of
	// this query is not already in the [Executor]'s cache.
	Execute(Task) T
}

// AnyQuery type-erases a [Query].
//
// This is intended to be combined with [Resolve], for cases where queries
// of different types want to be run in parallel.
func AnyQuery[T any](q Query[T]) Query[any] {
	return anyQuery{
		url:     q.URL(),
		execute: func(t Task) any { return q.Execute(t) },
	}
}

type anyQuery struct {
	url     string
	execute func(Task) any
}

func (q anyQuery) URL() string        { return q.url }
func (q anyQuery) Execute(t Task) any { return q.execute(t) }
