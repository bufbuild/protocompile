package id

// Context is an "ID context", which allows converting between IDs and the
// underlying values they represent.
//
// Users of this package should not call the Context methods directly.
type Context interface {
	// FromID gets the value for a given ID.
	//
	// The ID will be passed in as a raw 64-bit value. It is up to the caller
	// to interpret it based on the requested type.
	//
	// The requested type is passed in via the parameter want, which will be
	// a nil pointer to a value of the desired type. E.g., if the desired type
	// is *int, want will be (**int)(nil).
	FromID(id uint64, want any) any
}

// Constraint is a version of [Context] that can be used as a constraint.
type Constraint interface {
	comparable
	Context
}

// HasContext is a helper for adding IsZero and Context methods to a type.
//
// Simply alias it as an unexported type in your package, and embed it into
// types of interest.
type HasContext[C comparable] struct {
	context C
}

// For embedding within this package.
type hasContext[C comparable] = HasContext[C]

// WrapContext wraps the context c in a [HasContext].
func WrapContext[C comparable](c C) HasContext[C] {
	return HasContext[C]{c}
}

// IsZero returns whether this is a zero value.
func (c HasContext[C]) IsZero() bool {
	var z C
	return z == c.context
}

// Context returns this value's context.
func (c HasContext[C]) Context() C {
	return c.context
}
