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

package ast2

// Number is a numeric literal.
type Number struct {
	withContext

	raw rawToken
}

// Token returns the underlying quoted literal token.
func (n Number) Token() Token {
	return n.raw.With(n)
}

// Span implements [Spanner] for Number.
func (n Number) Span() Span {
	return n.Token().Span()
}

// Int returns this Number's value as a signed integer, or false if it
// is not representable as such.
func (n Number) Int() (int64, bool) {
	return 0, false
}

// UInt returns this Number's value as an unsigned integer, or false if it
// is not representable as such.
func (n Number) UInt() (uint64, bool) {
	return 0, false
}

// Float returns this Number's value as a float, or false if it
// is not representable as such.
func (n Number) Float() (float64, bool) {
	return 0, false
}

// String is a string literal.
type String struct {
	withContext

	raw rawToken
}

// Token returns the underlying quoted literal token.
func (s String) Token() Token {
	return s.raw.With(s)
}

// Span implements [Spanner] for String.
func (s String) Span() Span {
	return s.Token().Span()
}

// Value returns this String's value as a Go string.
func (s String) Value() string {
	tok := s.Token()
	if tok.Nil() {
		return ""
	}

	// Synthetic strings don't have quotes around them and don't
	// contain escapes.
	if synth := s.Token().synthetic(); synth != nil {
		return synth.text
	}

	unescaped, ok := s.Context().stringValues[s.raw]
	if ok {
		return unescaped
	}

	// If it's not in the map, that means this is a single
	// leaf string whose quotes we can just pull off.

	text := tok.Text()
	return text[1 : len(text)-2]
}
