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

package iterx

import (
	"iter"
)

// Left returns a new iterator that drops the right value of a [iter.Seq2].
func Left[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return Map2to1(seq, func(k K, _ V) K { return k })
}

// Right returns a new iterator that drops the left value of a [iter.Seq2].
func Right[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return Map2to1(seq, func(_ K, v V) V { return v })
}
