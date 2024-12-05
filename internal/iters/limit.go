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
