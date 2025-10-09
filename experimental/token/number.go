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

// Base returns this number's base.
func (n NumberToken) Base() byte {
	switch n.Prefix().Text() {
	case "0b", "0B":
		return 2
	case "0", "0o", "0O":
		return 8
	case "0x", "0X":
		return 16
	default:
		return 10
	}
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

// IsFloat returns whether this token can only be used as a float literal (even
// if it has integer value).
func (n NumberToken) IsFloat() bool {
	return n.meta != nil && n.meta.IsFloat
}

// HasSeparators returns whether this token contains thousands separator
// runes.
func (n NumberToken) HasSeparators() bool {
	return n.meta != nil && n.meta.ThousandsSep
}

// IsValid returns whether this token was able to parse properly at all.
func (n NumberToken) IsValid() bool {
	return n.meta == nil || !n.meta.SyntaxError
}

// Int converts this value into a 64-bit unsigned integer.
//
// Returns whether the conversion was exact.
func (n NumberToken) Int() (v uint64, exact bool) {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return v, err == nil
	}

	switch {
	case n.meta.Big != nil:
		v, acc := n.meta.Big.Uint64()
		return v, acc == big.Exact
	case n.meta.IsFloat:
		f := math.Float64frombits(n.meta.Word)
		n := uint64(f)
		return n, f == float64(n)
	default:
		return n.meta.Word, true
	}
}

// Float converts this value into a 64-bit float.
//
// Returns whether the conversion was exact.
func (n NumberToken) Float() (v float64, exact bool) {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, err := strconv.ParseUint(n.Token().Text(), 10, 64)
		return float64(v), err == nil && uint64(float64(v)) == v
	}

	switch {
	case n.meta.Big != nil:
		v, acc := n.meta.Big.Float64()
		return v, acc == big.Exact
	case n.meta.IsFloat:
		f := math.Float64frombits(n.meta.Word)
		return f, true
	default:
		v := n.meta.Word
		return float64(v), uint64(float64(v)) == v
	}
}

// Value returns the underlying arbitrary-precision numeric value.
func (n NumberToken) Value() *big.Float {
	if n.meta == nil {
		// This is a decimal integer, so we just parse on the fly.
		v, _ := strconv.ParseUint(n.Token().Text(), 10, 64)
		return new(big.Float).SetUint64(v)
	}

	switch {
	case n.meta.Big != nil:
		return n.meta.Big
	case n.meta.IsFloat:
		f := math.Float64frombits(n.meta.Word)
		return new(big.Float).SetFloat64(f)
	default:
		v := n.meta.Word
		return new(big.Float).SetUint64(v)
	}
}
