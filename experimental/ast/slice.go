// Copyright 2020-2024 Buf Technologies, Inc.
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

package ast

import "github.com/bufbuild/protocompile/experimental/token"

// Slice is a type that offers a Go slice's interface, but read-only.
//
// This is used to provide a consistent interface to various AST nodes that
// contain a variable number of "something", but the actual backing array
// is some compressed representation.
type Slice[T any] interface {
	// Len returns this slice's length.
	Len() int

	// At returns the nth value of this slice.
	//
	// Panics if n is negative or n >= Len().
	At(n int) T

	// Iter is an iterator over the slice.
	Iter(yield func(int, T) bool)
}

// Inserter is a [Slice] that allows insertion and removal of elements at specific
// indices.
//
// Insertion/removal behavior while calling Iter() is unspecified.
type Inserter[T any] interface {
	Slice[T]

	// Append appends a value to this sequence.
	Append(value T)

	// Inserts a value at the index n, shifting things around as needed.
	//
	// Panics if n > Len(). Insert(Len(), x) will append.
	Insert(n int, value T)

	// Delete deletes T from the index n.
	//
	// Panics if n >= Len().
	Delete(n int)
}

// Commas is like [Slice], but it's for a comma-delimited list of some kind.
//
// This makes it easy to work with the list as though it's a slice, while also
// allowing access to the commas.
type Commas[T any] interface {
	Inserter[T]

	// Comma is like [Slice.At] but returns the comma that follows the nth
	// element.
	//
	// May be [token.Zero], either because it's the last element
	// (a common situation where there is no comma) or it was added with
	// Insert() rather than InsertComma().
	Comma(n int) token.Token

	// InsertComma is like Append, but includes an explicit comma.
	AppendComma(value T, comma token.Token)

	// InsertComma is like Insert, but includes an explicit comma.
	InsertComma(n int, value T, comma token.Token)
}

type withComma[T any] struct {
	Value T
	Comma token.ID
}
