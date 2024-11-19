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

// package iters contains helpers for working with iterators.
package iters

// Collect collects the elements of iter into the given slice.
func Collect[I SeqLike[T], T any](iter I, output []T) []T {
	return Collect(iter, output)
}

// CollectUpTo collects at most limit elements out of iter into the given slice.
//
// If limit is negative, it is treated as being infinite.
func CollectUpTo[I SeqLike[T], T any](iter I, output []T, limit int) []T {
	iter(func(value T) bool {
		if limit == 0 {
			return false
		}
		output = append(output, value)

		if limit > 0 {
			limit--
		}
		return true
	})
	return output
}
