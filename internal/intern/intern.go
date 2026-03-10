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

// Package intern provides an interning table abstraction to optimize symbol
// resolution.
package intern

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/ext/syncx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// ID is an interned string in a particular [Table].
//
// IDs can be compared very cheaply. The zero value of ID always
// corresponds to the empty string.
//
// # Representation
//
// Interned strings are represented thus. If the high bit is cleared, then this
// is an index into the stored strings inside of the [Table] that created it.
//
// Otherwise, it is up to five characters drawn from the [LLVM char6 encoding],
// represented in-line using the bits of the ID.
//
// NOTE: The 32-bit length is chosen because it is small. However, if we used a
// 64-bit ID this would allow us to inline 10-byte identifiers. We should
// investigate whether this results in improved memory usage overall.
//
// [LLVM char6 encoding]: https://llvm.org/docs/BitCodeFormat.html#bit-characters
type ID int32

// String implements [fmt.Stringer].
//
// Note that this will not convert the ID back into a string; to do that, you
// must call [Table.Value].
func (id ID) String() string {
	if id == 0 {
		return `intern.ID("")`
	}
	if id < 0 {
		return fmt.Sprintf("intern.ID(%q)", decodeChar6(id, new(inlined)))
	}
	return fmt.Sprintf("intern.ID(%d)", int(id))
}

// GoString implements [fmt.GoStringer].
func (id ID) GoString() string {
	return id.String()
}

// Table is an interning table.
//
// A table can be used to convert strings into [ID]s and back again.
//
// The zero value of Table is empty and ready to use.
type Table struct {
	index sync.Map // [string, atomic.Int32]
	table syncx.Log[string]
	stats atomic.Pointer[stats]
}

// Stats are cache behavior statistics for a [Table].
//
// See [Table.Stats].
type Stats struct {
	Hits    int64 // Times [Table.Intern] returns a previously interned string.
	Misses  int64 // Times [Table.Intern] interns a new string.
	Queries int64 // Times [Table.Query] or [Table.Intern] is called.
	Inlined int64 // Times [Table.Query] or [Table.Intern] processes an inlined string.

	AvgQuery  float64 // Average length of queried strings.
	AvgIntern float64 // Average length of interned strings (excludes inlined strings).
}

// stats contains performance counters for a [Table].
//
// The fields here are carefully designed to minimize the amount of work
// needed to record performance information in Query and Intern.
type stats struct {
	hits    atomic.Int64 // Hits
	total   atomic.Int64 // Hits + Misses
	queries atomic.Int64 // Queries - Inlined
	inlined atomic.Int64 // Inlined

	queryBytes  atomic.Int64 // Total bytes queried.
	internBytes atomic.Int64 // Total bytes interned.
}

// RecordStats sets whether this table records statistics on cache behavior.
//
// Calling RecordStats(true) will reset any records set so far.
func (t *Table) RecordStats(b bool) {
	if b {
		t.stats.Store(new(stats))
	} else {
		t.stats.Store(nil)
	}
}

// Stats returns recorded statistics.
//
// Panics if [Table.RecordStats](true) has not been called.
func (t *Table) Stats() Stats {
	stats := t.stats.Load()
	if stats == nil {
		panic("intern.Table.Stats: must call RecordStats(true)")
	}

	var out Stats

	hits := stats.hits.Load()
	total := stats.total.Load()

	out.Hits = hits
	out.Misses = total - hits

	queries := stats.queries.Load()
	inlined := stats.inlined.Load()
	out.Queries = queries + inlined
	out.Inlined = inlined

	bytes := stats.queryBytes.Load()
	out.AvgQuery = float64(bytes) / float64(out.Queries)

	bytes = stats.internBytes.Load()
	out.AvgIntern = float64(bytes) / float64(total)

	return out
}

// Intern interns the given string into this table.
//
// This function may be called by multiple goroutines concurrently.
func (t *Table) Intern(s string) ID {
	// Fast path for strings that have already been interned. In the common case
	// all strings are interned, so we can take a read lock to avoid needing
	// to trap to the scheduler on concurrent access (all calls to Intern() will
	// still contend mu.readCount, because RLock atomically increments it).
	id, ok := t.Query(s)

	if stats := t.stats.Load(); stats != nil {
		stats.hits.Add(int64(bitsx.Bit(ok)))
		stats.total.Add(1)
		stats.internBytes.Add(int64(len(s) & bitsx.Mask(ok)))
	}

	if !ok {
		id = t.internSlow(s)
	}
	return id
}

