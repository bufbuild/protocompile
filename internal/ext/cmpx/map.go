package cmpx

import (
	"unsafe"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// MapWrapper is wrapper over a map[K]V that is identity comparable, so it can
// be used as a map key. Note that Go maps are implemented as a single pointer
// to an opaque value.
//
// This is the same comparison as comparing maps using reflect.Value.UnsafePointer.
type MapWrapper[K comparable, V any] struct {
	// NOTE: This type has the same layout as a map[K]V.
	_ [0]*map[K]V
	p unsafe.Pointer
}

// WrapMap returns a [MapWrapper] that wraps m.
func NewMapWrapper[K comparable, V any](m map[K]V) MapWrapper[K, V] {
	return unsafex.Bitcast[MapWrapper[K, V]](m)
}

// Nil returns whether this wraps the nil map.
func (m MapWrapper[K, V]) Nil() bool {
	return m.p == nil
}

// Get returns the wrapped map.
func (m MapWrapper[K, V]) Get() map[K]V {
	return unsafex.Bitcast[map[K]V](m)
}
