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
	"fmt"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
)

// Context is where all of the book-keeping for the AST of a particular file is kept.
//
// Virtually all operations inside of package ast2 involve a Context. However, most of
// the exported types carry their Context with them, so you don't need to worry about
// passing it around.
type Context struct {
	file *report.IndexedFile

	// Storage for tokens.
	stream          []tokenImpl
	syntheticTokens []tokenSynthetic

	// This contains materialized literals for some tokens.
	//
	// Not all tokens will have an entry here; only those that have "unusual"
	// representations. This means the lexer can deal with the complex parsing
	// logic on our behalf in general, but common cases are re-parsed on-demand.
	//
	// All elements of this map are string, uint64, or float64.
	literals map[rawToken]any

	// Storage for the various node types.
	decls decls
	types types
	exprs exprs

	options arena.Arena[rawCompactOptions]
}

// Contextual is any AST type that carries a context (virtually all of them).
type Contextual interface {
	// Context returns this types's [Context].
	//
	// Zero values of this type should return nil.
	Context() *Context
}

// newContext creates a fresh context for a particular file.
func newContext(file report.File) *Context {
	c := &Context{file: report.NewIndexedFile(file), literals: map[rawToken]any{}}
	c.NewDeclBody(Token{}) // This is the rawBody for the whole file.
	return c
}

// Parse parses a Protobuf file, and places any diagnostics encountered in report.
func Parse(file report.File, report *report.Report) File {
	lexer := lexer{Context: newContext(file)}

	report.Stage++
	lexer.Lex(report)

	report.Stage++
	parse(report, lexer.Context)

	report.Stage++
	legalize(report, nil, lexer.Context.Root())

	return lexer.Context.Root()
}

// Context implements [Contextual] for Context.
func (c *Context) Context() *Context {
	return c
}

// Stream returns a cursor over the whole lexed token stream.
func (c *Context) Stream() *Cursor {
	return &Cursor{
		withContext: withContext{c},
		start:       1,
		end:         rawToken(len(c.stream) + 1),
	}
}

// Path returns the (alleged) file system path for this file.
//
// This path is not used for anything except for diagnostics.
func (c *Context) Path() string {
	return c.file.File().Path
}

// Returns the full text of the file.
func (c *Context) Text() string {
	return c.file.File().Text
}

// Root returns the root AST node for this context.
func (c *Context) Root() File {
	// NewContext() sticks the root at the beginning of bodies for us.
	return File{wrapDecl[DeclScope](1, c)}
}

// Tokens returns a flat slice over all of the non-synthetic tokens in this context,
// with no respect to nesting.
//
// You should probably use [Context.Stream] instead of this.
func (c *Context) Tokens() Slice[Token] {
	return funcSlice[tokenImpl, Token]{
		s: c.stream,
		f: func(i int, _ *tokenImpl) Token { return rawToken(i + 1).With(c) },
	}
}

// NOTE: Some methods of Context live in the context_*.go files. This is to
// reduce clutter in this file.

// panicIfNil panics if this context is nil.
//
// This is helpful for immediately panicking on function entry.
func (c *Context) panicIfNil() {
	_ = c.file
}

// ours checks that a contextual value is owned by this context, and panics if not.
//
// Does not panic if that is nil or has a nil context. Panics if c is nil.
func (c *Context) panicIfNotOurs(that ...Contextual) {
	c.panicIfNil()
	for _, that := range that {
		if that == nil {
			continue
		}

		c2 := that.Context()
		if c2 == nil || c2 == c {
			continue
		}
		panic(fmt.Sprintf("protocompile/ast: attempt to mix different contexts: %p(%q) and %p(%q)", c, c.Path(), c2, c2.Path()))
	}
}

// withContext is an embedable type that provides common operations involving
// a context, causing it to implement Contextual.
type withContext struct {
	ctx *Context
}

// Context returns this type's associated [ast.Context].
//
// Returns `nil` if this is this type's zero value.
func (c withContext) Context() *Context {
	return c.ctx
}

// Nil checks whether this is this type's zero value.
func (c withContext) Nil() bool {
	return c.ctx == nil
}

// panicIfNil panics if this context is nil.
//
// This is helpful for immediately panicking on function entry.
func (c *withContext) panicIfNil() {
	c.Context().panicIfNil()
}
