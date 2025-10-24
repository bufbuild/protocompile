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
	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Context is where all of the book-keeping for the AST of a particular file is kept.
//
// Virtually all operations inside of package ast involve a Context. However, most of
// the exported types carry their Context with them, so you don't need to worry about
// passing it around.
type Context interface {
	token.Context

	Nodes() *Nodes
}

type withContext = internal.With[Context]

// NewContext creates a fresh context for a particular file.
func NewContext(file *report.File) Context {
	c := new(context)
	c.stream = &token.Stream{
		Context: c,
		File:    file,
	}
	c.nodes = &Nodes{
		Context: c,
	}

	c.Nodes().NewDeclBody(token.Zero) // This is the rawBody for the whole file.
	return c
}

type context struct {
	stream *token.Stream
	nodes  *Nodes
}

func (c *context) Stream() *token.Stream {
	return c.stream
}

func (c *context) Nodes() *Nodes {
	return c.nodes
}
