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
	"math/big"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Set to true to enable verbose debug printing of tokens.
const debug = true

// IsSkippable returns whether this is a token that should be examined during
// syntactic analysis.
func (t Kind) IsSkippable() bool {
	// Note: kind.go is a generated file.
	return t == Space || t == Comment || t == Unrecognized
}

// Zero is the zero [Token].
var Zero Token

// Value is a constraint that represents a literal scalar value in source.
//
// This union does not include bool because they are lexed as
// identifiers and then later converted to boolean values based on their
// context (since "true" and "false" are also valid identifiers for named
// types).
type Value interface {
	uint64 | float64 | *big.Int | string
}

// Token is a lexical element of a Protobuf file.
//
// Protocompile's token stream is actually a tree of tokens. Some tokens, called
// non-leaf tokens, contain a selection of tokens "within" them. For example, the
// two matched braces of a message body are a single token, and all of the tokens
// between the braces are contained inside it. This moves certain complexity into
// the lexer in a way that allows us to handle matching delimiters generically.
//
// The zero value of Token is the so-called "zero token", which is used to denote the
// absence of a token.
type Token id.Node[Token, *Stream, rawToken]

type rawToken struct{}

// IsLeaf returns whether this is a non-zero leaf token.
func (t Token) IsLeaf() bool {
	if t.IsZero() {
		return false
	}

	if impl := t.nat(); impl != nil {
		return impl.IsLeaf()
	}
	return t.synth().IsLeaf()
}

// IsSynthetic returns whether this is a non-zero synthetic token (i.e., a token that didn't
// come from a parsing operation.)
func (t Token) IsSynthetic() bool {
	return t.ID() < 0
}

// Kind returns what kind of token this is.
//
// Returns [Unrecognized] if this token is zero.
func (t Token) Kind() Kind {
	if t.IsZero() {
		return Unrecognized
	}

	if impl := t.nat(); impl != nil {
		return impl.Kind()
	}
	return t.synth().kind
}

// Keyword returns the [keyword.Keyword] corresponding to this token's textual
// value.
//
// This is intended to be used for simplifying parsing, instead of comparing
// [Token.Text] to a literal string value.
func (t Token) Keyword() keyword.Keyword {
	switch {
	case t.IsZero():
		return keyword.Unknown
	case t.IsSynthetic():
		return t.synth().Keyword()
	default:
		return t.nat().Keyword()
	}
}

// Text returns the text fragment referred to by this token. This does not
// return the text contained inside of non-leaf tokens; if this token refers to
// a token tree, this will return only the text of the open (or close) token.
//
// For example, for a matched pair of braces, this will only return the text of
// the open brace, "{".
//
// Returns empty string for the zero token.
func (t Token) Text() string {
	if t.IsZero() {
		return ""
	}

	if synth := t.synth(); synth != nil {
		if synth.kind == String {
			// If this is a string, we need to add quotes and escape it.
			// This can be done on-demand.

			var escaped strings.Builder
			escaped.WriteRune('"')
			for _, r := range synth.text {
				switch {
				case r == '\n':
					escaped.WriteString("\\n")
				case r == '\r':
					escaped.WriteString("\\r")
				case r == '\t':
					escaped.WriteString("\\t")
				case r == '\a':
					escaped.WriteString("\\a")
				case r == '\b':
					escaped.WriteString("\\b")
				case r == '\f':
					escaped.WriteString("\\f")
				case r == '\v':
					escaped.WriteString("\\v")
				case r == 0:
					escaped.WriteString("\\0")
				case r == '"':
					escaped.WriteString("\\\"")
				case r == '\\':
					escaped.WriteString("\\\\")
				case r < ' ' || r == '\x7f':
					fmt.Fprintf(&escaped, "\\x%02x", r)
				case unicode.IsGraphic(r):
					escaped.WriteRune(r)
				case r < 0x10000:
					fmt.Fprintf(&escaped, "\\u%04x", r)
				default:
					fmt.Fprintf(&escaped, "\\U%08x", r)
				}
			}
			escaped.WriteRune('"')
			return escaped.String()
		}

		return synth.text
	}

	start, end := t.offsets()
	return t.Context().Text()[start:end]
}

// Span implements [Spanner].
func (t Token) Span() source.Span {
	if t.IsZero() || t.IsSynthetic() {
		return source.Span{}
	}

	var a, b int
	if !t.IsLeaf() {
		start, end := t.StartEnd()
		a, _ = start.offsets()
		_, b = end.offsets()
	} else {
		a, b = t.offsets()
	}

	return t.Context().Span(a, b)
}

// LeafSpan returns the span that this token would have if it was a leaf token.
func (t Token) LeafSpan() source.Span {
	if t.IsZero() || t.IsSynthetic() {
		return source.Span{}
	}

	return t.Context().Span(t.offsets())
}

