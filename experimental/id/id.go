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
//	type Node Value[Context, rawNode]
type Value[T any, C Constraint, Raw any] = struct {
	impl[T, C, Raw]
}

// Get gets the value of type T with the given ID from c.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func Get[T ~Value[T, C, Raw], C Constraint, Raw any](c C, id ID[T]) T {
	var z C
	if z == c || id.IsZero() {
		return T{}
	}

	raw := c.FromID(int32(id), (*Raw)(nil))
	if raw == nil {
		return T{}
	}

	return T{
		impl: impl[T, C, Raw]{
			context: c,
			raw:     raw.(Raw),
			id:      int32(id),
		},
	}
}

// FromRaw constructs a new T using the given raw value.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func FromRaw[T ~Value[T, C, Raw], C Constraint, Raw any](c C, id ID[T], raw Raw) T {
	var z C
	if z == c || id.IsZero() {
		return T{}
	}

	return T{
		impl: impl[T, C, Raw]{
			context: c,
			raw:     raw,
			id:      int32(id),
		},
	}
}

// Context is an "ID context", which allows converting between IDs and the
// underlying values they represent.
//
// Users of this package should not call the Context methods directly.
type Context interface {
	// FromID gets the value for a given ID.
	//
	// The requested type is passed in via the parameter want, which will be
	// a nil pointer to a value of the desired type. E.g., if the desired type
	// is *int, want will be (**int)(nil).
	FromID(id int32, want any) any
}

// Constraint is a version of [Context] that can be used as a constraint.
type Constraint interface {
	comparable
	Context
}

// impl is where we hang the methods associated with Value from.
// These need to be defined in a separate struct, so that we can embed it into
// Value, so that then when named types use Value as their underlying type,
// they pick up those methods.
type impl[T any, C Constraint, Raw any] struct {
	context C
	raw     Raw
	id      int32
}

// IsZero returns whether this is a zero value.
func (v impl[T, C, R]) IsZero() bool {
	var z C
	return z == v.context
}

// Context returns this value's context.
func (v impl[T, C, R]) Context() C {
	return v.context
}

// Raw returns the wrapped raw value.
func (v impl[T, C, R]) Raw() R {
	return v.raw
}

// ID returns this value's ID.
func (v impl[T, C, R]) ID() ID[T] {
	return ID[T](v.id)
}
