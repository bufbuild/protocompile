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

package intern

import "github.com/bufbuild/protocompile/internal/ext/mapsx"

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
