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

// Package iterx contains extensions to Go's package iter.
package iterx

import (
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile/internal/iter"
)

// Count counts the number of elements in seq that match the given predicate.
//
// If p is nil, it is treated as func(_ T) bool { return true }.
func Count[T any](seq iter.Seq[T]) int {
	var total int
	seq(func(_ T) bool {
		total++
		return true
	})
	return total
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

// Every returns whether every element of an iterator satisfies the given
// predicate. Returns true if seq yields no values.
func Every[T any](seq iter.Seq[T], p func(T) bool) bool {
	all := true
	seq(func(v T) bool {
		all = p(v)
		return all
	})
	return all
}
