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

package token

import (
	"fmt"
	"iter"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// Cursor is an iterator-like construct for looping over a token tree.
// Unlike a plain range func, it supports peeking.
type Cursor struct {
	context *Stream

	// This is used if this is a cursor over the children of a synthetic token.
	// If stream is nil, we know we're in the natural case.
	stream []ID
	// This is the index into either Context().Stream().nats or the stream.
	idx int
	// This is used to know if we moved forwards or backwards when calculating
	// the offset jump on a change of directions.
	isBackwards bool
}

// CursorMark is the return value of [Cursor.Mark], which marks a position on
// a Cursor for rewinding to.
type CursorMark struct {
	// This contains exactly the values needed to rewind the cursor.
	owner       *Cursor
	idx         int
	isBackwards bool
}

// NewCursorAt returns a new cursor at the given token.
//
// Panics if the token is zero or synthetic.
func NewCursorAt(tok Token) *Cursor {
	if tok.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to NewCursorAt: %v", tok))
	}
	if tok.IsSynthetic() {
		panic(fmt.Sprintf("protocompile/token: passed synthetic token to NewCursorAt: %v", tok))
	}

	return &Cursor{
		context:     tok.Context(),
		idx:         naturalIndex(tok.ID()), // Convert to 0-based index.
		isBackwards: tok.nat().IsClose(),    // Set the direction to calculate the offset.
	}
}

// NewSliceCursor returns a new cursor over a slice of token IDs in the given
// context.
func NewSliceCursor(stream *Stream, slice []ID) *Cursor {
	return &Cursor{
		context: stream,
		stream:  slice,
	}
}

// Context returns this Cursor's context.
func (c *Cursor) Context() *Stream {
	return c.context
}

// Done returns whether or not there are still tokens left to yield.
func (c *Cursor) Done() bool {
	return c.Peek().IsZero()
}

// IsSynthetic returns whether this is a cursor over synthetic tokens.
func (c *Cursor) IsSynthetic() bool {
	return c.stream != nil
}

// Clone returns a copy of this cursor, which allows performing operations on
// it without mutating the original cursor.
func (c *Cursor) Clone() *Cursor {
	clone := *c
	return &clone
}

// Mark makes a mark on this cursor to indicate a place that can be rewound
// to.
func (c *Cursor) Mark() CursorMark {
	return CursorMark{
		owner:       c,
		idx:         c.idx,
		isBackwards: c.isBackwards,
	}
}

// Rewind moves this cursor back to the position described by Rewind.
//
// Panics if mark was not created using this cursor's Mark method.
func (c *Cursor) Rewind(mark CursorMark) {
	if c != mark.owner {
		panic("protocompile/ast: rewound cursor using the wrong cursor's mark")
	}
	c.idx = mark.idx
	c.isBackwards = mark.isBackwards
}

// PeekSkippable returns the current token in the sequence, if there is one.
// This may return a skippable token.
//
// Returns the zero token if this cursor is at the end of the stream.
func (c *Cursor) PeekSkippable() Token {
	if c == nil {
		return Zero
	}
	if c.IsSynthetic() {
		tokenID, ok := slicesx.Get(c.stream, c.idx)
		if !ok {
			return Zero
		}
		return id.Wrap(c.Context(), tokenID)
	}
	stream := c.Context()
	impl, ok := slicesx.Get(stream.nats, c.idx)
	if !ok || (!c.isBackwards && impl.IsClose()) {
		return Zero // Reached the end.
	}
	return id.Wrap(c.Context(), ID(c.idx+1))
}

// PeekPrevSkippable returns the token before the current token in the sequence, if there is one.
// This may return a skippable token.
//
// Returns the zero token if this cursor is at the beginning of the stream.
func (c *Cursor) PeekPrevSkippable() Token {
	if c == nil {
		return Zero
	}
	if c.IsSynthetic() {
		tokenID, ok := slicesx.Get(c.stream, c.idx-1)
		if !ok {
			return Zero
		}
		return id.Wrap(c.Context(), tokenID)
	}
	stream := c.Context()
	idx := c.idx - 1
	if c.isBackwards {
		impl, ok := slicesx.Get(stream.nats, c.idx)
		if ok && impl.IsClose() {
			idx += impl.Offset()
		}
	}
	impl, ok := slicesx.Get(stream.nats, idx)
	if !ok || impl.IsOpen() {
		return Zero // Reached the start.
	}
	return id.Wrap(c.Context(), ID(idx+1))
}

