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

package mapsx

import "iter"

// Collect polyfills [maps.Collect].
func Collect[K comparable, V any](seq iter.Seq2[K, V]) map[K]V {
	return Insert(make(map[K]V), seq)
}

// Insert polyfills [maps.Insert].
func Insert[M ~map[K]V, K comparable, V any](m M, seq iter.Seq2[K, V]) M {
	seq(func(k K, v V) bool {
		m[k] = v
		return true
	})
	return m
}

// CollectSet is like [Collect], but it implicitly fills in each map value
// with a struct{} value.
func CollectSet[K comparable](seq iter.Seq[K]) map[K]struct{} {
	return InsertKeys(make(map[K]struct{}), seq)
}

// InsertKeys is like [Insert], but it implicitly fills in each map value
// with the zero value.
func InsertKeys[M ~map[K]V, K comparable, V any](m M, seq iter.Seq[K]) M {
	seq(func(k K) bool {
		var zero V
		m[k] = zero
		return true
	})
	return m
}
