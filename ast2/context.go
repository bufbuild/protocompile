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

import (
	"fmt"
	"math"

	"github.com/bufbuild/protocompile/report2"
)

// Context is where all of the book-keeping for the AST of a particular file is kept.
//
// Virtually all operations inside of package ast2 involve a Context. However, most of
// the exported types carry their Context with them, so you don't need to worry about
// passing it around.
//
// Context implements [Slice] over [Token]s.
type Context struct {
	file *report2.IndexedFile

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

	// Storage for Decls.
	pragmas  pointers[rawPragma]
	packages pointers[rawPackage]
	imports  pointers[rawImport]
	defs     pointers[rawDef]
	bodies   pointers[rawBody]
	fields   pointers[rawField]
	methods  pointers[rawMethod]
	ranges   pointers[rawRange]
	options  pointers[rawOption]

	// Storage for Types.
	modifieds pointers[rawModified]
	generics  pointers[rawGeneric]

	// Storage for Exprs.
	signedExprs  pointers[rawExprSigned]
	rangeExprs   pointers[rawExprRange]
	arrayExprs   pointers[rawExprArray]
	messageExprs pointers[rawExprMessage]
	fieldExprs   pointers[rawExprField]

	// Storage for Keys.
	extnKeys pointers[rawKeyExtension]
	anyKeys  pointers[rawKeyAny]
}

var _ Slice[Token] = (*Context)(nil)

// Contextual is any AST type that carries a context (virtually all of them).
type Contextual interface {
	// Context returns this types's [Context].
	//
	// Zero values of this type should return nil.
	Context() *Context
}

// newContext creates a fresh context for a particular file.
func newContext(file report2.File) *Context {
	c := &Context{file: report2.NewIndexedFile(file), literals: map[rawToken]any{}}
	c.NewBody(Token{}) // This is the rawBody for the whole file.
	return c
}

// Parse parses a Protobuf file, and places any diagnostics encountered in report.
func Parse(file report2.File, report *report2.Report) File {
	lexer := lexer{Context: newContext(file)}
	lexer.Lex(report)
	return lexer.Context.Root()
}

// Context implements [Contextual] for Context.
func (c *Context) Context() *Context {
	return c
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
	return File{decl[Body](1).With(c)}
}

// Len returns the number of non-synthetic tokens in this context.
func (c *Context) Len() int {
	return len(c.stream)
}

// At returns the nth non-synthetic token in this context.
func (c *Context) At(n int) Token {
	_ = c.stream[n]
	return rawToken(n + 1).With(c)
}

// Iter is an iterator over the non-synthetic tokens in this context.
func (c *Context) Iter(yield func(int, Token) bool) {
	for i := range c.Len() {
		if !yield(i, rawToken(i+1).With(c)) {
			break
		}
	}
}

// NewSpan creates a new span in this context.
//
// Panics if either endpoint is out of bounds or if start > end.
func (c *Context) NewSpan(start, end int) Span {
	c.panicIfNil()

	if start > end {
		panic(fmt.Sprintf("protocompile/ast: called NewSpan() with %d > %d", start, end))
	}
	if end > len(c.Text()) {
		panic(fmt.Sprintf("protocompile/ast: NewSpan() argument out of bounds: %d > %d", end, len(c.Text())))
	}

	return Span{withContext{c}, start, end}
}

// NewPragma creates a new Pragma node.
func (c *Context) NewPragma(args PragmaArgs) Pragma {
	c.panicIfNotOurs(args.Keyword, args.Equals, args.Value, args.Semicolon)
	c.pragmas.Append(rawPragma{
		keyword: args.Keyword.id,
		equals:  args.Equals.id,
		semi:    args.Semicolon.id,
	})
	pragma := decl[Pragma](c.pragmas.Len()).With(c)
	pragma.SetValue(args.Value)
	return pragma
}

// NewBody creates a new [Body] in this context, with the given token (nillable) as the braces.
func (c *Context) NewBody(braces Token) Body {
	c.panicIfNotOurs(braces)
	c.bodies.Append(rawBody{braces: braces.id})
	return decl[Body](c.bodies.Len()).With(c)
}

// PushToken mints the next token referring to a piece of the input source.
func (c *Context) PushToken(length int, kind TokenKind) Token {
	c.panicIfNil()

	if length < 0 || length > math.MaxInt32 {
		panic(fmt.Sprintf("protocompile/ast: PushToken() called with invalid length: %d", length))
	}

	var prevEnd int
	if len(c.stream) != 0 {
		prevEnd = int(c.stream[len(c.stream)-1].end)
	}

	c.stream = append(c.stream, tokenImpl{
		end:           uint32(prevEnd + length),
		kindAndOffset: int32(kind) & tokenKindMask,
	})

	return Token{withContext{c}, rawToken(len(c.stream))}
}

// FuseTokens marks a pair of tokens as their respective open and close.
//
// If open or close are synthethic or not currently a leaf, this function panics.
func (c *Context) FuseTokens(open, close Token) {
	c.panicIfNil()

	impl1 := open.impl()
	if impl1 == nil {
		panic("protocompile/ast: called FuseTokens() with a synthetic open token")
	}
	if !impl1.IsLeaf() {
		panic("protocompile/ast: called FuseTokens() with non-leaf as the open token")
	}

	impl2 := close.impl()
	if impl2 == nil {
		panic("protocompile/ast: called FuseTokens() with a synthetic open token")
	}
	if !impl2.IsLeaf() {
		panic("protocompile/ast: called FuseTokens() with non-leaf as the open token")
	}

	diff := int32(close.id - open.id)
	if diff <= 0 {
		panic("protocompile/ast: called FuseTokens() with out-of-order")
	}

	impl1.kindAndOffset |= diff << tokenOffsetShift
	impl2.kindAndOffset |= -diff << tokenOffsetShift
}

// NewIdent mints a new synthetic identifier token with the given name.
func (c *Context) NewIdent(name string) Token {
	c.panicIfNil()

	return c.newSynth(tokenSynthetic{
		text: name,
		kind: TokenIdent,
	})
}

// NewIdent mints a new synthetic punctuation token with the given text.
func (c *Context) NewPunct(text string) Token {
	c.panicIfNil()

	return c.newSynth(tokenSynthetic{
		text: text,
		kind: TokenPunct,
	})
}

// NewString mints a new synthetic string containing the given text.
func (c *Context) NewString(text string) Token {
	c.panicIfNil()

	return c.newSynth(tokenSynthetic{
		text: text,
		kind: TokenString,
	})
}

// NewOpenClose mints a new synthetic open/close pair using the given tokens.
//
// Panics if either open or close is non-synthetic or non-leaf.
func (c *Context) NewOpenClose(open, close Token, children ...Token) {
	c.panicIfNil()

	if !open.IsSynthetic() || !close.IsSynthetic() {
		panic("protocompile/ast: called NewOpenClose() with non-synthetic delimiters")
	}
	if !open.IsLeaf() || !close.IsLeaf() {
		panic("protocompile/ast: called PushCloseToken() with non-leaf as a delimiter token")
	}

	synth := open.synthetic()
	synth.otherEnd = close.id
	synth.children = make([]rawToken, len(children))
	for i, t := range children {
		synth.children[i] = t.id
	}
	close.synthetic().otherEnd = open.id
}

func (c *Context) newSynth(tok tokenSynthetic) Token {
	c.panicIfNil()

	raw := rawToken(^len(c.syntheticTokens))
	c.syntheticTokens = append(c.syntheticTokens, tok)
	return raw.With(c)
}

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
