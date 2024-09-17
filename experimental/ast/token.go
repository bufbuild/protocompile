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
	"strconv"
	"strings"
	"unicode"
)

// Constants for extracting the parts of tokenImpl.kindAndOffset.
const (
	tokenKindMask    = 0b111
	tokenOffsetShift = 3
)

const (
	TokenUnrecognized TokenKind = iota // Unrecognized garbage in the input file.

	TokenSpace   // Non-comment contiguous whitespace.
	TokenComment // A single comment.
	TokenIdent   // An identifier.
	TokenString  // A string token. May be a non-leaf for non-contiguous quoted strings.
	TokenNumber  // A run of digits that is some kind of number.
	TokenPunct   // Some punctuation. May be a non-leaf for delimiters like {}.
	_TokenUnused // Reserved for future use.

	// DO NOT ADD MORE TOKEN KINDS: ONLY THREE BITS ARE AVAILABLE
	// TO STORE THEM.
)

// TokenKind identifies what kind of token a particular [Token] is.
type TokenKind byte

// IsSkippable returns whether this is a token that should be examined during
// syntactic analysis.
func (t TokenKind) IsSkippable() bool {
	return t == TokenSpace || t == TokenComment || t == TokenUnrecognized
}

// String implements [strings.Stringer] for TokenKind.
func (t TokenKind) String() string {
	switch t {
	case TokenUnrecognized:
		return "TokenUnrecognized"
	case TokenSpace:
		return "TokenSpace"
	case TokenComment:
		return "TokenComment"
	case TokenIdent:
		return "TokenIdent"
	case TokenString:
		return "TokenString"
	case TokenNumber:
		return "TokenNumber"
	case TokenPunct:
		return "TokenPunct"
	default:
		return fmt.Sprintf("TokenKind(%d)", int(t))
	}
}

// Token is a lexical element of a Protobuf file.
//
// Protocompile's token stream is actually a tree of tokens. Some tokens, called
// non-leaf tokens, contain a selection of tokens "within" them. For example, the
// two matched braces of a message body are a single token, and all of the tokens
// between the braces are contained inside it. This moves certain complexity into
// the lexer in a way that allows us to handle matching delimiters generically.
//
// The zero value of Token is the so-called "nil token", which is used to denote the
// absence of a token.
type Token struct {
	withContext

	raw rawToken
}

// IsPaired returns whether this is a non-nil leaf token.
func (t Token) IsLeaf() bool {
	if t.Nil() {
		return false
	}

	if impl := t.impl(); impl != nil {
		return impl.IsLeaf()
	}
	return t.synthetic().IsLeaf()
}

// IsSynthetic returns whether this is a non-nil synthetic token (i.e., a token that didn't
// come from a parsing operation.)
func (t Token) IsSynthetic() bool {
	return t.raw < 0
}

// Kind returns what kind of token this is.
//
// Returns [TokenUnrecognized] if this token is nil.
func (t Token) Kind() TokenKind {
	if t.Nil() {
		return TokenUnrecognized
	}

	if impl := t.impl(); impl != nil {
		return impl.Kind()
	}
	return t.synthetic().kind
}