// Query will query whether s has already been interned.
//
// If s has never been interned, returns false. This is useful for e.g. querying
// an intern-keyed map using a string: a failed query indicates that the string
// has never been seen before, so searching the map will be futile.
//
// If s is small enough to be inlined in an ID, it is treated as always being
// interned.
func (t *Table) Query(s string) (ID, bool) {
	stats := t.stats.Load()

	if char6, ok := encodeChar6(s); ok {
		if stats != nil {
			stats.inlined.Add(1)
			stats.queryBytes.Add(int64(len(s)))
		}

		// This also handles s == "".
		return char6, true
	}

	v, ok := t.index.Load(s)
	if stats != nil {
		stats.queries.Add(1)
		stats.queryBytes.Add(int64(len(s)))
	}

	if !ok {
		return 0, false
	}

	p := v.(*atomic.Int32) //nolint:errcheck
	if p == nil {
		// This key has been poisoned because we ran out of entries.
		return 0, false
	}
	id := ID(p.Load())
	if id == 0 {
		// Handle the case where this is a mid-insertion.
		return 0, false
	}

	return id, true
}

func (t *Table) internSlow(s string) ID {
	// Intern tables are expected to be long-lived. Avoid holding onto a larger
	// buffer that s is an internal pointer to by cloning it.
	//
	// This is also necessary for the correctness of InternBytes, which aliases
	// a []byte as a string temporarily for querying the intern table.
	s = strings.Clone(s)

	// Pre-convert to `any`, since this triggers an allocation via
	// `runtime.convTstring`.
	key := any(s)

again:
	// Try to become the "leader" which is interning s. Insert a 0, which is
	// "" (never interned), to mark this slot as taken.
	v, loaded := t.index.LoadOrStore(key, new(atomic.Int32))
	p := v.(*atomic.Int32) //nolint:errcheck
	if loaded {
		if p == nil {
			// We ran out of IDs for this key.
			panic(syncx.ErrLogExhausted)
		}

		id := ID(p.Load())
		if id == 0 {
			// Someone *else* is doing the inserting, apparently.
			runtime.Gosched()
			goto again
		}

		// Someone else already inserted, we'de done.
		return id
	}

	// Figure out the next interning ID.
	i, err := t.table.Append(s)
	if err != nil {
		// Poison this key. This will cause any goroutines waiting for interning
		// to complete to also panic.
		t.index.Store(key, (*atomic.Int32)(nil))
		panic(err)
	}

	// Commit the new ID.
	id := ID(i + 1)
	p.Store(int32(id))

	return id
}

// InternBytes interns the given byte string into this table.
//
// This function may be called by multiple goroutines concurrently, but bytes
// must not be modified until this function returns.
func (t *Table) InternBytes(bytes []byte) ID {
	// Intern() will not modify its argument, since it believes that it is a
	// string. It will also clone the string if it needs to write it to the
	// intern table, so it does not hold onto its argument after it returns.
	//
	// Thus, we can simply turn bytes into a string temporarily to pass to
	// Intern.
	return t.Intern(unsafex.StringAlias(bytes))
}

// QueryBytes will query whether bytes has already been interned.
//
// This function may be called by multiple goroutines concurrently, but bytes
// must not be modified until this function returns.
func (t *Table) QueryBytes(bytes []byte) (ID, bool) {
	// See InternBytes's comment.
	return t.Query(unsafex.StringAlias(bytes))
}

// Value converts an [ID] back into its corresponding string.
//
// If id was created by a different [Table], the results are unspecified,
// including potentially a panic.
//
// This function may be called by multiple goroutines concurrently.
func (t *Table) Value(id ID) string {
	// NOTE: this function is carefully written such that Go inlines it into
	// the caller, allowing the result to be promoted to the stack.
	return t.value(id, new(inlined))
}

//go:noinline
func (t *Table) value(id ID, buf *inlined) string {
	if id <= 0 {
		return decodeChar6(id, buf)
	}

	return t.table.Load(int(id) - 1)
}

// Preload takes a pointer to a struct type and initializes [ID]-typed fields
// with statically-specified strings.
//
// Specifically, every exported field whose type is [ID] and which has a struct
// tag "intern" will be set to t.Intern(...) with that tag's value.
//
// Panics if ids is not a pointer to a struct type.
func (t *Table) Preload(ids any) {
	r := reflect.ValueOf(ids).Elem()
	for i := range r.NumField() {
		f := r.Type().Field(i)
		if !f.IsExported() || f.Type != reflect.TypeFor[ID]() {
			continue
		}

		text, ok := f.Tag.Lookup("intern")
		if ok {
			r.Field(i).Set(reflect.ValueOf(t.Intern(text)))
		}
	}
}
