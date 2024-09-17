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

package ast2

import "iter"

// Message is a message definition.
//
// Like other definitions, it consists of a keyword (message), a name, and a body.
type Message struct {
	withContext

	raw *rawDef
}

// Keyword returns the "message" keyword for this definition.
func (m Message) Keyword() Token {
	return m.raw.keyword.With(m)
}

// Name returns this message's declared name.
//
// For permissiveness, this is a path, not a single identifier. You will almost
// always want to do Name().AsIdent(). May also be nil, if the user wrote something
// like message {}.
func (m Message) Name() Path {
	return m.raw.name.With(m)
}

// Body returns this message's body.
//
// May be nil for partly-invalid message definitions.
func (m Message) Body() Body {
	return m.raw.body.With(m)
}

// Span implements [Spanner] for Message.
func (m Message) Span() Span {
	return JoinSpans(m.Keyword(), m.Name(), m.Body())
}

func (Message) with(ctx *Context, idx int) Decl {
	return Message{withContext{ctx}, ctx.defs.At(idx)}
}

// Enum is an enum definition.
//
// Like other definitions, it consists of a keyword (enum), a name, and a body.
type Enum struct {
	withContext

	raw *rawDef
}

// Keyword returns the "enum" keyword for this definition.
func (e Enum) Keyword() Token {
	return e.raw.keyword.With(e)
}

// Name returns this enum's declared name.
//
// For permissiveness, this is a path, not a single identifier. You will almost
// always want to do Name().AsIdent(). May also be nil, if the user wrote something
// like enum {}.
func (e Enum) Name() Path {
	return e.raw.name.With(e)
}

// Body returns this enum's body.
//
// May be nil for partly-invalid enum definitions.
func (e Enum) Body() Body {
	return e.raw.body.With(e)
}

// Span implements [Spanner] for Enum.
func (e Enum) Span() Span {
	return JoinSpans(e.Keyword(), e.Name(), e.Body())
}

func (Enum) with(ctx *Context, idx int) Decl {
	return Enum{withContext{ctx}, ctx.defs.At(idx)}
}

// Extends is an extension definition.
//
// Like other definitions, it consists of a keyword (extends), a name, and a body.
type Extends struct {
	withContext

	raw *rawDef
}

// Keyword returns the "service" keyword for this definition.
func (e Extends) Keyword() Token {
	return e.raw.keyword.With(e)
}

// Extendee returns the path of the message type being extended.
//
// May be nil if the user wrote something like extend {}.
func (e Extends) Extendee() Path {
	return e.raw.name.With(e)
}

// Body returns this service's body.
//
// May be nil for partly-invalid service definitions.
func (e Extends) Body() Body {
	return e.raw.body.With(e)
}

// Span implements [Spanner] for Service.
func (e Extends) Span() Span {
	return JoinSpans(e.Keyword(), e.Extendee(), e.Body())
}

func (Extends) with(ctx *Context, idx int) Decl {
	return Extends{withContext{ctx}, ctx.defs.At(idx)}
}

// Service is a service definition.
//
// Like other definitions, it consists of a keyword (service), a name, and a body.
type Service struct {
	withContext

	raw *rawDef
}

// Keyword returns the "service" keyword for this definition.
func (s Service) Keyword() Token {
	return s.raw.keyword.With(s)
}

// Name returns this service's declared name.
//
// For permissiveness, this is a path, not a single identifier. You will almost
// always want to do Name().AsIdent(). May also be nil, if the user wrote something
// like service {}.
func (s Service) Name() Path {
	return s.raw.name.With(s)
}

// Body returns this service's body.
//
// May be nil for partly-invalid service definitions.
func (s Service) Body() Body {
	return s.raw.body.With(s)
}

// Span implements [Spanner] for Service.
func (s Service) Span() Span {
	return JoinSpans(s.Keyword(), s.Name(), s.Body())
}

func (Service) with(ctx *Context, idx int) Decl {
	return Service{withContext{ctx}, ctx.defs.At(idx)}
}

// rawDef is the backing data shared by all "definition-like" structures
// that are a keyword, a name, and a brace-delimited body.
type rawDef struct {
	keyword rawToken
	name    rawPath
	body    rawDecl[Body]
}

// Body is the body of a definition like a [Message], or the whole contents of a [File]. The
// protocompile AST is very lenient, and allows any declaration to exist anywhere, for the
// benefit of rich diagnostics and refactorings. For example, it is possible to represent an
// "orphaned" field or oneof outside of a message, or an RPC method inside of an enum, and
// so on.
//
// Body implements [Slice], providing access to its declarations.
type Body struct {
	withContext

	raw *rawBody
}

type rawBody struct {
	braces rawToken

	// These slices are co-indexed
	kinds   []declKind
	indices []uint32
}

// Body implements Slice[Node].
var _ Slice[Decl] = Body{}

// Braces returns this body's surrounding braces, if it has any.
func (b Body) Braces() Token {
	return b.raw.braces.With(b)
}

// Span implements [Spanner] for Body.
func (b Body) Span() Span {
	if !b.Braces().Nil() {
		return b.Braces().Span()
	}

	if b.Len() == 0 {
		return Span{}
	}

	return JoinSpans(b.At(0), b.At(b.Len()-1))
}

// Len returns the number of declarations inside of this body.
func (b Body) Len() int {
	return len(b.raw.indices)
}

// At returns the nth element of this body.
func (b Body) At(n int) Decl {
	return b.raw.kinds[n].reify().with(b.Context(), int(b.raw.indices[n]))
}

// Iter is an iterator over the nodes inside this body.
func (b Body) Iter(yield func(int, Decl) bool) {
	for i := range b.raw.kinds {
		if !yield(i, b.At(i)) {
			break
		}
	}
}

// Decls returns an iterator over the nodes within a body of a particular type.
func Decls[T Decl](b Body) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		var idx int
		for _, decl := range b.Iter {
			if actual, ok := decl.(T); ok {
				if !yield(idx, actual) {
					break
				}
				idx++
			}
		}
	}
}

func (Body) with(ctx *Context, idx int) Decl {
	return Body{withContext{ctx}, ctx.bodies.At(idx)}
}
