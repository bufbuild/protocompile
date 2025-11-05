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

package ast

import (
	"slices"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Commas is like [Slice], but it's for a comma-delimited list of some kind.
//
// This makes it easy to work with the list as though it's a slice, while also
// allowing access to the commas.
type Commas[T any] interface {
	seq.Inserter[T]

	// Comma is like [seq.Indexer.At] but returns the comma that follows the nth
	// element.
	//
	// May be [token.Zero], either because it's the last element
	// (a common situation where there is no comma) or it was added with
	// Insert() rather than InsertComma().
	Comma(n int) token.Token

	// AppendComma is like [seq.Append], but includes an explicit comma.
	AppendComma(value T, comma token.Token)

	// InsertComma is like [seq.Inserter.Insert], but includes an explicit comma.
	InsertComma(n int, value T, comma token.Token)
}

type withComma[T any] struct {
	Value T
	Comma token.ID
}

type commas[T, E any] struct {
	seq.SliceInserter[T, withComma[E]]
	file *File
}

func (c commas[T, _]) Comma(n int) token.Token {
	return id.Wrap(c.file.Stream(), (*c.SliceInserter.Slice)[n].Comma)
}

func (c commas[T, _]) AppendComma(value T, comma token.Token) {
	c.InsertComma(c.Len(), value, comma)
}

func (c commas[T, _]) InsertComma(n int, value T, comma token.Token) {
	c.file.Nodes().panicIfNotOurs(comma)
	v := c.SliceInserter.Unwrap(n, value)
	v.Comma = comma.ID()

	*c.Slice = slices.Insert(*c.Slice, n, v)
}
