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

import "github.com/bufbuild/protocompile/internal/iter"

// Collect polyfills [maps.Collect].
func Collect[K comparable, V any](seq iter.Seq2[K, V]) map[K]V {
	out := make(map[K]V)
	seq(func(k K, v V) bool {
		out[k] = v
		return true
	})
	return out
}

// CollectSet is like [Collect], but it implicitly fills in each map value
// with a struct{} value.
func CollectSet[K comparable](seq iter.Seq[K]) map[K]struct{} {
	out := make(map[K]struct{})
	seq(func(k K) bool {
		out[k] = struct{}{}
		return true
	})
	return out
}
