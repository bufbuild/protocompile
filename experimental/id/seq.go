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