// Text returns the text fragment referred to by this token.
// Note that this DOES NOT include any child tokens!
//
// Returns empty string fot the nil token.
func (t Token) Text() string {
	if t.Nil() {
		return ""
	}

	if synth := t.synthetic(); synth != nil {
		if synth.kind == TokenString {
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
				case r == '\x00':
					escaped.WriteString("\\0")
				case r == '"':
					escaped.WriteString("\\\"")
				case r == '\\':
					escaped.WriteString("\\\\")
				case r < ' ':
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

// Span implements [Spanner] for Token.
func (t Token) Span() Span {
	if t.Nil() || t.IsSynthetic() {
		return Span{}
	}

	if !t.IsLeaf() {
		start, end := t.StartEnd()
		a, _ := start.offsets()
		_, b := end.offsets()

		return t.Context().NewSpan(a, b)
	}

	return t.Context().NewSpan(t.offsets())
}

// StartEnd returns the open and close tokens for this token.
//
// If this is a leaf token, start and end will be the same token and will compare as equal.
//
// Panics if this is a nil token.
func (t Token) StartEnd() (start, end Token) {
	t.panicIfNil()

	switch impl := t.impl(); {
	case impl == nil:
		switch synth := t.synthetic(); {
		case synth.IsLeaf():
			return t, t
		case synth.IsOpen():
			start = t
			end = synth.otherEnd.With(t)
		case synth.IsClose():
			start = synth.otherEnd.With(t)
			end = t
		}

	case impl.IsLeaf():
		return t, t
	case impl.IsOpen():
		start = t
		end = (t.raw + rawToken(impl.Offset())).With(t)
	case impl.IsClose():
		start = (t.raw + rawToken(impl.Offset())).With(t)
		end = t
	}

	return
}

// Offsets returns the byte offsets of this token within the file it came from.
//
// The return value for synthetic tokens is unspecified.
//
// Note that this DOES NOT include any child tokens!
func (t Token) offsets() (start, end int) {
	if t.IsSynthetic() {
		return
	}

	end = int(t.impl().end)
	// If this is the first token, the start is implicitly zero.
	if t.raw == 1 {
		return 0, end
	}

	prev := (t.raw - 1).With(t)
	return int(prev.impl().end), end
}

// Children returns a Cursor over the children of this token.
//
// If the token is nil or is a leaf token, returns nil.
func (t Token) Children() *Cursor {
	if t.Nil() || t.IsLeaf() {
		return nil
	}

	if impl := t.impl(); impl != nil {
		start, end := t.StartEnd()
		return &Cursor{
			withContext: t.withContext,
			start:       start.raw + 1, // Skip the start!
			end:         end.raw,
		}
	}

	synth := t.synthetic()
	if synth.IsClose() {
		return synth.otherEnd.With(t).Children()
	}
	return &Cursor{
		withContext: t.withContext,
		stream:      synth.children,
	}
}

// Name converts this token into its corresponding identifier name, potentially
// performing normalization.
//
// Currently, we perform no normalization, so this is the same value as Text(), but
// that may change in the future.
//
// Returns "" for non-identifiers.
func (t Token) Name() string {
	if t.Kind() != TokenIdent {
		return ""
	}
	return t.Text()
}

// AsUInt converts this token into an unsigned integer if it is a numeric token.
// bits is the maximum number of bits that are used to represent this value.
//
// Otherwise, or if the result would overflow, returns 0, false.
func (t Token) AsInt() (uint64, bool) {
	if t.Kind() != TokenNumber {
		return 0, false
	}

	// Check if this number has already been parsed for us.
	vAny, present := t.Context().literals[t.raw]
	if v, ok := vAny.(uint64); present && ok {
		return v, true
	}

	// Otherwise, it's an base 10 integer.
	v, err := strconv.ParseUint(t.Text(), 10, 64)
	return v, err == nil
}

// AsFloat converts this token into float if it is a numeric token. If the value is
// not precisely representable as a float64, it is clamped to an infinity or
// rounded (ties-to-even).
//
// This function does not handle the special non-finite values inf and nan.
//
// Otherwise, returns 0.0, false.
func (t Token) AsFloat() (float64, bool) {
	if t.Kind() != TokenNumber {
		return 0, false
	}

	// Check if this number has already been parsed for us.
	vAny, present := t.Context().literals[t.raw]
	if v, ok := vAny.(float64); present && ok {
		return v, true
	}
	if v, ok := vAny.(uint64); present && ok {
		return float64(v), true
	}

	// Otherwise, it's an base 10 integer.
	v, err := strconv.ParseUint(t.Text(), 10, 64)
	return float64(v), err == nil
}

// AsString converts this token into a Go string if it is in fact a string literal token.
//
// Otherwise, returns "", false.
func (t Token) AsString() (string, bool) {
	if t.Kind() != TokenString {
		return "", false
	}

	// Synthetic strings don't have quotes around them and don't
	// contain escapes.
	if synth := t.synthetic(); synth != nil {
		return synth.text, true
	}

	// Check if there's an unescaped version of this string.
	v, present := t.Context().literals[t.raw]
	if unescaped, ok := v.(string); present && ok {
		return unescaped, true
	}

	// If it's not in the map, that means this is a single
	// leaf string whose quotes we can just pull of off the
	// token, after removing the quotes.
	text := t.Text()
	if len(text) < 2 {
		// Some kind of invalid, unterminated string token.
		return "", true
	}
	return text[1 : len(text)-1], true
}

// IsPureString returns whether this token was parsed from a string literal
// that did not need post-processing after being parsed.
//
// Returns false for synthetic tokens.
func (t Token) IsPureString() bool {
	if t.IsSynthetic() || t.Kind() != TokenString {
		return false
	}
	_, present := t.Context().literals[t.raw]
	return !present
}

// String implements [strings.Stringer] for Token.
func (t Token) String() string {
	return t.raw.String()
}

func (t Token) impl() *tokenImpl {
	t.panicIfNil()

	if t.IsSynthetic() {
		return nil
	}
	// Need to subtract off one, because the zeroth
	// rawToken is used as a "missing" sentinel.
	return &t.ctx.stream[t.raw-1]
}

func (t Token) synthetic() *tokenSynthetic {
	t.panicIfNil()

	if !t.IsSynthetic() {
		return nil
	}
	return &t.ctx.syntheticTokens[^t.raw]
}

// Cursor is an iterator-like construct for looping over a token tree.
// Unlike a plain range func, it supports peeking.
type Cursor struct {
	withContext

	// This is used if this is a cursor over non-synthetic tokens.
	// start is inclusive, end is exclusive. start == end means the stream
	// is empty.
	start, end rawToken
	// This is used if this is a cursor over the children of a synthetic token.
	// If stream is nil, we know we're in the non-synthetic case.
	stream []rawToken
	idx    int
}

// CursorMark is the return value of [Cursor.Mark], which marks a position on
// a Cursor for rewinding to.
type CursorMark struct {
	// This contains exactly the values needed to rewind the cursor.
	owner *Cursor
	start rawToken
	idx   int
}

// Done returns whether or not there are still tokens left to yield.
func (c *Cursor) Done() bool {
	return c.Peek().Nil()
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
// Returns the nil token if this cursor is at the end of the stream.
func (c *Cursor) PeekSkippable() Token {
	if c == nil {
		return Token{}
	}

	if c.IsSynthetic() {
		if c.idx == len(c.stream) {
			return Token{}
		}
		return c.stream[c.idx].With(c)
	}
	if c.start >= c.end {
		return Token{}
	}
	return c.start.With(c)
}

// Pop returns the next skippable token in the sequence, and advances the cursor.
func (c *Cursor) PopSkippable() Token {
	tok := c.PeekSkippable()
	if tok.Nil() {
		return tok
	}

	if c.IsSynthetic() {
		c.idx++
	} else {
		impl := c.start.With(c).impl()
		if impl.Offset() > 0 {
			c.start += rawToken(impl.Offset())
		}
		c.start++
	}
	return tok
}

// Peek returns the next token in the sequence, if there is one.
// This automatically skips past skippable tokens.
//
// Returns the nil token if this cursor is at the end of the stream.
func (c *Cursor) Peek() Token {
	for {
		next := c.PeekSkippable()
		if next.Nil() || !next.Kind().IsSkippable() {
			return next
		}
		c.PopSkippable()
	}
}

// Pop returns the next token in the sequence, and advances the cursor.
func (c *Cursor) Pop() Token {
	tok := c.Peek()
	if tok.Nil() {
		return tok
	}

	return c.PopSkippable()
}

// Iter is an iterator over the remaining tokens in the cursor.
//
// Note that breaking out of a loop over this iterator, and starting
// a new loop, will resume at the iteration that was broken at. E.g., if
// we break out of a loop over c.Iter at token tok, and start a new range
// over c.Iter, the first yielded token will be tok.
func (c *Cursor) Iter(yield func(Token) bool) {
	for {
		tok := c.Peek()
		if tok.Nil() || !yield(tok) {
			break
		}
		_ = c.Pop()
	}
}

// IterSkippable is like [Cursor.Iter]. but it yields skippable tokens, too.
//
// Note that breaking out of a loop over this iterator, and starting
// a new loop, will resume at the iteration that was broken at. E.g., if
// we break out of a loop over c.Iter at token tok, and start a new range
// over c.Iter, the first yielded token will be tok.
func (c *Cursor) IterSkippable(yield func(Token) bool) {
	for {
		tok := c.PeekSkippable()
		if tok.Nil() || !yield(tok) {
			break
		}
		_ = c.PopSkippable()
	}
}

// IsSynthetic returns whether this is a cursor over synthetic tokens.
func (c *Cursor) IsSynthetic() bool {
	return c.stream != nil
}

// ** PRIVATE ** //

// rawToken is the ID of a token separated from its context.
//
// Let n := int(id). If n is zero, it is the nil token. If n is positive, it is
// a non-synthetic token, whose index is n - 1. If it is negative, it is a
// synthetic token, whose index is ^n.
type rawToken int32

// Wrap wraps this rawToken with a context to present to the user.
func (t rawToken) With(c Contextual) Token {
	if t == 0 {
		return Token{}
	}
	return Token{withContext{c.Context()}, t}
}

func (t rawToken) String() string {
	if t == 0 {
		return "Token(<nil>)"
	}
	if t < 0 {
		return fmt.Sprintf("Token(synth#%d)", ^int(t))
	}

	return fmt.Sprintf("Token(%d)", int(t)-1)
}

// tokenImpl is the data of a token stored in a [Context].
type tokenImpl struct {
	// We store the end of the token, and the start is implicitly
	// given by the end of the previous token. We use the end, rather
	// than the start, it makes adding tokens one by one to the stream
	// easier, because once the token is pushed, its start and end are
	// set correctly, and don't depend on the next token being pushed.
	end           uint32
	kindAndOffset int32
}

// Kind extracts the token's kind, which is stored.
func (t tokenImpl) Kind() TokenKind {
	return TokenKind(t.kindAndOffset & tokenKindMask)
}

// Offset returns the offset from this token to its matching open/close, if any.
func (t tokenImpl) Offset() int {
	return int(t.kindAndOffset >> tokenOffsetShift)
}

// IsLeaf checks whether this is a leaf token.
func (t tokenImpl) IsLeaf() bool {
	return t.Offset() == 0
}

// IsLeaf checks whether this is a open token with a matching closer.
func (t tokenImpl) IsOpen() bool {
	return t.Offset() > 0
}

// IsLeaf checks whether this is a closer token with a matching opener.
func (t tokenImpl) IsClose() bool {
	return t.Offset() < 0
}

// tokenSynthetic is the data of a synthetic token stored in a [Context].
type tokenSynthetic struct {
	text string
	kind TokenKind

	// Non-zero if this token has a matching other end. Whether this is
	// the opener or the closer is determined by whether children is
	// nil: it is nil for the closer.
	otherEnd rawToken
	children []rawToken
}

// IsLeaf checks whether this is a leaf token.
func (t tokenSynthetic) IsLeaf() bool {
	return t.otherEnd == 0
}

// IsLeaf checks whether this is a open token with a matching closer.
func (t tokenSynthetic) IsOpen() bool {
	return !t.IsLeaf() && t.children != nil
}

// IsLeaf checks whether this is a closer token with a matching opener.
func (t tokenSynthetic) IsClose() bool {
	return !t.IsLeaf() && t.children == nil
}
