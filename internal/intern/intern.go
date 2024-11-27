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

// package intern provides an interning table abstraction to optimize symbol
// resolution.
package intern

import (
	"fmt"
	"strings"
	"sync"
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
	if char6, ok := encodeChar6(s); ok {
		return char6
	}

	// Fast path for strings that have already been interned. In the common case
	// all strings are interned, so we can take a read lock to avoid needing
	// to trap to the scheduler on concurrent access (all calls to Intern() will
	// still contend mu.readCount, because RLock atomically increments it).
	t.mu.RLock()
	id, ok := t.index[s]
	t.mu.RUnlock()
	if ok {
		// We never delete from this map, so if we see ok here, that cannot be
		// changed by another goroutine.
		return id
	}

	// Intern tables are expected to be long-lived. Avoid holding onto a larger
	// buffer that s is an internal pointer to by cloning it.
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
	id = ID(len(t.table))
	if id < 0 {
		panic(fmt.Sprintf("internal/intern: %d interning IDs exhausted", len(t.table)))
	}

	if t.index == nil {
		t.index = make(map[string]ID)
	}
	t.index[s] = id

	return id
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

func (t *Table) getSlow(id ID) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.table[int(id)-1]
}
