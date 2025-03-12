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
	"iter"
	"strings"
)

// Count counts the number of elements in seq.
func Count[T any](seq iter.Seq[T]) int {
	var total int
	for range seq {
		total++
	}
	return total
}

// Count2 counts the number of elements in seq.
func Count2[T, U any](seq iter.Seq2[T, U]) int {
	var total int
	for range seq {
		total++
	}
	return total
}

// Join is like [strings.Join], but works on an iterator. Elements are
// stringified as if by [fmt.Print].
func Join[T any](seq iter.Seq[T], sep string) string {
	var out strings.Builder
	for i, v := range Enumerate(seq) {
		if i > 0 {
			out.WriteString(sep)
		}
		fmt.Fprint(&out, v)
	}
	return out.String()
}

// Every returns whether every element of an iterator satisfies the given
// predicate. Returns true if seq yields no values.
func Every[T any](seq iter.Seq[T], p func(T) bool) bool {
	for v := range seq {
		if !p(v) {
			return false
		}
	}
	return true
}

// Exhaust runs an iterator to completion for its side-effects.
//
// This mostly exists because there is a noisy lint that incorrectly thinks all
// range loops are side-effect free, and because gofmt won't format
// "for range iter {}" on one line.
func Exhaust[T any](seq iter.Seq[T]) {
	//nolint:revive // Empty block has side-effects.
	for range seq {
	}
}
