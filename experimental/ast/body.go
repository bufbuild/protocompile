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

// DeclBody is the body of a [DeclDef], or the whole contents of a [File]. The
// protocompile AST is very lenient, and allows any declaration to exist anywhere, for the
// benefit of rich diagnostics and refactorings. For example, it is possible to represent an
// "orphaned" field or oneof outside of a message, or an RPC method inside of an enum, and
// so on.
//
// DeclBody implements [Slice], providing access to its declarations.
type DeclBody struct {
	withContext

	idx int
	raw *rawDeclBody
}

type rawDeclBody struct {
	braces rawToken

	// These slices are co-indexed; they are parallelizes to save
	// three bytes per decl (declKind is 1 byte, but decl is 4; if
	// they're stored in AOS format, we waste 3 bytes of padding).
	kinds   []declKind
	indices []decl[Decl]
}

var (
	_ Decl           = DeclBody{}
	_ Inserter[Decl] = DeclBody{}
)

// Braces returns this body's surrounding braces, if it has any.
func (d DeclBody) Braces() Token {
	return d.raw.braces.With(d)
}

// Span implements [Spanner] for Body.
func (d DeclBody) Span() Span {
	if !d.Braces().Nil() {
		return d.Braces().Span()
	}

	if d.Len() == 0 {
		return Span{}
	}

	return JoinSpans(d.At(0), d.At(d.Len()-1))
}

// Len returns the number of declarations inside of this body.
func (d DeclBody) Len() int {
	return len(d.raw.indices)
}

// At returns the nth element of this body.
func (d DeclBody) At(n int) Decl {
	return d.raw.kinds[n].reify().with(
		d.Context(), int(d.raw.indices[n]-1))
}

// Iter is an iterator over the nodes inside this body.
func (d DeclBody) Iter(yield func(int, Decl) bool) {
	for i := range d.raw.kinds {
		if !yield(i, d.At(i)) {
			break
		}
	}
}

// Append appends a new declaration to this body.
func (d DeclBody) Append(value Decl) {
	d.Insert(d.Len(), value)
}

// Insert inserts a new declaration at the given index.
func (d DeclBody) Insert(n int, value Decl) {
	d.Context().panicIfNotOurs(value)

	insertSlice(&d.raw.kinds, n, value.declKind())
	insertSlice(&d.raw.indices, n, decl[Decl](value.declIndex()+1))
}

// Delete deletes the declaration at the given index.
func (d DeclBody) Delete(n int) {
	deleteSlice(&d.raw.kinds, n)
	deleteSlice(&d.raw.indices, n)
}

func (d DeclBody) declIndex() int {
	return d.idx
}

// Decls returns an iterator over the nodes within a body of a particular type.
func Decls[T Decl](d DeclBody) func(func(int, T) bool) {
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

func (DeclBody) with(ctx *Context, idx int) Decl {
	return DeclBody{withContext{ctx}, idx, ctx.decls.bodies.At(idx)}
}
