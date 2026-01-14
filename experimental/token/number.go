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
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
	"github.com/bufbuild/protocompile/experimental/source"
)

// NumberToken provides access to detailed information about a [Number].
type NumberToken id.Node[NumberToken, *Stream, *tokenmeta.Number]

// Token returns the wrapped token value.
func (n NumberToken) Token() Token {
	return id.Wrap(n.Context(), ID(n.ID()))
}

// Base returns this number's base.
func (n NumberToken) Base() byte {
	if n.Raw() == nil {
		return 10
	}
	base := n.Raw().Base
	if base == 0 {
		return 10
	}
	return base
}

// IsLegacyOctal returns whether this is a C-style octal literal, such as 0777,
// as opposed to a modern octal literal like 0o777.
func (n NumberToken) IsLegacyOctal() bool {
	if n.Base() != 8 {
		return false
	}

	text := n.Token().Text()
	return !strings.HasPrefix(text, "0o") && !strings.HasPrefix(text, "0O")
}

// ExpBase returns this number's exponent base, if this number has an exponent;
// returns 1 if it has no exponent.
func (n NumberToken) ExpBase() int {
	if n.Raw() == nil {
		return 1
	}
	return max(1, int(n.Raw().ExpBase))
}

// Prefix returns this number's base prefix (e.g. 0x).
func (n NumberToken) Prefix() source.Span {
	if n.Raw() == nil || n.Raw().Prefix == 0 {
		return source.Span{}
	}

	span := n.Token().Span()
	span.End = span.Start + int(n.Raw().Prefix)
	return span
}

// Suffix returns an arbitrary suffix attached to this number (the suffix will
// have no whitespace before the end of the digits).
func (n NumberToken) Suffix() source.Span {
	if n.Raw() == nil || n.Raw().Prefix == 0 {
		return source.Span{}
	}

	span := n.Token().Span()
	span.Start = span.End - int(n.Raw().Suffix)
	return span
}

// Mantissa returns the mantissa digits for this literal, i.e., everything
// between the prefix and the (possibly empty) exponent.
//
// For example, for 0x123.456p-789, this will be 123.456.
func (n NumberToken) Mantissa() source.Span {
	span := n.Token().Span()
	if n.Raw() == nil {
		return span
	}

	start := int(n.Raw().Prefix)
	end := span.Len() - int(n.Raw().Suffix) - int(n.Raw().Exp)
	return span.Range(start, end)
}

// Exponent returns the exponent digits for this literal, i.e., everything
// after the exponent letter. Returns the zero span if there is no exponent.
//
// For example, for 0x123.456p-789, this will be -789.
func (n NumberToken) Exponent() source.Span {
	if n.Raw() == nil || n.Raw().Exp == 0 {
		return source.Span{}
	}

	span := n.Token().Span()
	end := span.Len() - int(n.Raw().Suffix)
	start := end - int(n.Raw().Exp) + 1 // Skip the exponent letter.

	return span.Range(start, end)
}

// IsFloat returns whether this token can only be used as a float literal (even
// if it has integer value).
func (n NumberToken) IsFloat() bool {
	return n.Raw() != nil && n.Raw().IsFloat
}

// HasSeparators returns whether this token contains thousands separator
// runes.
func (n NumberToken) HasSeparators() bool {
	return n.Raw() != nil && n.Raw().ThousandsSep
}

// IsValid returns whether this token was able to parse properly at all.
func (n NumberToken) IsValid() bool {
	return n.Raw() == nil || !n.Raw().SyntaxError
}

// Int converts this value into a 64-bit unsigned integer.
//
// Returns whether the conversion was exact.
func (n NumberToken) Int() (v uint64, exact bool) {
	if n.Raw() == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return v, err == nil
	}

	switch {
	case n.Raw().Big != nil:
		v, acc := n.Raw().Big.Uint64()
		return v, acc == big.Exact && n.Raw().Big.IsInt()
	case n.Raw().IsFloat:
		f := math.Float64frombits(n.Raw().Word)
		n := uint64(f)
		return n, f == float64(n)
	default:
		return n.Raw().Word, true
	}
}

// Float converts this value into a 64-bit float.
//
// Returns whether the conversion was exact.
func (n NumberToken) Float() (v float64, exact bool) {
	if n.Raw() == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return float64(v), err == nil && uint64(float64(v)) == v
	}

	switch {
	case n.Raw().Big != nil:
		v, acc := n.Raw().Big.Float64()
		return v, acc == big.Exact
	case n.Raw().IsFloat:
		f := math.Float64frombits(n.Raw().Word)
		return f, true
	default:
		v := n.Raw().Word
		return float64(v), uint64(float64(v)) == v
	}
}

// Value returns the underlying arbitrary-precision numeric value.
func (n NumberToken) Value() *big.Float {
	if n.Raw() == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, _ := strconv.ParseUint(n.Token().Text(), 10, 64)
		return new(big.Float).SetUint64(v)
	}

	switch {
	case n.Raw().Big != nil:
		return n.Raw().Big
	case n.Raw().IsFloat:
		f := math.Float64frombits(n.Raw().Word)
		return new(big.Float).SetFloat64(f)
	default:
		v := n.Raw().Word
		return new(big.Float).SetUint64(v)
	}
}
