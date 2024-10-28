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

package token

import "fmt"

const (
	Unrecognized Kind = iota // Unrecognized garbage in the input file.

	Space       // Non-comment contiguous whitespace.
	Comment     // A single comment.
	Ident       // An identifier.
	String      // A string token. May be a non-leaf for non-contiguous quoted strings.
	Number      // A run of digits that is some kind of number.
	Punct       // Some punctuation. May be a non-leaf for delimiters like {}.
	_KindUnused // Reserved for future use.

	// DO NOT ADD MORE TOKEN KINDS: ONLY THREE BITS ARE AVAILABLE
	// TO STORE THEM.
)

// Kind identifies what kind of token a particular [Token] is.
type Kind byte

// IsSkippable returns whether this is a token that should be examined during
// syntactic analysis.
func (t Kind) IsSkippable() bool {
	return t == Space || t == Comment || t == Unrecognized
}

// String implements [strings.Stringer].
func (t Kind) String() string {
	switch t {
	case Unrecognized:
		return "Unrecognized"
	case Space:
		return "Space"
	case Comment:
		return "Comment"
	case Ident:
		return "Ident"
	case String:
		return "String"
	case Number:
		return "Number"
	case Punct:
		return "Punct"
	default:
		return fmt.Sprintf("token.Kind(%d)", int(t))
	}
}
