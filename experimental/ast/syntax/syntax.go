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

// Code generated by github.com/bufbuild/protocompile/internal/enum syntax.yaml. DO NOT EDIT.

package syntax

import (
	"fmt"
	"iter"
)

// Syntax is a known syntax pragma.
//
// Not only does this include "proto2" and "proto3", but also all of the
// editions.
type Syntax int

const (
	Unknown Syntax = iota
	Proto2
	Proto3
	Edition2023
)

// String implements [fmt.Stringer].
func (v Syntax) String() string {
	if int(v) < 0 || int(v) > len(_table_Syntax_String) {
		return fmt.Sprintf("Syntax(%v)", int(v))
	}
	return _table_Syntax_String[int(v)]
}

// GoString implements [fmt.GoStringer].
func (v Syntax) GoString() string {
	if int(v) < 0 || int(v) > len(_table_Syntax_GoString) {
		return fmt.Sprintf("syntaxSyntax(%v)", int(v))
	}
	return _table_Syntax_GoString[int(v)]
}

// Lookup looks up a syntax pragma by name.
//
// If name does not name a known pragma, returns [Unknown].
func Lookup(s string) Syntax {
	return _table_Syntax_Lookup[s]
}

// All returns an iterator over all known [Syntax] values.
func All() iter.Seq[Syntax] {
	return func(yield func(Syntax) bool) {
		for i := 1; i < 4; i++ {
			if !yield(Syntax(i)) {
				return
			}
		}
	}
}

var _table_Syntax_String = [...]string{
	Unknown:     "<unknown>",
	Proto2:      "proto2",
	Proto3:      "proto3",
	Edition2023: "2023",
}

var _table_Syntax_GoString = [...]string{
	Unknown:     "Unknown",
	Proto2:      "Proto2",
	Proto3:      "Proto3",
	Edition2023: "Edition2023",
}

var _table_Syntax_Lookup = map[string]Syntax{
	"proto2": Proto2,
	"proto3": Proto3,
	"2023":   Edition2023,
}
var _ iter.Seq[int] // Mark iter as used.
