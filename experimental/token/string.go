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
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
)

// StringToken provides access to detailed information about a [String].
type StringToken id.Node[StringToken, *Stream, *tokenmeta.String]

// Escape is an escape inside of a [StringToken]. See [StringToken.Escapes].
type Escape struct {
	source.Span

	// If Rune is zero, this escape represents a raw byte rather than a
	// Unicode character.
	Rune rune
	Byte byte
}

// Token returns the wrapped token value.
func (s StringToken) Token() Token {
	return id.Wrap(s.Context(), ID(s.ID()))
}

// Text returns the post-processed contents of this string.
func (s StringToken) Text() string {
	if s.Raw() != nil && s.Raw().Text != "" {
		return s.Raw().Text
	}
	return s.RawContent().Text()
}

// HasEscapes returns whether the string had escapes which were processed.
func (s StringToken) HasEscapes() bool {
	return s.Raw() != nil && s.Raw().Escapes != nil
}

// Escapes returns the escapes that contribute to the value of this string.
func (s StringToken) Escapes() seq.Indexer[Escape] {
	var spans []tokenmeta.Escape
	if s.Raw() != nil {
		spans = s.Raw().Escapes
	}

	return seq.NewFixedSlice(spans, func(_ int, esc tokenmeta.Escape) Escape {
		return Escape{
			Span: s.Token().Context().Span(int(esc.Start), int(esc.End)),
			Rune: esc.Rune,
			Byte: esc.Byte,
		}
	})
}

// IsConcatenated returns whether the string was built from
// implicitly-concatenated strings.
func (s StringToken) IsConcatenated() bool {
	return s.Raw() != nil && s.Raw().Concatenated
}

// IsPure returns whether the string required post-processing (escaping or
// concatenation) after lexing.
func (s StringToken) IsPure() bool {
	return s.Raw() == nil || !(s.Raw().Escapes != nil || s.Raw().Concatenated)
}

// Prefix returns an arbitrary prefix attached to this string (the prefix will
// have no whitespace before the open quote).
func (s StringToken) Prefix() source.Span {
	if s.Raw() == nil {
		return source.Span{}
	}

	span := s.Token().LeafSpan()
	span.End = span.Start + int(s.Raw().Prefix)
	return span
}

// Quotes returns the opening and closing delimiters for this string literal,
// not including the sigil.
//
//nolint:revive,predeclared
func (s StringToken) Quotes() (open, close source.Span) {
	if s.IsZero() {
		return source.Span{}, source.Span{}
	}

	open = s.Token().LeafSpan()
	close = open

	if s.Raw() == nil {
		if open.Len() < 2 {
			// Deal with the really degenerate case of a single quote.
			close.Start = close.End
			return open, close
		}

		// Assume that the quotes are a single byte wide if we don't have any
		// metadata.
		open.End = open.Start + 1
		close.Start = close.End - 1
		return open, close
	}

	open.Start += int(s.Raw().Prefix)
	close.Start += int(s.Raw().Prefix)

	quote := int(max(1, s.Raw().Quote)) // 1 byte quotes if not set explicitly.

	// Unterminated?
	switch {
	case open.Len() < quote:
		close.Start = close.End
	case open.Len() < 2*quote:
		open.End = open.Start + quote
		close.Start = open.End
	default:
		open.End = open.Start + quote
		close.Start = close.End - quote
	}

	return open, close
}

// RawContent returns the unprocessed contents of the string.
func (s StringToken) RawContent() source.Span {
	open, close := s.Quotes() //nolint:revive,predeclared
	open.Start = open.End
	open.End = close.Start
	return open
}
