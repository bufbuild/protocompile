// Package id provides generic utilities for working with node IDs.
package id

import (
	"fmt"
	"reflect"
)

// ID is a generic 32-bit identifier for a value of type T. The zero value is
// reserved as a sentinel "no value" ID.
//
// IDs are typed and require a [Context] to be interpreted.
type ID[T any] int32

// IsZero returns whether this is the zero ID.
func (id ID[T]) IsZero() bool {
	return id == 0
}

// String implements [fmt.Stringer].
func (id ID[T]) String() string {
	ty := reflect.TypeFor[T]()
	name := ty.Name()

	if id.IsZero() {
		return name + "(<nil>)"
	}
	if id < 0 {
		return fmt.Sprintf("%s(^%d)", name, ^int32(id))
	}
	return fmt.Sprintf("%s(%d)", name, int32(id)-1)
}

// Value is a raw value with an associated context and ID.
//
// Types which are a context/ID pair should be defined as
//
//	type Node Value[Node, Context, rawNode]
type Value[T any, C Constraint, Raw any] = struct {
	impl[T, C, Raw]
}

// NewValue gets the value of type T with the given ID from c.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func NewValue[T ~Value[T, C, Raw], C Constraint, Raw any](c C, id ID[T]) T {
	var z C
	if z == c || id.IsZero() {
		return T{}
	}

	raw := c.FromID(uint64(uint32(id)), (*Raw)(nil))
	if raw == nil {
		return T{}
	}

	return T{
		impl: impl[T, C, Raw]{
			hasContext: hasContext[C]{c},
			raw:        raw.(Raw),
			id:         int32(id),
		},
	}
}

// NewValueFromRaw constructs a new T using the given raw value.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func NewValueFromRaw[T ~Value[T, C, Raw], C Constraint, Raw any](c C, id ID[T], raw Raw) T {
	var z C
	if z == c || id.IsZero() {
		return T{}
	}

	return T{
		impl: impl[T, C, Raw]{
			hasContext: hasContext[C]{c},
			raw:        raw,
			id:         int32(id),
		},
	}
}

// impl is where we hang the methods associated with Value from.
// These need to be defined in a separate struct, so that we can embed it into
// Value, so that then when named types use Value as their underlying type,
// they pick up those methods.
type impl[T any, C Constraint, Raw any] struct {
	hasContext[C]
	raw Raw
	id  int32
}

// Raw returns the wrapped raw value.
func (v impl[T, C, R]) Raw() R {
	return v.raw
}

// ID returns this value's ID.
func (v impl[T, C, R]) ID() ID[T] {
	return ID[T](v.id)
}
