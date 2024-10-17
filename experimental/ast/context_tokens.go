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
	"math"
)

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

	if length < 0 || length > math.MaxInt32 {
		panic(fmt.Sprintf("protocompile/ast: PushToken() called with invalid length: %d", length))
	}

	var prevEnd int
	if len(c.stream) != 0 {
		prevEnd = int(c.stream[len(c.stream)-1].end)
	}

	end := prevEnd + length
	if end > len(c.Text()) {
		panic(fmt.Sprintf("protocompile/ast: PushToken() overflowed backing text: %d > %d", end, len(c.Text())))
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
//
//nolint:predeclared // For close.
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

	diff := int32(close.raw - open.raw)
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
// Panics if either open or close is natural or non-leaf.
func (c *Context) NewOpenClose(openTok, closeTok Token, children ...Token) {
	c.panicIfNil()

	if !openTok.IsSynthetic() || !closeTok.IsSynthetic() {
		panic("protocompile/ast: called NewOpenClose() with natural delimiters")
	}
	if !openTok.IsLeaf() || !closeTok.IsLeaf() {
		panic("protocompile/ast: called PushCloseToken() with non-leaf as a delimiter token")
	}

	synth := openTok.synthetic()
	synth.otherEnd = closeTok.raw
	synth.children = make([]rawToken, len(children))
	for i, t := range children {
		synth.children[i] = t.raw
	}
	closeTok.synthetic().otherEnd = openTok.raw
}

func (c *Context) newSynth(tok tokenSynthetic) Token {
	c.panicIfNil()

	raw := rawToken(^len(c.syntheticTokens))
	c.syntheticTokens = append(c.syntheticTokens, tok)
	return raw.With(c)
}
