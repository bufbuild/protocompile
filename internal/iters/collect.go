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

import "github.com/bufbuild/protocompile/internal/iter"

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
