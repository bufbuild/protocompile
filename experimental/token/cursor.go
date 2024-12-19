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

package token

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/iter"
)

// Cursor is an iterator-like construct for looping over a token tree.
// Unlike a plain range func, it supports peeking.
type Cursor struct {
	withContext

	// This is used if this is a cursor over natural tokens.
	// start is inclusive, end is exclusive. start == end means the stream
	// is empty.
	start, end ID
	// This is used if this is a cursor over the children of a synthetic token.
	// If stream is nil, we know we're in the natural case.
	stream []ID
	idx    int
}

// CursorMark is the return value of [Cursor.Mark], which marks a position on
// a Cursor for rewinding to.
type CursorMark struct {
	// This contains exactly the values needed to rewind the cursor.
	owner *Cursor
	start ID
	idx   int
}

// NewCursor returns a new cursor over the given tokens.
//
// Panics if either token is zero, the tokens come from different contexts, or
// either token is synthetic.
func NewCursor(start, end Token) *Cursor {
	if start.IsZero() || end.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to NewCursor: %v, %v", start, end))
	}
	if start.Context() != end.Context() {
		panic("protocompile/token: passed tokens from different context to NewCursor")
	}
	if start.IsSynthetic() || end.IsSynthetic() {
		panic("protocompile/token: passed synthetic token to NewCursor")
	}

	return &Cursor{
		withContext: internal.NewWith(start.Context()),
		start:       start.ID(),
		end:         end.ID() + 1, // Remember, Cursor.end is exclusive!
	}
}

// Done returns whether or not there are still tokens left to yield.
func (c *Cursor) Done() bool {
	return c.Peek().IsZero()
}

// IsSynthetic returns whether this is a cursor over synthetic tokens.
func (c *Cursor) IsSynthetic() bool {
	return c.stream != nil
}

// Mark makes a mark on this cursor to indicate a place that can be rewound
// to.
func (c *Cursor) Mark() CursorMark {
	return CursorMark{
		owner: c,
		start: c.start,
		idx:   c.idx,
	}
}

// Rewind moves this cursor back to the position described by Rewind.
//
// Panics if mark was not created using this cursor's Mark method.
func (c *Cursor) Rewind(mark CursorMark) {
	if c != mark.owner {
		panic("protocompile/ast: rewound cursor using the wrong cursor's mark")
	}
	c.start = mark.start
	c.idx = mark.idx
}

// Peek returns the next token in the sequence, if there is one.
// This may return a skippable token.
//
// Returns the zero token if this cursor is at the end of the stream.
func (c *Cursor) PeekSkippable() Token {
	if c == nil {
		return Zero
	}

	if c.IsSynthetic() {
		if c.idx == len(c.stream) {
			return Zero
		}
		return c.stream[c.idx].In(c.Context())
	}
	if c.start >= c.end {
		return Zero
	}
	return c.start.In(c.Context())
}

// Pop returns the next skippable token in the sequence, and advances the cursor.
func (c *Cursor) PopSkippable() Token {
	tok := c.PeekSkippable()
	if tok.IsZero() {
		return tok
	}

	if c.IsSynthetic() {
		c.idx++
	} else {
		impl := c.start.In(c.Context()).nat()
		if impl.Offset() > 0 {
			c.start += ID(impl.Offset())
		}
		c.start++
	}
	return tok
}

// Peek returns the next token in the sequence, if there is one.
// This automatically skips past skippable tokens.
//
// Returns the zero token if this cursor is at the end of the stream.
func (c *Cursor) Peek() Token {
	for {
		next := c.PeekSkippable()
		if next.IsZero() || !next.Kind().IsSkippable() {
			return next
		}
		c.PopSkippable()
	}
}

// Pop returns the next token in the sequence, and advances the cursor.
func (c *Cursor) Pop() Token {
	tok := c.Peek()
	if tok.IsZero() {
		return tok
	}

	return c.PopSkippable()
}

// Iter returns an iterator over the remaining tokens in the cursor.
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
			_ = c.Pop()
		}
	}
}

// IterSkippable is like [Cursor.Iter]. but it yields skippable tokens, too.
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
			_ = c.PopSkippable()
		}
	}
}

// JustAfter returns a span for whatever comes immediately after the end of
// this cursor (be that a token or the EOF). If it is a token, this will return
// that token, too.
//
// Returns [Zero] for a synthetic cursor.
func (c *Cursor) JustAfter() (Token, report.Span) {
	if c.stream != nil {
		return Zero, report.Span{}
	}

	stream := c.Context().Stream()
	if int(c.end) > len(stream.nats) {
		// This is the case where this cursor is a Stream.Cursor(). Thus, the
		// just-after span should be the EOF.
		return Zero, stream.EOF()
	}

	// Otherwise, return end.
	tok := c.end.In(c.Context())
	return tok, stream.Span(tok.offsets())
}
