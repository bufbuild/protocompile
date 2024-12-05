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

// package taxa (plural of taxon, an element of a taxonomy) provides support for
// classifying Protobuf syntax productions for use in the parser and in
// diagnostics.
//
// The Subject enum is also used in the parser stack as a simple way to inform
// recursive descent calls of what their caller is, since the What enum
// represents "everything" the parser stack pushes around.
package taxa

import (
	"fmt"
	"strconv"

	"github.com/bufbuild/protocompile/internal/iter"
)

// Subject is a syntactic or semantic element within the grammar that can be
// referred to within a diagnostic.
type Subject int

const (
	Unknown Subject = iota
	Unrecognized
	TopLevel
	EOF

	Decl
	Empty
	Syntax
	Edition
	Package
	Import
	WeakImport
	PublicImport
	Extensions
	Reserved
	Body

	Def
	Message
	Enum
	Service
	Extend
	Oneof

	Option
	CustomOption

	Field
	EnumValue
	Method

	FieldTag
	OptionValue

	CompactOptions
	MethodIns
	MethodOuts

	Path
	ExtensionInPath

	Expr
	Range
	Array
	Dict
	DictField

	Type
	TypeParams

	Whitespace
	Comment
	Ident
	String
	Float
	Int

	Semicolon
	Comma
	Slash
	Colon
	Equals
	Minus
	Period

	LParen
	LBracket
	LBrace
	LAngle

	RParen
	RBracket
	RBrace
	RAngle

	Parens
	Brackets
	Braces
	Angles

	KeywordSyntax
	KeywordEdition
	KeywordImport
	KeywordWeak
	KeywordPublic
	KeywordPackage

	KeywordOption
	KeywordMessage
	KeywordEnum
	KeywordService
	KeywordExtend
	KeywordOneof

	KeywordExtensions
	KeywordReserved
	KeywordTo
	KeywordRPC
	KeywordReturns

	KeywordOptional
	KeywordRepeated
	KeywordRequired
	KeywordGroup
	KeywordStream

	// Total is the total number of known [What] values.
	Total int = iota
)

// In is a shorthand for the "in" preposition.
func (s Subject) In() Place {
	return Place{s, "in"}
}

// After is a shorthand for the "after" preposition.
func (s Subject) After() Place {
	return Place{s, "after"}
}

// Without is a shorthand for the "without" preposition.
func (s Subject) Without() Place {
	return Place{s, "without"}
}

// AsSet returns a singleton set containing this What.
func (s Subject) AsSet() Set {
	return NewSet(s)
}

// String implements [fmt.Stringer].
func (s Subject) String() string {
	if int(s) >= len(names) {
		return names[0]
	}
	return names[s]
}

// All returns an iterator over all subjects.
func All() iter.Seq[Subject] {
	return func(yield func(Subject) bool) {
		for i := 0; i < Total; i++ {
			if !yield(Subject(i)) {
				break
			}
		}
	}
}

// GoString implements [fmt.GoStringer].
//
// This exists to get pretty output out of the assert package.
func (s Subject) GoString() string {
	if int(s) >= len(constNames) {
		return strconv.Itoa(int(s))
	}
	return "what." + constNames[s]
}

// Place is a location within the grammar that can be referred to within a
// diagnostic.
//
// It corresponds to a prepositional phrase in English, so it is actually
// somewhat more general than a place, and more accurately describes a general
// state of being.
type Place struct {
	Subject

	How string // The preposition, such as "in" or "after".
}

// String implements [fmt.Stringer].
func (p Place) String() string {
	return p.How + " " + p.Subject.String()
}

// GoString implements [fmt.GoStringer].
//
// This exists to get pretty output out of the assert package.
func (p Place) GoString() string {
	return fmt.Sprintf("{%#v, %#v}", p.Subject, p.How)
}
