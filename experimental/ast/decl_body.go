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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// DeclBody is the body of a [DeclBody], or the whole contents of a [File]. The
// protocompile AST is very lenient, and allows any declaration to exist anywhere, for the
// benefit of rich diagnostics and refactorings. For example, it is possible to represent an
// "orphaned" field or oneof outside of a message, or an RPC method inside of an enum, and
// so on.
//
// # Grammar
//
//	DeclBody := `{` DeclAny* `}`
//
// Note that a [File] is simply a DeclBody that is delimited by the bounds of
// the source file, rather than braces.
type DeclBody id.Node[DeclBody, *File, *rawDeclBody]

// HasBody is an AST node that contains a [Body].
//
// [File], [DeclBody], and [DeclDef] all implement this interface.
type HasBody interface {
	source.Spanner

	Body() DeclBody
}

type rawDeclBody struct {
	braces token.ID

	// These slices are co-indexed; they are parallelizes to save
	// three bytes per decl (declKind is 1 byte, but decl is 4; if
	// they're stored in AOS format, we waste 3 bytes of padding).
	kinds []DeclKind
	ptrs  []id.ID[DeclAny]
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclBody) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindBody, id.ID[DeclAny](d.ID())))
}

// Braces returns this body's surrounding braces, if it has any.
func (d DeclBody) Braces() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().braces)
}

// Span implements [source.Spanner].
func (d DeclBody) Span() source.Span {
	decls := d.Decls()
	switch {
	case d.IsZero():
		return source.Span{}
	case !d.Braces().IsZero():
		return d.Braces().Span()
	case decls.Len() == 0:
		return source.Span{}
	default:
		return source.Join(decls.At(0), decls.At(decls.Len()-1))
	}
}

// Body implements [HasBody].
func (d DeclBody) Body() DeclBody {
	return d
}

// Decls returns a [seq.Inserter] over the declarations in this body.
func (d DeclBody) Decls() seq.Inserter[DeclAny] {
	if d.IsZero() {
		return seq.SliceInserter2[DeclAny, DeclKind, id.ID[DeclAny]]{}
	}
	return seq.NewSliceInserter2(
		&d.Raw().kinds,
		&d.Raw().ptrs,
		func(_ int, k DeclKind, p id.ID[DeclAny]) DeclAny {
			return id.WrapDyn(d.Context(), id.NewDyn(k, p))
		},
		func(_ int, d DeclAny) (DeclKind, id.ID[DeclAny]) {
			d.Context().Nodes().panicIfNotOurs(d)
			return d.ID().Kind(), d.ID().Value()
		},
	)
}
