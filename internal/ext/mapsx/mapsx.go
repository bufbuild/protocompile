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

// package mapsx contains extensions to Go's package maps.
package mapsx

// KeySet returns a copy of m, with its values replaced with empty structs.
func KeySet[M ~map[K]V, K comparable, V any](m M) map[K]struct{} {
	// return CollectSet(Keys(m))
	// Instead of going through an iterator, inline the loop so that
	// we can preallocate and avoid rehashes.
	keys := make(map[K]struct{}, len(m))
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

// Contains is a shorthand for _, ok := m[k] that allows it to be used in
// expression position.
func Contains[M ~map[K]V, K comparable, V any](m M, k K) bool {
	_, ok := m[k]
	return ok
}

// Add inserts k into the map if it is not present. Returns whether insertion
// occurred, and the value that k maps to in the map.
func Add[M ~map[K]V, K comparable, V any](m M, k K, v V) (mapped V, inserted bool) {
	if v, ok := m[k]; ok {
		return v, false
	}
	m[k] = v
	return v, true
}

// AddZero inserts k into the map if it is not present, using the zero value of
// V as the value. Returns whether insertion occurred.
func AddZero[M ~map[K]V, K comparable, V any](m M, k K) (inserted bool) {
	var z V
	_, inserted = Add(m, k, z)
	return inserted
}

// Append appends the given value to the slice in the entry for the given key.
func Append[M ~map[K][]V, K comparable, V any](m M, k K, v V) {
	m[k] = append(m[k], v)
}
