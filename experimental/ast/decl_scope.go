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

	"github.com/bufbuild/protocompile/internal/arena"
)

// DeclScope is the body of a [DeclDef], or the whole contents of a [File]. The
// protocompile AST is very lenient, and allows any declaration to exist anywhere, for the
// benefit of rich diagnostics and refactorings. For example, it is possible to represent an
// "orphaned" field or oneof outside of a message, or an RPC method inside of an enum, and
// so on.
//
// DeclScope implements [Slice], providing access to its declarations.
type DeclScope struct {
	withContext

	ptr arena.Untyped
	raw *rawDeclScope
}

type rawDeclScope struct {
	braces rawToken

	// These slices are co-indexed; they are parallelizes to save
	// three bytes per decl (declKind is 1 byte, but decl is 4; if
	// they're stored in AOS format, we waste 3 bytes of padding).
	kinds []declKind
	ptrs  []arena.Untyped
}

var (
	_ Decl           = DeclScope{}
	_ Inserter[Decl] = DeclScope{}
)

// Braces returns this body's surrounding braces, if it has any.
func (d DeclScope) Braces() Token {
	return d.raw.braces.With(d)
}

// Span implements [Spanner] for Body.
func (d DeclScope) Span() Span {
	if !d.Braces().Nil() {
		return d.Braces().Span()
	}

	if d.Len() == 0 {
		return Span{}
	}

	return JoinSpans(d.At(0), d.At(d.Len()-1))
}

// Len returns the number of declarations inside of this body.
func (d DeclScope) Len() int {
	return len(d.raw.ptrs)
}

// At returns the nth element of this body.
func (d DeclScope) At(n int) Decl {
	return d.raw.kinds[n].reify().with(d.Context(), d.raw.ptrs[n])
}

// Iter is an iterator over the nodes inside this body.
func (d DeclScope) Iter(yield func(int, Decl) bool) {
	for i := range d.raw.kinds {
		if !yield(i, d.At(i)) {
			break
		}
	}
}

// Append appends a new declaration to this body.
func (d DeclScope) Append(value Decl) {
	d.Insert(d.Len(), value)
}

// Insert inserts a new declaration at the given index.
func (d DeclScope) Insert(n int, value Decl) {
	d.Context().panicIfNotOurs(value)

	d.raw.kinds = slices.Insert(d.raw.kinds, n, value.declKind())
	d.raw.ptrs = slices.Insert(d.raw.ptrs, n, value.declIndex())
}

// Delete deletes the declaration at the given index.
func (d DeclScope) Delete(n int) {
	d.raw.kinds = slices.Delete(d.raw.kinds, n, n+1)
	d.raw.ptrs = slices.Delete(d.raw.ptrs, n, n+1)
}

func (d DeclScope) declIndex() arena.Untyped {
	return d.ptr
}

// Decls returns an iterator over the nodes within a body of a particular type.
func Decls[T Decl](d DeclScope) func(func(int, T) bool) {
	return func(yield func(int, T) bool) {
		var idx int
		d.Iter(func(_ int, decl Decl) bool {
			if actual, ok := decl.(T); ok {
				if !yield(idx, actual) {
					return false
				}
				idx++
			}
			return true
		})
	}
}

func (DeclScope) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclScope{withContext{ctx}, ptr, ctx.decls.bodies.At(ptr)}
}