// NextSkippable returns the next skippable token in the sequence, and advances the cursor.
func (c *Cursor) NextSkippable() Token {
	tok := c.PeekSkippable()
	if tok.IsZero() {
		return tok
	}

	c.isBackwards = false
	if c.IsSynthetic() {
		c.idx++
	} else {
		impl := tok.nat()
		if impl.IsOpen() {
			c.idx += impl.Offset()
		}
		c.idx++
	}
	return tok
}

// PrevSkippable returns the previous skippable token in the sequence, and decrements the cursor.
func (c *Cursor) PrevSkippable() Token {
	tok := c.PeekPrevSkippable()
	if tok.IsZero() {
		return tok
	}

	c.isBackwards = true
	if c.IsSynthetic() {
		c.idx--
	} else {
		c.idx = naturalIndex(tok.ID())
	}
	return tok
}

// Peek returns the next token in the sequence, if there is one.
// This automatically skips past skippable tokens.
//
// Returns the zero token if this cursor is at the end of the stream.
func (c *Cursor) Peek() Token {
	if c == nil {
		return Zero
	}
	cursor := *c
	return cursor.Next()
}

// PeekPrev returns the previous token in the sequence, if there is one.
// This automatically skips past skippable tokens.
//
// Returns the zero token if this cursor is at the start of the stream.
func (c *Cursor) PeekPrev() Token {
	if c == nil {
		return Zero
	}
	cursor := *c
	return cursor.Prev()
}

// Next returns the next token in the sequence, and advances the cursor.
func (c *Cursor) Next() Token {
	for {
		next := c.NextSkippable()
		if next.IsZero() || !next.Kind().IsSkippable() {
			return next
		}
	}
}

// Prev returns the previous token in the sequence, and decrements the cursor.
func (c *Cursor) Prev() Token {
	for {
		prev := c.PrevSkippable()
		if prev.IsZero() || !prev.Kind().IsSkippable() {
			return prev
		}
	}
}

// Rest returns an iterator over the remaining tokens in the cursor.
//
// Note that breaking out of a loop over this iterator, and starting
// a new loop, will resume at the iteration that was broken at. E.g., if
// we break out of a loop over c.Iter at token tok, and start a new range
// over c.Iter, the first yielded token will be tok.
func (c *Cursor) Rest() iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for {
			tok := c.Peek()
			if tok.IsZero() || !yield(tok) {
				break
			}
			_ = c.Next()
		}
	}
}

// RestSkippable is like [Cursor.Rest]. but it yields skippable tokens, too.
//
// Note that breaking out of a loop over this iterator, and starting
// a new loop, will resume at the iteration that was broken at. E.g., if
// we break out of a loop over c.Iter at token tok, and start a new range
// over c.Iter, the first yielded token will be tok.
func (c *Cursor) RestSkippable() iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for {
			tok := c.PeekSkippable()
			if tok.IsZero() || !yield(tok) {
				break
			}
			_ = c.NextSkippable()
		}
	}
}

// SeekToEnd returns a span for whatever comes immediately after the end of this
// cursor (be that a token or the EOF), and advances the cursor to the end.
// If it is a token, this will return that token, too.
//
// Returns [Zero] for a synthetic cursor.
func (c *Cursor) SeekToEnd() (Token, source.Span) {
	if c == nil || c.stream != nil {
		return Zero, source.Span{}
	}

	// Seek to the end.
	end := c.NextSkippable()
	for !end.IsZero() {
		end = c.NextSkippable()
	}

	stream := c.Context()
	if c.idx >= len(stream.nats) {
		// This is the case where this cursor is a Stream.Cursor(). Thus, the
		// just-after span should be the EOF.
		return Zero, stream.EOF()
	}
	// Otherwise, return end.
	tok := id.Wrap(c.Context(), ID(c.idx+1))
	return tok, stream.Span(tok.offsets())
}
