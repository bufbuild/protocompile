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

package iters

import "github.com/bufbuild/protocompile/internal/iter"

// Partition returns an iterator of subslices of s such that each yielded
// slice is delimited according to delimit. Also yields the starting index of
// the subslice.
//
// In other words, suppose delimit is !=. Then, the slice [a a a b c c] is yielded
// as the subslices [a a a], [b], and [c c c].
//
// Will never yield an empty slice.
//
//nolint:dupword
func Partition[T any](s []T, delimit func(a, b *T) bool) iter.Seq2[int, []T] {
	return func(yield func(int, []T) bool) {
		var start int
		for i := 1; i < len(s); i++ {
			if delimit(&s[i-1], &s[i]) {
				if !yield(start, s[start:i]) {
					return
				}
				start = i
			}
		}
		rest := s[start:]
		if len(rest) > 0 {
			yield(start, rest)
		}
	}
}
