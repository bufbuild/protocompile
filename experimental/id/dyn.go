package id

import (
	"fmt"
	"reflect"
)

// Dyn is a generic 64-bit identifier for a value of type T, intended to
// carry additional dynamic type information than an [ID]. It consists of a
// K and an [ID][T]. However, some uses may make use of the whole 64 bits of
// this ID, which can be accessed with [Dyn.Ints].
//
// DynIDs are typed and require a [Context] to be interpreted.
type Dyn[T any, K Kind[K]] uint64

// Kind is a kind type usable in a DynID.
//
// Self should be the type implementing Kind.
type Kind[Self any] interface {
	comparable // Make it not useable as an interface.

	// Decodes the kind by decoding the low and high parts of a [DynID]
	// and setting this value to the result. A zero return value is taken to
	// mean that decoding failed.
	DecodeDynID(lo, hi int32) Self

	// EncodeDynID encodes a new DynID with the given id part.
	EncodeDynID(value int32) (lo, hi int32, ok bool)
}

// NewDyn encodes a new [Dyn] from the given parts.
func NewDyn[T any, K Kind[K]](kind K, id ID[T]) Dyn[T, K] {
	lo, hi, ok := kind.EncodeDynID(int32(id))
	if !ok {
		return 0
	}
	return NewDynFromRaw[T, K](lo, hi)
}

// NewDynFromRaw encodes a new [Dyn] from the given raw parts.
func NewDynFromRaw[T any, K Kind[K]](lo, hi int32) Dyn[T, K] {
	return Dyn[T, K](uint64(uint32(lo)) | (uint64(uint32(hi)) << 32))
}

// IsZero returns whether this is the zero ID.
func (id Dyn[T, K]) IsZero() bool {
	return id == 0
}

// Kind returns the kind of this DynID.
func (id Dyn[T, K]) Kind() K {
	var kind K
	return kind.DecodeDynID(id.Ints())
}

// Value returns the id part of this DynID.
//
// If the resulting ID is not well-formed, returns zero.
func (id Dyn[T, K]) Value() ID[T] {
	var z K
	if id.Kind() == z {
		return 0
	}
	_, v := id.Ints()
	return ID[T](v)
}

// Ints reinterprets this ID as two 32-bit integers.
func (id Dyn[T, K]) Ints() (lo, hi int32) {
	return int32(id), int32(id >> 32)
}

// String implements [fmt.Stringer].
func (id Dyn[T, K]) String() string {
	ty := reflect.TypeFor[T]()
	name := ty.Name()

	if id.IsZero() {
		return name + "(<nil>)"
	}

	a, b := id.Ints()
	var z K
	if k := id.Kind(); k != z {
		if b < 0 {
			return fmt.Sprintf("%s(%v, ^%d)", name, k, ^int32(b))
		}
		return fmt.Sprintf("%s(%v, %d)", name, k, int32(b)-1)
	}

	return fmt.Sprintf("%s(%08x %08x)", name, a, b)
}

// DynValue is the equivalent of [Value] for [Dyn].
//
// Types which are a context/ID pair should be defined as
//
//	type Node DynValue[Node, NodeKind, Context]
type DynValue[T any, K Kind[K], C Constraint] = struct {
	implDynamic[T, K, C]
}

// NewDynValue wraps a dynamic ID with a context.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func NewDynValue[T ~DynValue[T, K, C], K Kind[K], C Constraint](c C, id Dyn[T, K]) T {
	var z C
	if z == c || id.IsZero() {
		return T{}
	}

	return T{
		implDynamic: implDynamic[T, K, C]{
			hasContext: hasContext[C]{c},
			id:         uint64(id),
		},
	}
}

// impl is where we hang the methods associated with Value from.
// These need to be defined in a separate struct, so that we can embed it into
// Value, so that then when named types use Value as their underlying type,
// they pick up those methods.
type implDynamic[T any, K Kind[K], C Constraint] struct {
	hasContext[C]
	id uint64
}

// Kind returns this value's kind.
func (v implDynamic[T, K, C]) Kind() K {
	return v.ID().Kind()
}

// ID returns this value's ID.
func (v implDynamic[T, K, C]) ID() Dyn[T, K] {
	return Dyn[T, K](v.id)
}
