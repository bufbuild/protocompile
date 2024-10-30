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

/*
package incremental implements a query-oriented incremental compilation
framework.

The primary type of this package is [Executor], which executes [Query] values
and caches their results. Queries can themselves depend on other queries, and
can request that those dependencies be executed (in parallel) using [Resolve].

Queries are intended to be relatively fine-grained. For example, there might be
a query that represents "compile a module" that contains a list of file names as
input. It would then depend on the AST queries for each of those files, from
which it would compute lists of imports, and depend on queries based on those
inputs.

# Implementing a Query

Each query must provide a key that uniquely identifies it, and a function for
actually computing it. Queries can partially succeed: instead of a query
returning (T, error), it only returns a T, and errors are flagged to the [Task]
argument.

If a query cannot proceed, it can call [Task.Fail], which will mark the query
as failed and exit the goroutine dedicated to running that query. No queries
that depend on it will be executed. Non-fatal errors can be recorded with
[Task.Error].

This means that generally queries do not need to worry about propagating errors
correctly; this happens automatically in the framework. The entry-point for
query execution, [Run], will return all errors that partially-succeeding or
failing queries return.

Why can queries partially succeed? Consider a parsing operation. This may
generate diagnostics that we want to bubble up to the caller, but whether or
not the presence of errors is actually fatal depends on what the caller wants
to do with the query result. Thus, queries should generally not fail unless
one of their dependencies produced an error they cannot handle.

Queries can inspect errors generated by their direct dependencies, but not by
those dependencies' dependencies. ([Run], however, returns all transitive errors).

# Invalidating Queries

[Executor] supports invalidating queries by key, which will cause all queries
that depended on that query to be discarded and require recomputing. This can be
used e.g. to mark a file as changed and require that everything that that file
depended on is recomputed. See [Executor.Evict].
*/
//nolint:dupword  // "that that" is grammatical!
package incremental
