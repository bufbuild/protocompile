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
	"math"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/iter"
)

// Stream is a token stream.
//
// Internally, Stream uses a compressed representation for storing tokens, and
// is not precisely a [][Token]. In particular, it supports the creation of
// "synthetic" tokens, described in detail in this package's documentation.
//
// Streams may be "frozen", meaning that whatever lexing operation it was
// meant for is complete, and new tokens cannot be pushed to it. This is used
// by the Protocompile lexer to prevent re-use of a stream for multiple files.
type Stream struct {
	// The context that owns this stream.
	Context

	// The file this stream is over.
	*report.File

	// Storage for tokens.
	nats   []nat
	synths []synth

	// This contains materialized literals for some tokens. For example, given
	// a token with text 1.5, this map will map that token's ID to the float
	// value 1.5.
	//
	// Not all literal tokens will have an entry here; only those that have
	// uncommon representations, such as hex literals, floats, and strings with
	// escapes/implicit concatenation.
	//
	// This means the lexer can deal with the complex literal parsing logic on
	// our behalf in general, but common cases are re-parsed on-demand.
	// Specifically, the most common literals (decimal integers and simple
	// quoted strings) do not generate entries in this map and thus do not
	// contribute at-rest memory usage.
	//
	// All values in this map are string, uint64, or float64.
	literals map[ID]any

	// If true, no further mutations (except for synthetic tokens) are
	// permitted.
	frozen bool
}

// All returns an iterator over all tokens in this stream. First the natural
// tokens in order of creation, and then the synthetic tokens in the same.
func (s *Stream) All() iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for i := range s.nats {
			if !yield(ID(i + 1).In(s.Context)) {
				return
			}
		}
		for i := range s.synths {
			if !yield(ID(^i).In(s.Context)) {
				return
			}
		}
	}
}

// Cursor returns a cursor over the natural token stream.
func (s *Stream) Cursor() *Cursor {
	return &Cursor{
		withContext: internal.NewWith(s.Context),
	}
}

func (s *Stream) Naturals() iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for i := range s.nats {
			if !yield(ID(i + 1).In(s.Context)) {
				return
			}
		}
	}
}

//func (s *Stream) CursorBetween(start, end Token) *Cursor {
//	return &Cursor{
//		withContext: internal.NewWith(s.Context),
//		start:       start.ID() + 1,
//		end:         end.ID(),
//	}
//}
//
//func (s *Stream) CursorAfter(start Token) *Cursor {
//	return &Cursor{
//		withContext: internal.NewWith(s.Context),
//		start:       start.ID() + 1,
//		end:         ID(len(s.nats) + 1),
//	}
//}

// AssertEmpty asserts that no natural tokens have been created in this stream
// yet. It panics if they already have.
func (s *Stream) AssertEmpty() {
	if len(s.nats) > 0 {
		panic("protocompile/token: expected an empty token stream for " + s.Path())
	}
}

// Freeze marks this stream as frozen. This means that all mutation operations
// except for creation of synthetic tokens will panic.
//
// Freezing cannot be checked for or undone; callers must assume any token
// stream they did not create has already been frozen.
func (s *Stream) Freeze() {
	s.frozen = true
}

// Push mints the next token referring to a piece of the input source.
//
// Panics if this stream is frozen.
func (s *Stream) Push(length int, kind Kind) Token {
	if s.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	if length < 0 || length > math.MaxInt32 {
		panic(fmt.Sprintf("protocompile/token: Push() called with invalid length: %d", length))
	}

	var prevEnd int
	if len(s.nats) != 0 {
		prevEnd = int(s.nats[len(s.nats)-1].end)
	}

	end := prevEnd + length
	if end > len(s.Text()) {
		panic(fmt.Sprintf("protocompile/token: Push() overflowed backing text: %d > %d", end, len(s.Text())))
	}

	s.nats = append(s.nats, nat{
		end:           uint32(prevEnd + length),
		kindAndOffset: int32(kind) & kindMask,
	})

	return Token{internal.NewWith[Context](s), ID(len(s.nats))}
}

// NewIdent mints a new synthetic identifier token with the given name.
func (s *Stream) NewIdent(name string) Token {
	return s.newSynth(synth{
		text: name,
		kind: Ident,
	})
}

// NewPunct mints a new synthetic punctuation token with the given text.
func (s *Stream) NewPunct(text string) Token {
	return s.newSynth(synth{
		text: text,
		kind: Punct,
	})
}

// NewString mints a new synthetic string containing the given text.
func (s *Stream) NewString(text string) Token {
	return s.newSynth(synth{
		text: text,
		kind: String,
	})
}

// NewFused mints a new synthetic open/close pair using the given tokens.
//
// Panics if either open or close is natural or non-leaf.
func (s *Stream) NewFused(openTok, closeTok Token, children ...Token) {
	if !openTok.IsSynthetic() || !closeTok.IsSynthetic() {
		panic("protocompile/token: called NewOpenClose() with natural delimiters")
	}
	if !openTok.IsLeaf() || !closeTok.IsLeaf() {
		panic("protocompile/token: called PushCloseToken() with non-leaf as a delimiter token")
	}

	synth := openTok.synth()
	synth.otherEnd = closeTok.id
	synth.children = make([]ID, len(children))
	for i, t := range children {
		synth.children[i] = t.id
	}
	closeTok.synth().otherEnd = openTok.id
}

func (s *Stream) newSynth(tok synth) Token {
	raw := ID(^len(s.synths))
	s.synths = append(s.synths, tok)
	return raw.In(s.Context)
}