// StartEnd returns the open and close tokens for this token.
//
// If this is a leaf token, start and end will be the same token and will compare as equal.
//
// Panics if this is a zero token.
func (t Token) StartEnd() (start, end Token) {
	if t.IsZero() {
		return Zero, Zero
	}

	switch impl := t.nat(); {
	case impl == nil:
		switch synth := t.synth(); {
		case synth.IsLeaf():
			return t, t
		case synth.IsOpen():
			start = t
			end = id.Wrap(t.Context(), synth.otherEnd)
		case synth.IsClose():
			start = id.Wrap(t.Context(), synth.otherEnd)
			end = t
		}

	case impl.IsLeaf():
		return t, t
	case impl.IsOpen():
		start = t
		end = id.Wrap(t.Context(), t.ID()+ID(impl.Offset()))
	case impl.IsClose():
		start = id.Wrap(t.Context(), t.ID()+ID(impl.Offset()))
		end = t
	}

	return
}

// Next returns the next token in this token's stream.
//
// Panics if this is not a natural token.
func (t Token) Next() Token {
	c := NewCursorAt(t)
	_ = c.Next()
	return c.Next()
}

// Prev returns the previous token in this token's stream.
//
// Panics if this is not a natural token.
func (t Token) Prev() Token {
	c := NewCursorAt(t)
	return c.Prev()
}

// Fuse marks a pair of tokens as their respective open and close.
//
// If open or close are synthetic or not currently a leaf, have different
// contexts, or are part of a frozen [Stream], this function panics.
func Fuse(open, close Token) { //nolint:predeclared,revive // For close.
	if open.Context() != close.Context() {
		panic("protocompile/token: attempted to fuse tokens from different streams")
	}
	if open.Context().frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	impl1 := open.nat()
	if impl1 == nil {
		panic("protocompile/token: called FuseTokens() with a synthetic open token")
	}
	if !impl1.IsLeaf() {
		panic("protocompile/token: called FuseTokens() with non-leaf as the open token")
	}

	impl2 := close.nat()
	if impl2 == nil {
		panic("protocompile/token: called FuseTokens() with a synthetic close token")
	}
	if !impl2.IsLeaf() {
		panic("protocompile/token: called FuseTokens() with non-leaf as the close token")
	}

	fuseImpl(int32(close.ID()-open.ID()), impl1, impl2)
}

// Children returns a Cursor over the children of this token.
//
// If the token is zero or is a leaf token, returns nil.
func (t Token) Children() *Cursor {
	if t.IsZero() || t.IsLeaf() {
		return nil
	}

	if impl := t.nat(); impl != nil {
		start, _ := t.StartEnd()
		return &Cursor{
			context: t.Context(),
			idx:     naturalIndex(start.ID()) + 1, // Skip the start!
		}
	}

	synth := t.synth()
	if synth.IsClose() {
		return id.Wrap(t.Context(), synth.otherEnd).Children()
	}
	return NewSliceCursor(t.Context(), synth.children)
}

// SyntheticChildren returns a cursor over the given subslice of the children
// of this token.
//
// Panics if t is not synthetic.
func (t Token) SyntheticChildren(i, j int) *Cursor {
	synth := t.synth()
	if synth == nil {
		panic("protocompile/token: called SyntheticChildren() on non-synthetic token")
	}
	if synth.IsClose() {
		return id.Wrap(t.Context(), synth.otherEnd).SyntheticChildren(i, j)
	}
	return NewSliceCursor(t.Context(), synth.children[i:j])
}

// Name converts this token into its corresponding identifier name, potentially
// performing normalization.
//
// Currently, we perform no normalization, so this is the same value as Text(), but
// that may change in the future.
//
// Returns "" for non-identifiers.
func (t Token) Name() string {
	if t.Kind() != Ident {
		return ""
	}
	return t.Text()
}

// AsNumber returns number information for this token.
func (t Token) AsNumber() NumberToken {
	if t.Kind() != Number {
		return NumberToken{}
	}
	return id.Wrap(t.Context(), id.ID[NumberToken](t.ID()))
}

// AsString returns string information for this token.
func (t Token) AsString() StringToken {
	if t.Kind() != String {
		return StringToken{}
	}
	return id.Wrap(t.Context(), id.ID[StringToken](t.ID()))
}

// String implements [strings.Stringer].
func (t Token) String() string {
	if debug && !t.IsZero() {
		if t.IsSynthetic() {
			return fmt.Sprintf("{%v %#v}", t.ID(), t.synth())
		}
		return fmt.Sprintf("{%v %#v}", t.ID(), t.nat())
	}

	return fmt.Sprintf("{%v %v}", t.ID(), t.Kind())
}

// offsets returns the byte offsets of this token within the file it came from.
//
// The return value for synthetic tokens is unspecified.
//
// Note that this DOES NOT include any child tokens!
func (t Token) offsets() (start, end int) {
	if t.IsSynthetic() {
		return
	}

	end = int(t.nat().end)
	// If this is the first token, the start is implicitly zero.
	if t.ID() == 1 {
		return 0, end
	}

	prev := id.Wrap(t.Context(), t.ID()-1)
	return int(prev.nat().end), end
}

func (t Token) nat() *nat {
	if t.IsSynthetic() {
		return nil
	}
	return &t.Context().nats[naturalIndex(t.ID())]
}

func (t Token) synth() *synth {
	if !t.IsSynthetic() {
		return nil
	}
	return &t.Context().synths[syntheticIndex(t.ID())]
}
