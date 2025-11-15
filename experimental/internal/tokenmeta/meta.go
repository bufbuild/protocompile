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

// Package meta defines internal metadata types shared between the token package
// and the lexer.
package tokenmeta

import (
	"math/big"
)

// Meta is a type defined in this package.
type Meta interface{ meta() }

type Number struct {
	// Inlined storage for small int/float values.
	Word uint64

	// big.Float can represent any uint64 or float64 (except NaN), and any
	// *big.Int, too.
	Big *big.Float

	// Length of a prefix or suffix on this integer.
	// The prefix is the base prefix; the suffix is any identifier
	// characters that follow the last digit.
	Prefix, Suffix uint32

	Exp          uint32 // Length of the exponent measured from the e.
	IsFloat      bool
	ThousandsSep bool

	Base, ExpBase byte

	SyntaxError bool // Whether parsing a concrete value failed.
}

type String struct {
	// Post-processed string contents.
	Text string

	// Lengths of the sigil and quotes for this string
	Prefix, Quote uint32

	// Whether concatenation took place.
	Concatenated bool

	// Spans at which escapes occur.
	Escapes []Escape
}

type Escape struct {
	Start, End uint32
	Rune       rune
	Byte       byte
}

func (Number) meta() {}
func (String) meta() {}
