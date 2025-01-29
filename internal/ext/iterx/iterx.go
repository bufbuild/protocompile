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

// package iterx contains extensions to Go's package iter.
package iterx

import (
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile/internal/iter"
)

// Limit limits a sequence to only yield at most limit times.
func Limit[T any](limit uint, seq iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		seq(func(value T) bool {
			if limit == 0 || !yield(value) {
				return false
			}
			limit--
			return true
		})
	}
}

// First retrieves the first element of an iterator.
func First[T any](seq iter.Seq[T]) (v T, ok bool) {
	seq(func(x T) bool {
		v = x
		ok = true
		return false
	})
	return v, ok
}

// All returns whether every element of an iterator satisfies the given
// predicate. Returns true if seq yields no values.
func All[T any](seq iter.Seq[T], p func(T) bool) bool {
	all := true
	seq(func(v T) bool {
		all = p(v)
		return all
	})
	return all
}

// Map returns a new iterator applying f to each element of seq.
func Map[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return FilterMap(seq, func(v T) (U, bool) { return f(v), true })
}

// Filter returns a new iterator that only includes values satisfying p.
func Filter[T any](seq iter.Seq[T], p func(T) bool) iter.Seq[T] {
	return FilterMap(seq, func(v T) (T, bool) { return v, p(v) })
}

// FilterMap combines the operations of [Map] and [Filter].
func FilterMap[T, U any](seq iter.Seq[T], f func(T) (U, bool)) iter.Seq[U] {
	return func(yield func(U) bool) {
		seq(func(v T) bool {
			v2, ok := f(v)
			return !ok || yield(v2)
		})
	}
}

// Join is like [strings.Join], but works on an iterator. Elements are
// stringified as if by [fmt.Print].
func Join[T any](seq iter.Seq[T], sep string) string {
	var out strings.Builder
	first := true
	seq(func(v T) bool {
		if !first {
			out.WriteString(sep)
		}
		first = false

		fmt.Fprint(&out, v)
		return true
	})
	return out.String()
}
