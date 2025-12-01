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

// Package taxa (plural of taxon, an element of a taxonomy) provides support for
// classifying Protobuf syntax productions for use in the parser and in
// diagnostics.
//
// The Subject enum is also used in the parser stack as a simple way to inform
// recursive descent calls of what their caller is, since the What enum
// represents "everything" the parser stack pushes around.
package taxa

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum noun.yaml

const (
	keywordCount = 135 // Verified by a unit test.
	taxaCount    = nounCount + keywordCount
)

const Unknown Noun = 0

// In is a shorthand for the "in" preposition.
func (n Noun) In() Place {
	return Place{n, "in"}
}

// After is a shorthand for the "after" preposition.
func (n Noun) After() Place {
	return Place{n, "after"}
}

// Without is a shorthand for the "without" preposition.
func (n Noun) Without() Place {
	return Place{n, "without"}
}

// On is a shorthand for the "on" preposition.
func (n Noun) On() Place {
	return Place{n, "on"}
}

// AsSet returns a singleton set containing this What.
func (n Noun) AsSet() Set {
	return NewSet(n)
}

// IsKeyword returns whether this is a wrapped [keyword.Keyword] value.
func (n Noun) IsKeyword() bool {
	return n > 0 && n < keywordCount
}

// Place is a location within the grammar that can be referred to within a
// diagnostic.
//
// It corresponds to a prepositional phrase in English, so it is actually
// somewhat more general than a place, and more accurately describes a general
// state of being.
type Place struct {
	subject     Noun
	preposition string
}

// Subject returns this place's subject.
func (p Place) Subject() Noun {
	return p.subject
}

// String implements [fmt.Stringer].
func (p Place) String() string {
	return p.preposition + " " + p.subject.String()
}

// GoString implements [fmt.GoStringer].
//
// This exists to get pretty output out of the assert package.
func (p Place) GoString() string {
	return fmt.Sprintf("{%#v, %#v}", p.subject, p.preposition)
}

func init() {
	// Fill out the string tables for Noun with their keyword values.
	_table_Noun_String[Unknown] = "<unknown>"
	_table_Noun_GoString[Unknown] = "taxa.Unknown"

	for kw := range keyword.All() {
		if kw == keyword.Unknown {
			continue
		}

		name := kw.String()
		if kw == keyword.Newline {
			name = "\\n" // Make sure the newline token is escaped.
		}

		_table_Noun_String[kw] = "`" + name + "`"
		_table_Noun_GoString[kw] = kw.GoString()
	}
}
