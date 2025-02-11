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

package slicesx

import (
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/iter"
)

// Collect polyfills [slices.Collect].
func Collect[E any](seq iter.Seq[E]) []E {
	return AppendSeq[[]E](nil, seq)
}

// AppendSeq polyfills [slices.AppendSeq].
func AppendSeq[S ~[]E, E any](s S, seq iter.Seq[E]) []E {
	seq(func(v E) bool {
		s = append(s, v)
		return true
	})
	return s
}

// Map is a helper for generating a mapped iterator over a slice, to avoid
// a noisy call to [Values].
func Map[S ~[]E, E, U any](s S, f func(E) U) iter.Seq[U] {
	return iterx.Map(Values(s), f)
}

// Values is a polyfill for [slices.Values].
func Values[S ~[]E, E any](s S) iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}
