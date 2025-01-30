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
	"strconv"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/report"
)

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
type Token struct {
	withContext
	id ID
}

// ID returns this token's raw ID, disassociated from its context. This is
// useful for storing tokens of some ambient context in a compressed manner.
//
// Calling t.ID().In(ctx) with any value other than t.Context() will result in
// unspecified behavior.
func (t Token) ID() ID {
	return t.id
}

// IsPaired returns whether this is a non-zero leaf token.
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
	return t.id < 0
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
	return t.Context().Stream().Text()[start:end]
}

// Span implements [Spanner].
func (t Token) Span() report.Span {
	if t.IsZero() || t.IsSynthetic() {
		return report.Span{}
	}

	var a, b int
	if !t.IsLeaf() {
		start, end := t.StartEnd()
		a, _ = start.offsets()
		_, b = end.offsets()
	} else {
		a, b = t.offsets()
	}

	return t.Context().Stream().Span(a, b)
}

// LeafSpan returns the span that this token would have if it was a leaf token.
func (t Token) LeafSpan() report.Span {
	if t.IsZero() || t.IsSynthetic() {
		return report.Span{}
	}

	return t.Context().Stream().Span(t.offsets())
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
			end = synth.otherEnd.In(t.Context())
		case synth.IsClose():
			start = synth.otherEnd.In(t.Context())
			end = t
		}

	case impl.IsLeaf():
		return t, t
	case impl.IsOpen():
		start = t
		end = (t.id + ID(impl.Offset())).In(t.Context())
	case impl.IsClose():
		start = (t.id + ID(impl.Offset())).In(t.Context())
		end = t
	}

	return
}

// SetValue sets the associated literal value with a token. The token must be
// of the appropriate kind ([Number] or [String]) for the literal.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func SetValue[T Value](token Token, value T) {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to SetValue: %s", token))
	}

	var wantKind Kind
	switch any(value).(type) {
	case uint64, float64, *big.Int:
		wantKind = Number
	case string:
		wantKind = String
	}

	if token.Kind() != wantKind {
		panic(fmt.Sprintf("protocompile/token: passed token of invalid kind to SetValue: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	if stream.literals == nil {
		stream.literals = map[ID]any{}
	}
	stream.literals[token.id] = value
}

// ClearValue clears the associated literal value of a token.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but is not for symmetry
// with [SetValue].
func ClearValue(token Token) {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to ClearValue: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	delete(stream.literals, token.id)
}

// Fuse marks a pair of tokens as their respective open and close.
//
// If open or close are synthetic or not currently a leaf, have different
// contexts, or are part of a frozen [Stream], this function panics.
func Fuse(open, close Token) { //nolint:predeclared,revive // For close.
	if open.Context().Stream() != close.Context().Stream() {
		panic("protocompile/token: attempted to fuse tokens from different streams")
	}
	if open.Context().Stream().frozen {
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

	fuseImpl(int32(close.id-open.id), impl1, impl2)
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
			withContext: t.withContext,
			idx:         start.id.naturalIndex() + 1, // Skip the start!
		}
	}

	synth := t.synth()
	if synth.IsClose() {
		return synth.otherEnd.In(t.Context()).Children()
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
	if t.Kind() != Ident {
		return ""
	}
	return t.Text()
}

// AsInt converts this token into an unsigned integer if it is an integer token.
// Returns 0, false if it is not an integer or the result would overflow a uint64.
func (t Token) AsInt() (uint64, bool) {
	if t.Kind() != Number {
		return 0, false
	}

	// Check if this number has already been parsed for us.
	vAny, present := t.Context().Stream().literals[t.id]
	if v, ok := vAny.(uint64); present && ok {
		return v, true
	}

	// Otherwise, it's a base 10 integer.
	v, err := strconv.ParseUint(t.Text(), 10, 64)
	return v, err == nil
}

// AsBigInt converts this token into a big integer if it is an integer token.
// Returns nil if it is not an integer.
func (t Token) AsBigInt() *big.Int {
	if t.Kind() != Number {
		return nil
	}

	// Check if this is a big integer.
	vAny, present := t.Context().Stream().literals[t.id]
	if v, ok := vAny.(*big.Int); present && ok {
		return v
	}

	// Otherwise, fall back to AsInt.
	v, ok := t.AsInt()
	if !ok {
		return nil
	}

	n := new(big.Int)
	n.SetUint64(v)
	return n
}

// AsFloat converts this token into float if it is a numeric token. If the value is
// not precisely representable as a float64, it is clamped to an infinity or
// rounded (ties-to-even).
//
// This function does not handle the special non-finite values inf and nan.
//
// Otherwise, returns 0.0, false.
func (t Token) AsFloat() (float64, bool) {
	if t.Kind() != Number {
		return 0, false
	}

	// Check if this number has already been parsed for us.
	vAny, present := t.Context().Stream().literals[t.id]
	if v, ok := vAny.(float64); present && ok {
		return v, true
	}
	if v, ok := vAny.(uint64); present && ok {
		return float64(v), true
	}
	if v, ok := vAny.(*big.Int); present && ok {
		f, _ := v.Float64()
		return f, true
	}

	// Otherwise, it's an base 10 integer.
	v, err := strconv.ParseUint(t.Text(), 10, 64)
	return float64(v), err == nil
}

// AsString converts this token into a Go string if it is in fact a string literal token.
//
// Otherwise, returns "", false.
func (t Token) AsString() (string, bool) {
	if t.Kind() != String {
		return "", false
	}

	// Synthetic strings don't have quotes around them and don't
	// contain escapes.
	if synth := t.synth(); synth != nil {
		return synth.text, true
	}

	// Check if there's an unescaped version of this string.
	v, present := t.Context().Stream().literals[t.id]
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
	if t.IsSynthetic() || t.Kind() != String {
		return false
	}
	_, present := t.Context().Stream().literals[t.id]
	return !present
}

// String implements [strings.Stringer].
func (t Token) String() string {
	return fmt.Sprintf("{%v %v}", t.id, t.Kind())
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
	if t.id == 1 {
		return 0, end
	}

	prev := (t.id - 1).In(t.Context())
	return int(prev.nat().end), end
}

func (t Token) nat() *nat {
	if t.IsSynthetic() {
		return nil
	}
	return &t.Context().Stream().nats[t.id.naturalIndex()]
}

func (t Token) synth() *synth {
	if !t.IsSynthetic() {
		return nil
	}
	return &t.Context().Stream().synths[t.id.syntheticIndex()]
}
