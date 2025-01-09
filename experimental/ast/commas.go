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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
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

type commas[T any, Raw unsafex.Int] struct {
	seq.InserterWrapper2[T, Raw, token.ID, *slicesx.Inline[Raw], *slicesx.Inline[token.ID]]
	ctx Context
}

func (c commas[T, _]) Comma(n int) token.Token {
	return c.InserterWrapper2.Slice2.At(n).In(c.ctx)
}

func (c commas[T, _]) AppendComma(value T, comma token.Token) {
	c.InsertComma(c.Len(), value, comma)
}

func (c commas[T, _]) InsertComma(n int, value T, comma token.Token) {
	c.ctx.Nodes().panicIfNotOurs(comma)
	e1, _ := c.InserterWrapper2.Unwrap(value)
	c.InserterWrapper2.Slice1.Insert(n, e1)
	c.InserterWrapper2.Slice2.Insert(n, comma.ID())
}
