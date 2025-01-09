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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
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
type DeclBody struct{ declImpl[rawDeclBody] }

type rawDeclBody struct {
	braces token.ID

	// These slices are co-indexed; they are parallelizes to save
	// three bytes per decl (declKind is 1 byte, but decl is 4; if
	// they're stored in AOS format, we waste 3 bytes of padding).
	kinds slicesx.Inline[DeclKind]
	ptrs  slicesx.Inline[arena.Untyped]
}

// Braces returns this body's surrounding braces, if it has any.
func (d DeclBody) Braces() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.braces.In(d.Context())
}

// Span implements [report.Spanner].
func (d DeclBody) Span() report.Span {
	decls := d.Decls()
	switch {
	case d.IsZero():
		return report.Span{}
	case !d.Braces().IsZero():
		return d.Braces().Span()
	case decls.Len() == 0:
		return report.Span{}
	default:
		return report.Join(decls.At(0), decls.At(decls.Len()-1))
	}
}

// Decls returns a [seq.Inserter] over the declarations in this body.
func (d DeclBody) Decls() seq.Inserter[DeclAny] {
	var (
		kinds *slicesx.Inline[DeclKind]
		ptrs  *slicesx.Inline[arena.Untyped]
	)
	if !d.IsZero() {
		kinds = &d.raw.kinds
		ptrs = &d.raw.ptrs
	}

	// A single return here promotes devirtualization of both the interface
	// and the funcvals within.
	return seq.WrapInserter2(
		kinds, ptrs,
		func(k DeclKind, p arena.Untyped) DeclAny {
			return rawDecl{p, k}.With(d.Context())
		},
		func(d DeclAny) (DeclKind, arena.Untyped) {
			d.Context().Nodes().panicIfNotOurs(d)
			return d.raw.kind, d.raw.ptr
		},
	)
}

func wrapDeclBody(c Context, ptr arena.Pointer[rawDeclBody]) DeclBody {
	return DeclBody{wrapDecl(c, ptr)}
}
