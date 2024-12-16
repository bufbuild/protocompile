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

import (
	"slices"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
)

// DeclBody is the body of a [DeclBody], or the whole contents of a [File]. The
// protocompile AST is very lenient, and allows any declaration to exist anywhere, for the
// benefit of rich diagnostics and refactorings. For example, it is possible to represent an
// "orphaned" field or oneof outside of a message, or an RPC method inside of an enum, and
// so on.
//
// DeclBody implements [Slice], providing access to its declarations.
//
// # Grammar
//
//	DeclBody := `{` DeclAny* `}`
//
// Note that a [File] is simply a DeclBody that is delimited by the bounds of
// the source file, rather than braces.
type DeclBody struct{ declImpl[rawDeclBody] }

type rawDeclBody struct {
	braces token.ID

	// These slices are co-indexed; they are parallelizes to save
	// three bytes per decl (declKind is 1 byte, but decl is 4; if
	// they're stored in AOS format, we waste 3 bytes of padding).
	kinds []DeclKind
	ptrs  []arena.Untyped
}

var (
	_ Inserter[DeclAny] = DeclBody{}
)

// Braces returns this body's surrounding braces, if it has any.
func (d DeclBody) Braces() token.Token {
	return d.raw.braces.In(d.Context())
}

// Span implements [report.Spanner].
func (d DeclBody) Span() report.Span {
	if !d.Braces().Nil() {
		return d.Braces().Span()
	}

	if d.Len() == 0 {
		return report.Span{}
	}

	return report.Join(d.At(0), d.At(d.Len()-1))
}

// Len returns the number of declarations inside of this body.
func (d DeclBody) Len() int {
	return len(d.raw.ptrs)
}

// At returns the nth element of this body.
func (d DeclBody) At(n int) DeclAny {
	return rawDecl{d.raw.ptrs[n], d.raw.kinds[n]}.With(d.Context())
}

// Iter is an iterator over the nodes inside this body.
func (d DeclBody) Iter(yield func(int, DeclAny) bool) {
	for i := range d.raw.kinds {
		if !yield(i, d.At(i)) {
			break
		}
	}
}

// Append appends a new declaration to this body.
func (d DeclBody) Append(value DeclAny) {
	d.Insert(d.Len(), value)
}

// Insert inserts a new declaration at the given index.
func (d DeclBody) Insert(n int, value DeclAny) {
	d.Context().Nodes().panicIfNotOurs(value)

	d.raw.kinds = slices.Insert(d.raw.kinds, n, value.Kind())
	d.raw.ptrs = slices.Insert(d.raw.ptrs, n, value.raw.ptr)
}

// Delete deletes the declaration at the given index.
func (d DeclBody) Delete(n int) {
	d.raw.kinds = slices.Delete(d.raw.kinds, n, n+1)
	d.raw.ptrs = slices.Delete(d.raw.ptrs, n, n+1)
}

func wrapDeclBody(c Context, ptr arena.Pointer[rawDeclBody]) DeclBody {
	return DeclBody{wrapDecl(c, ptr)}
}
