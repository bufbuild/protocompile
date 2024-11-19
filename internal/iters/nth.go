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

// Nth retrieves the nth element of the given iterator, advancing up to n times.
func Nth[I SeqLike[T], T any](iter I, n int) (v T, ok bool) {
	var i int
	iter(func(x T) bool {
		if i == n {
			v = x
			ok = true
			return false
		}

		i++
		return true
	})
	return v, ok
}

// Nth2 retrieves the nth element of the given iterator, advancing up to n times.
func Nth2[I Seq2Like[T, U], T, U any](iter I, n int) (v1 T, v2 U, ok bool) {
	var i int
	iter(func(x T, y U) bool {
		if i == n {
			v1 = x
			v2 = y
			ok = true
			return false
		}

		i++
		return true
	})
	return v1, v2, ok
}
