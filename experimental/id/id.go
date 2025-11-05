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
	"fmt"
	"reflect"
)

// Kind is a kind type usable in a [Dyn].
//
// Self should be the type implementing Kind.
type Kind[Self any] interface {
	comparable // Make it not useable as an interface.

	// Decodes the kind by decoding the low and high parts of a Dyn
	// and setting this value to the result. A zero return value is taken to
	// mean that decoding failed.
	DecodeDynID(lo, hi int32) Self

	// EncodeDynID encodes a new Dyn with the given value part. Returns
	// arguments for [NewDynFromRaw].
	EncodeDynID(value int32) (lo, hi int32, ok bool)
}

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

// Dyn is a generic 64-bit identifier for a value of type T, intended to
// carry additional dynamic type information than an [ID]. It consists of a
// K and an [ID][T]. However, some uses may make use of the whole 64 bits of
// this ID, which can be accessed with [Dyn.Raw].
//
// DynIDs are typed and require a [Context] to be interpreted.
type Dyn[T any, K Kind[K]] uint64

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
	return kind.DecodeDynID(id.Raw())
}

// Value returns the id part of this DynID.
//
// If the resulting ID is not well-formed, returns zero.
func (id Dyn[T, K]) Value() ID[T] {
	var z K
	if id.Kind() == z {
		return 0
	}
	_, v := id.Raw()
	return ID[T](v)
}

// Raw reinterprets this ID as two 32-bit integers.
func (id Dyn[T, K]) Raw() (lo, hi int32) {
	return int32(id), int32(id >> 32)
}

// String implements [fmt.Stringer].
func (id Dyn[T, K]) String() string {
	ty := reflect.TypeFor[T]()
	name := ty.Name()

	if id.IsZero() {
		return name + "(<nil>)"
	}

	a, b := id.Raw()
	var z K
	if k := id.Kind(); k != z {
		if b < 0 {
			return fmt.Sprintf("%s(%v, ^%d)", name, k, ^b)
		}
		return fmt.Sprintf("%s(%v, %d)", name, k, b-1)
	}

	return fmt.Sprintf("%s(%08x %08x)", name, a, b)
}
