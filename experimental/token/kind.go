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

// Code generated by github.com/bufbuild/protocompile/internal/enum kind.yaml. DO NOT EDIT.

package token

import (
	"fmt"
	"iter"
)

// Kind identifies what kind of token a particular [Token] is.
type Kind byte

const (
	Unrecognized Kind = iota // Unrecognized garbage in the input file.
	Space                    // Non-comment contiguous whitespace.
	Comment                  // A single comment.
	Ident                    // An identifier.
	String                   // A string token. May be a non-leaf for non-contiguous quoted strings.
	Number                   // A run of digits that is some kind of number.
	Punct                    // Some punctuation. May be a non-leaf for delimiters like {}.
)

// String implements [fmt.Stringer].
func (v Kind) String() string {
	if int(v) < 0 || int(v) > len(_table_Kind_String) {
		return fmt.Sprintf("Kind(%v)", int(v))
	}
	return _table_Kind_String[int(v)]
}

// GoString implements [fmt.GoStringer].
func (v Kind) GoString() string {
	if int(v) < 0 || int(v) > len(_table_Kind_GoString) {
		return fmt.Sprintf("tokenKind(%v)", int(v))
	}
	return _table_Kind_GoString[int(v)]
}

var _table_Kind_String = [...]string{
	Unrecognized: "Unrecognized",
	Space:        "Space",
	Comment:      "Comment",
	Ident:        "Ident",
	String:       "String",
	Number:       "Number",
	Punct:        "Punct",
}

var _table_Kind_GoString = [...]string{
	Unrecognized: "Unrecognized",
	Space:        "Space",
	Comment:      "Comment",
	Ident:        "Ident",
	String:       "String",
	Number:       "Number",
	Punct:        "Punct",
}
var _ iter.Seq[int] // Mark iter as used.
