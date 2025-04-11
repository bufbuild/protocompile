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

// package cmpx contains extensions to Go's package cmp.
package cmpx

import (
	"cmp"
	"fmt"
	"math"
	"reflect"
)

// Result is the type returned by an [Ordering], and in particular
// [cmp.Compare].
type Result = int

const (
	// [cmp.Compare] guarantees these return values.
	Less    Result = -1
	Equal   Result = 0
	Greater Result = 1
)

// Ordered is like [cmp.Ordered], but includes additional types.
type Ordered interface {
	~bool | cmp.Ordered
}

// Ordering is an ordering for the type T, which is any function with the same
// signature as [Compare].
type Ordering[T any] func(T, T) Result

// Key returns an ordering for T according to a key function, which must return
// a [cmp.Ordered] value.
func Key[T any, U cmp.Ordered](key func(T) U) Ordering[T] {
	return func(a, b T) Result { return cmp.Compare(key(a), key(b)) }
}

// Join returns an ordering for T which returns the first of cmps returns a
// non-[Equal] value.
func Join[T any](cmps ...Ordering[T]) Ordering[T] {
	return func(a, b T) Result {
		for _, cmp := range cmps {
			if n := cmp(a, b); n != Equal {
				return n
			}
		}
		return Equal
	}
}

// Map is like [Join], but it maps the inputs with the given function first.
func Map[T any, U any](f func(T) U, cmps ...Ordering[U]) Ordering[T] {
	return func(x, y T) Result {
		a, b := f(x), f(y)

		for _, cmp := range cmps {
			if n := cmp(a, b); n != Equal {
				return n
			}
		}
		return Equal
	}
}

// Reverse returns an ordering which is the reverse of cmp.
func Reverse[T any](cmp Ordering[T]) Ordering[T] {
	return func(a, b T) Result { return -cmp(a, b) }
}

// Bool compares two bools, where false < true.
//
// This works around a bug where bool does not satisfy [cmp.Ordered].
func Bool[B ~bool](a, b B) Result {
	var ai, bi byte
	if a {
		ai = 1
	}
	if b {
		bi = 1
	}
	return cmp.Compare(ai, bi)
}

// Any compares any two [cmp.Ordered] types, according to the following criteria:
//
//  1. any(nil) is least of all.
//
//  2. If the values are not mutually comparable, their [reflect.Kind]s are
//     compared.
//
//  3. If either value is not of a [cmp.Ordered] type, this function panics.
//
//  4. Otherwise, the arguments are compared as-if by [cmp.Compare].
//
// For the purposes of this function, bool is treated as satisfying [cmp.Compare].
func Any(a, b any) Result {
	if a == nil || b == nil {
		return Bool(a != nil, b != nil)
	}

	ra := reflect.ValueOf(a)
	rb := reflect.ValueOf(b)

	type kind int
	const (
		kBool kind = 1 << iota
		kInt
		kUint
		kFloat
		kString
	)

	which := func(r reflect.Value) kind {
		switch r.Kind() {
		case reflect.Bool:
			return kBool
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return kInt
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Uintptr:
			return kUint
		case reflect.Float32, reflect.Float64:
			return kFloat
		case reflect.String:
			return kString
		default:
			panic(fmt.Sprintf("cmpx.Any: incomparable value %v (type %[1]T)", r.Interface()))
		}
	}

	//nolint:revive // Recommends removing some else {} branches that make the code less symmetric
	switch which(ra) | which(rb) {
	case kBool:
		return Bool(ra.Bool(), rb.Bool())

	case kInt:
		return cmp.Compare(ra.Int(), rb.Int())

	case kUint:
		return cmp.Compare(ra.Uint(), rb.Uint())

	case kInt | kUint:
		if rb.CanUint() {
			v := rb.Uint()
			if v > math.MaxInt64 {
				return Less
			}
			return cmp.Compare(ra.Int(), int64(v))
		} else {
			v := ra.Uint()
			if v > math.MaxInt64 {
				return Greater
			}
			return cmp.Compare(int64(v), rb.Int())
		}

	case kFloat:
		return cmp.Compare(ra.Float(), rb.Float())

	case kFloat | kInt:
		if ra.CanFloat() {
			return cmp.Compare(ra.Float(), float64(rb.Int()))
		} else {
			return cmp.Compare(float64(ra.Int()), rb.Float())
		}

	case kFloat | kUint:
		if ra.CanFloat() {
			return cmp.Compare(ra.Float(), float64(rb.Uint()))
		} else {
			return cmp.Compare(float64(ra.Uint()), rb.Float())
		}

	case kString:
		return cmp.Compare(ra.String(), rb.String())

	default:
		return cmp.Compare(ra.Kind(), rb.Kind())
	}
}
