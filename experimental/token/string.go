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
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/report"
)

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
	return s.meta == nil || !(s.meta.Escaped || s.meta.Concatenated)
}

// Prefix returns an arbitrary prefix attached to this string (the prefix will
// have no whitespace before the open quote).
func (s StringToken) Prefix() report.Span {
	if s.meta == nil {
		return report.Span{}
	}

	span := s.Token().LeafSpan()
	span.End = span.Start + int(s.meta.Prefix)
	return span
}

// Quotes returns the opening and closing delimiters for this string literal,
// not including the sigil.
//
//nolint:revive,predeclared
func (s StringToken) Quotes() (open, close report.Span) {
	if s.IsZero() {
		return report.Span{}, report.Span{}
	}

	open = s.Token().LeafSpan()
	close = open

	if s.meta == nil {
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

	open.Start += int(s.meta.Prefix)
	close.Start += int(s.meta.Prefix)

	quote := int(max(1, s.meta.Quote)) // 1 byte quotes if not set explicitly.

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
func (s StringToken) RawContent() report.Span {
	open, close := s.Quotes() //nolint:revive,predeclared
	open.Start = open.End
	open.End = close.Start
	return open
}
