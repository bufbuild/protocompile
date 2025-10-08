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

	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
)

// NumberToken provides access to detailed information about a [Number].
type NumberToken struct {
	withContext
	token ID
	meta  *tokenmeta.Number
}

// Token returns the wrapped token value.
func (n NumberToken) Token() Token {
	return n.token.In(n.Context())
}

// Prefix returns this number's base prefix (e.g. 0x).
func (n NumberToken) Prefix() report.Span {
	if n.meta == nil {
		return report.Span{}
	}

	span := n.Token().Span()
	span.End = span.Start + int(n.meta.Prefix)
	return span
}

// Suffix returns an arbitrary suffix attached to this number (the suffix will
// have no whitespace before the end of the digits).
func (n NumberToken) Suffix() report.Span {
	if n.meta == nil {
		return report.Span{}
	}

	span := n.Token().Span()
	span.Start = span.End + int(n.meta.Suffix)
	return span
}

// AsInt converts this value into a 64-bit unsigned integer.
//
// Returns whether the conversion was exact.
func (n NumberToken) AsInt() (v uint64, exact bool) {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return v, err == nil
	}

	switch {
	case n.meta.Big != nil:
		return n.meta.Big.Uint64(), n.meta.Big.IsUint64()
	case n.meta.Float != 0:
		f := n.meta.Float
		n := uint64(f)
		return n, f == float64(n)
	default:
		return n.meta.Word, true
	}
}

// AsFloat converts this value into a 64-bit float.
//
// Returns whether the conversion was exact.
func (n NumberToken) AsFloat() (v float64, exact bool) {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return float64(v), err == nil && uint64(float64(v)) == v
	}

	switch {
	case n.meta.Big != nil:
		v, acc := new(big.Float).SetInt(n.meta.Big).Float64()
		return v, acc == big.Exact
	case n.meta.Float != 0:
		return n.meta.Float, true
	default:
		v := n.meta.Word
		return float64(v), uint64(float64(v)) == v
	}
}

// AsBig converts this value into a big integer.
//
// Returns whether the conversion was exact.
func (n NumberToken) AsBig() (v *big.Int, exact bool) {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return new(big.Int).SetUint64(v), err == nil
	}

	switch {
	case n.meta.Big != nil:
		return n.meta.Big, true
	case n.meta.Float != 0:
		v, acc := new(big.Float).SetFloat64(n.meta.Float).Int(nil)
		return v, acc == big.Exact
	default:
		return new(big.Int).SetUint64(n.meta.Word), true
	}
}

// StringToken provides access to detailed information about a [String].
type StringToken struct {
	withContext
	token ID
	meta  *tokenmeta.String
}

// Token returns the wrapped token value.
func (s StringToken) Token() Token {
	return s.token.In(s.Context())
}

// Text returns the post-processed contents of this string.
func (s StringToken) Text() string {
	if s.meta != nil && s.meta.Text != "" {
		return s.meta.Text
	}
	return s.RawContent().Text()
}

// HasEscapes returns whether the string had escapes which were processed.
func (s StringToken) HasEscapes() bool {
	return s.meta != nil && s.meta.Escaped
}

// IsConcatenated returns whether the string was built from
// implicitly-concatenated strings.
func (s StringToken) IsConcatenated() bool {
	return s.meta != nil && s.meta.Concatenated
}

// IsPure returns whether the string required post-processing (escaping or
// concatenation) after lexing.
func (s StringToken) IsPure() bool {
	return s.meta != nil && (s.meta.Escaped || s.meta.Concatenated)
}

// Sigil returns an arbitrary prefix attached to this string (the prefix will
// have no whitespace before the open quote).
func (s StringToken) Sigil() report.Span {
	if s.meta == nil {
		return report.Span{}
	}

	span := s.Token().Span()
	span.End = span.Start + int(s.meta.Sigil)
	return span
}

// Quotes returns the opening and closing delimiters for this string literal,
// not including the sigil.
func (s StringToken) Quotes() (open, close report.Span) {
	if s.IsZero() {
		return report.Span{}, report.Span{}
	}

	open = s.Token().Span()
	close = open

	if s.meta == nil {
		// Assume that the quotes are a single byte wide if we don't have any
		// metadata.
		open.End = open.Start + 1
		close.Start = close.End - 1
		return open, close
	}

	quote := max(1, s.meta.Quote) // 1 byte quotes if not set explicitly.

	open.End = open.Start + int(s.meta.Sigil) + int(quote)
	close.Start = close.End - int(quote)
	return open, close
}

// RawContent returns the unprocessed contents of the string.
func (s StringToken) RawContent() report.Span {
	if s.IsZero() {
		return report.Span{}
	}

	span := s.Token().Span()

	if s.meta == nil {
		// Assume that the quotes are a single byte wide if we don't have any
		// metadata.
		span.Start++
		span.End--
		return span
	}

	span.Start += int(s.meta.Sigil) + int(s.meta.Quote)
	span.End -= int(s.meta.Quote)
	return span
}

// GetMeta returns the metadata value associated with this token. This function
// cannot be called outside of protocompile.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func GetMeta[M tokenmeta.Meta](token Token) *M {
	stream := token.Context().Stream()
	if meta, ok := stream.meta[token.ID()].(*M); ok {
		return meta
	}
	return nil
}

// MutateMeta is like [GetMeta], but it first initializes the meta value.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func MutateMeta[M tokenmeta.Meta](token Token) *M {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to MutateMeta: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	if stream.meta == nil {
		stream.meta = make(map[ID]any)
	}

	meta, _ := stream.meta[token.id].(*M)
	if meta == nil {
		meta = new(M)
		stream.meta[token.id] = meta
	}

	return meta
}

// ClearMeta clears the associated literal value of a token.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func ClearMeta[M tokenmeta.Meta](token Token) {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to ClearMeta: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	meta, _ := stream.meta[token.id].(*M)
	if meta != nil {
		delete(stream.meta, token.id)
	}
}
