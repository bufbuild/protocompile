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
	"slices"
	"strings"
	"sync"
	"unicode/utf16"
)

// Context is where all of the book-keeping for the AST of a particular file is kept.
//
// Virtually all operations inside of package ast2 involve a Context. However, most of
// the exported types carry their Context with them, so you don't need to worry about
// passing it around.
type Context struct {
	// The data of the file we've parsed.
	path, text string

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

	// Storage for Types.
	modifieds pointers[rawModified]
	generics  pointers[rawGeneric]

	// Protects the two line-related fields.
	linesOnce sync.Once
	// A prefix sum of the line lengths of text. Given a byte offset, it is possible
	// to recover which line that offset is on by performing a binary search on this
	// list.
	//
	// Alternatively, this slice can be interpreted as the index after each \n in the
	// original file.
	lines []int
	// Similar to the above, but instead using the length of each line in code units
	// if it was transcoded to UTF-16. This is required for compatibility with LSP.
	utf16Lines []int
}

// Contextual is any AST type that carries a context (virtually all of them).
type Contextual interface {
	// Context returns this types's [Context].
	//
	// Zero values of this type should return nil.
	Context() *Context
}

// NewContext creates a fresh context for a particular file.
func NewContext(path, text string) *Context {
	return &Context{path: path, text: text}
}

// Context implements [Contextual] for Context.
func (c *Context) Context() *Context {
	return c
}

// Path returns the (alleged) file system path for this file.
//
// This path is not used for anything except for diagnostics.
func (c *Context) Path() string {
	return c.path
}

// Returns the full text of the file.
func (c *Context) Text() string {
	return c.text
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

// PushToken mints the next token referring to a piece of the input source.
func (c *Context) PushToken(length int, kind TokenKind) Token {
	c.panicIfNil()

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

// PushToken mints the next token referring to a piece of the input source, and
// marks it as a close token for open.
//
// If open is synthethic or not currently a leaf, this function panics.
func (c *Context) PushCloseToken(length int, kind TokenKind, open Token) Token {
	c.panicIfNil()

	impl := open.impl()
	if impl == nil {
		panic("protocompile/ast: called PushCloseToken() with a synthetic open token")
	}
	if !impl.IsLeaf() {
		panic("protocompile/ast: called PushCloseToken() with non-leaf as the open token")
	}

	tok := c.PushToken(length, kind)

	diff := int32(tok.id - open.id)
	impl.kindAndOffset |= diff << tokenOffsetShift
	tok.impl().kindAndOffset |= -diff << tokenOffsetShift

	return tok
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

// Location returns location information for the given byte offset.
func (c *Context) Location(offset int) Location {
	c.panicIfNil()

	c.linesOnce.Do(func() {
		// Compute the prefix sum on-demand.
		var next, next16 int
		rest := c.text

		// We add 1 to the return value of IndexByte because we want to work
		// with the index immediately *after* the newline byte.
		for newline := strings.IndexByte(rest, '\n') + 1; newline != 0; {
			line := rest[:newline]
			rest = rest[newline:]

			c.lines = append(c.lines, next)
			next += newline

			// Calculate the length of `line` in UTF-16 code units.
			var utf16Len int
			for _, r := range line {
				utf16Len += utf16.RuneLen(r)
			}

			c.utf16Lines = append(c.utf16Lines, next16)
			next16 += utf16Len
		}

		c.lines = append(c.lines, next)
		c.utf16Lines = append(c.utf16Lines, next16)
	})

	// Find the smallest index in c.lines such that lines[line] <= offset
	line, _ := slices.BinarySearch(c.lines, offset)

	// Calculate the column.
	// TODO: Use unicode width properly.
	column := offset - c.lines[line]

	// Calculate the UTF-16 offset of of the offset within its line.
	chunk := c.text[c.lines[line]:offset]
	var utf16Col int
	for _, r := range chunk {
		utf16Col += utf16.RuneLen(r)
	}

	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
		UTF16:  utf16Col,
	}
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
	_ = c.path
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
	_ = c.ctx.path
}
