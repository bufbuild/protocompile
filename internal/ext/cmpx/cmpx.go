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

// package cmpx contains extensions to Go's package cmp.
package cmpx

import "cmp"

// Result is the type returned by an [Ordering], and in particular
// [cmp.Compare].
type Result = int

const (
	// [cmp.Compare] guarantees these return values.
	Less    Result = -1
	Equal   Result = 0
	Greater Result = 1
)

// Ordering is an ordering for the type T, which is any function with the same
// signature as [Compare].
type Ordering[T any] func(T, T) Result

// Key returns an ordering for T according to a key function, which must return
// a [cmp.Ordered] value.
func Key[T any, U cmp.Ordered](key func(T) U) Ordering[T] {
	return func(a, b T) Result { return cmp.Compare(key(a), key(b)) }
}

// Join returns an ordering for T which returns the first of cmps returns a
// non-[Equal] value.
func Join[T any](cmps ...Ordering[T]) Ordering[T] {
	return func(a, b T) Result {
		for _, cmp := range cmps {
			if n := cmp(a, b); n != Equal {
				return n
			}
		}
		return Equal
	}
}
