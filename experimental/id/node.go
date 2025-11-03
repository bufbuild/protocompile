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

// Node is a raw node value with an associated context and ID.
//
// Types which are a context/ID pair should be defined as
//
//	type MyNode id.Node[MyNode, Context, *rawMyNode]
type Node[T any, C Constraint, Raw any] = struct {
	impl[T, C, Raw]
}

// DynNode is the equivalent of [Node] for [Dyn].
//
// Types which are a context/ID pair should be defined as
//
//	type MyNode DynNode[MyNode, MyNodeKind, Context]
type DynNode[T any, K Kind[K], C Constraint] = struct {
	implDynamic[T, K, C]
}

// Wrap gets the value of type T with the given ID from c.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func Wrap[T ~Node[T, C, Raw], C Constraint, Raw any](c C, id ID[T]) T {
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
			raw:        raw.(Raw), //nolint:errcheck
			id:         int32(id),
		},
	}
}

// WrapDyn wraps a dynamic ID with a context.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func WrapDyn[T ~DynNode[T, K, C], K Kind[K], C Constraint](c C, id Dyn[T, K]) T {
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

// WrapRaw constructs a new T using the given raw value.
//
// If c or id is its zero value (e.g. nil), returns a zero value.
func WrapRaw[T ~Node[T, C, Raw], C Constraint, Raw any](c C, id ID[T], raw Raw) T {
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

// See impl.
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
