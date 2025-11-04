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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
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

type withContext = id.HasContext[Context]

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

func (c *context) FromID(id uint64, want any) any {
	switch want.(type) {
	case **rawDeclBody:
		return c.nodes.decls.bodies.Deref(arena.Pointer[rawDeclBody](id))
	case **rawDeclDef:
		return c.nodes.decls.defs.Deref(arena.Pointer[rawDeclDef](id))
	case **rawDeclEmpty:
		return c.nodes.decls.empties.Deref(arena.Pointer[rawDeclEmpty](id))
	case **rawDeclImport:
		return c.nodes.decls.imports.Deref(arena.Pointer[rawDeclImport](id))
	case **rawDeclPackage:
		return c.nodes.decls.packages.Deref(arena.Pointer[rawDeclPackage](id))
	case **rawDeclRange:
		return c.nodes.decls.ranges.Deref(arena.Pointer[rawDeclRange](id))
	case **rawDeclSyntax:
		return c.nodes.decls.syntaxes.Deref(arena.Pointer[rawDeclSyntax](id))

	case **rawExprError:
		return c.nodes.exprs.errors.Deref(arena.Pointer[rawExprError](id))
	case **rawExprArray:
		return c.nodes.exprs.arrays.Deref(arena.Pointer[rawExprArray](id))
	case **rawExprDict:
		return c.nodes.exprs.dicts.Deref(arena.Pointer[rawExprDict](id))
	case **rawExprField:
		return c.nodes.exprs.fields.Deref(arena.Pointer[rawExprField](id))
	case **rawExprPrefixed:
		return c.nodes.exprs.prefixes.Deref(arena.Pointer[rawExprPrefixed](id))
	case **rawExprRange:
		return c.nodes.exprs.ranges.Deref(arena.Pointer[rawExprRange](id))

	case **rawTypeError:
		return c.nodes.types.errors.Deref(arena.Pointer[rawTypeError](id))
	case **rawTypeGeneric:
		return c.nodes.types.generics.Deref(arena.Pointer[rawTypeGeneric](id))
	case **rawTypePrefixed:
		return c.nodes.types.prefixes.Deref(arena.Pointer[rawTypePrefixed](id))

	case **rawCompactOptions:
		return c.nodes.options.Deref(arena.Pointer[rawCompactOptions](id))

	default:
		return c.stream.FromID(id, want)
	}
}
