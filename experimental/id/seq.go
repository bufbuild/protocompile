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

package id

import (
	"github.com/bufbuild/protocompile/experimental/seq"
)

// Seq is an array of nodes which uses a compressed representation.
//
// A zero value is empty and ready to use.
type Seq[T ~Node[T, C, Raw], C Constraint, Raw any] struct {
	ids []ID[T]
}

// Inserter returns a [seq.Inserter] wrapping this [Seq].
func (s *Seq[T, C, Raw]) Inserter(context C) seq.Inserter[T] {
	var ids *[]ID[T]
	if s != nil {
		ids = &s.ids
	}

	return seq.NewSliceInserter(
		ids,
		func(_ int, p ID[T]) T {
			return Wrap(context, p)
		},
		func(_ int, t T) ID[T] {
			return Node[T, C, Raw](t).ID()
		},
	)
}

// DynSeq is an array of dynamic nodes which uses a compressed representation.
//
// A zero value is empty and ready to use.
type DynSeq[T ~DynNode[T, K, C], K Kind[K], C Constraint] struct {
	kinds []K
	ids   []ID[T]
}

// Inserter returns a [seq.Inserter] wrapping this [DynSeq].
func (s *DynSeq[T, K, C]) Inserter(context C) seq.Inserter[T] {
	var kinds *[]K
	var ids *[]ID[T]
	if s != nil {
		kinds = &s.kinds
		ids = &s.ids
	}

	return seq.NewSliceInserter2(
		kinds,
		ids,
		func(_ int, k K, p ID[T]) T {
			return WrapDyn(context, NewDyn(k, p))
		},
		func(_ int, t T) (K, ID[T]) {
			id := DynNode[T, K, C](t).ID()
			return id.Kind(), id.Value()
		},
	)
}
