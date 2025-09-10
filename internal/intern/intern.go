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
	"strings"
	"sync"

	"github.com/bufbuild/protocompile/internal/ext/mapsx"
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
		return fmt.Sprintf("intern.ID(%q)", decodeChar6(id))
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
	mu    sync.RWMutex
	index map[string]ID
	table []string
}

// Intern interns the given string into this table.
//
// This function may be called by multiple goroutines concurrently.
func (t *Table) Intern(s string) ID {
	// Fast path for strings that have already been interned. In the common case
	// all strings are interned, so we can take a read lock to avoid needing
	// to trap to the scheduler on concurrent access (all calls to Intern() will
	// still contend mu.readCount, because RLock atomically increments it).
	if id, ok := t.Query(s); ok {
		return id
	}

	// Outline the fallback for when we haven't interned, to promote inlining
	// of Intern().
	return t.internSlow(s)
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
	if char6, ok := encodeChar6(s); ok {
		// This also handles s == "".
		return char6, true
	}

	t.mu.RLock()
	id, ok := t.index[s]
	t.mu.RUnlock()

	return id, ok
}

func (t *Table) internSlow(s string) ID {
	// Intern tables are expected to be long-lived. Avoid holding onto a larger
	// buffer that s is an internal pointer to by cloning it.
	//
	// This is also necessary for the correctness of InternBytes, which aliases
	// a []byte as a string temporarily for querying the intern table.
	s = strings.Clone(s)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if someone raced us to intern this string. We have to check again
	// because in the unsynchronized section between RUnlock and Lock, another
	// goroutine might have successfully interned s.
	//
	// TODO: We can reduce the number of map hits if we switch to a different
	// Map implementation that provides an upsert primitive.
	if id, ok := t.index[s]; ok {
		return id
	}

	// As of here, we have unique ownership of the table, and s has not been
	// inserted yet.

	t.table = append(t.table, s)

	// The first ID will have value 1. ID 0 is reserved for "".
	id := ID(len(t.table))
	if id < 0 {
		panic(fmt.Sprintf("internal/intern: %d interning IDs exhausted", len(t.table)))
	}

	if t.index == nil {
		t.index = make(map[string]ID)
	}
	t.index[s] = id

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
	if id == 0 {
		return ""
	}

	if id < 0 {
		return decodeChar6(id)
	}

	// The locking part of Get is outlined to promote inlining of the two
	// fast paths above. This in turn allows decodeChar6 to be inlined, which
	// allows the returned string to be stack-promoted.
	return t.getSlow(id)
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

func (t *Table) getSlow(id ID) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.table[int(id)-1]
}

// Set is a set of intern IDs.
type Set map[ID]struct{}

// ContainsID returns whether s contains the given ID.
func (s Set) ContainsID(id ID) bool {
	_, ok := s[id]
	return ok
}

// Contains returns whether s contains the given string.
func (s Set) Contains(table *Table, key string) bool {
	k, ok := table.Query(key)
	if !ok {
		return false
	}
	_, ok = s[k]
	return ok
}

// AddID adds an ID to s, and returns whether it was added.
func (s Set) AddID(id ID) (inserted bool) {
	return mapsx.AddZero(s, id)
}

// Add adds a string to s, and returns whether it was added.
func (s Set) Add(table *Table, key string) (inserted bool) {
	k := table.Intern(key)
	_, ok := s[k]
	if !ok {
		s[k] = struct{}{}
	}
	return !ok
}

// Map is a map keyed by intern IDs.
type Map[T any] map[ID]T

// Get returns the value that key maps to.
func (m Map[T]) Get(table *Table, key string) (T, bool) {
	k, ok := table.Query(key)
	if !ok {
		var z T
		return z, false
	}
	v, ok := m[k]
	return v, ok
}

// AddID adds an ID to m, and returns whether it was added.
func (m Map[T]) AddID(id ID, v T) (mapped T, inserted bool) {
	return mapsx.Add(m, id, v)
}

// Add adds a string to m, and returns whether it was added.
func (m Map[T]) Add(table *Table, key string, v T) (mapped T, inserted bool) {
	return m.AddID(table.Intern(key), v)
}
