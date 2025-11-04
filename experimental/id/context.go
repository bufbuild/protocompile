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
type HasContext[Context comparable] struct {
	context Context
}

// For embedding within this package.
type hasContext[Context comparable] = HasContext[Context]

// WrapContext wraps the context c in a [HasContext].
func WrapContext[Context comparable](c Context) HasContext[Context] {
	return HasContext[Context]{c}
}

// IsZero returns whether this is a zero value.
func (c HasContext[Context]) IsZero() bool {
	var z Context
	return z == c.context
}

// Context returns this value's context.
func (c HasContext[Context]) Context() Context {
	return c.context
}
