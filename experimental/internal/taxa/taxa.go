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

// Noun is a syntactic or semantic element within the grammar that can be
// referred to within a diagnostic.
type Noun int

const (
	Unknown Noun = iota
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

	QualifiedName
	FullyQualifiedName
	ExtensionName

	Expr
	Range
	Array
	Dict
	DictField

	Type
	TypePath
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

	// total is the total number of known [What] values.
	total int = iota
)

// In is a shorthand for the "in" preposition.
func (s Noun) In() Place {
	return Place{s, "in"}
}

// After is a shorthand for the "after" preposition.
func (s Noun) After() Place {
	return Place{s, "after"}
}

// Without is a shorthand for the "without" preposition.
func (s Noun) Without() Place {
	return Place{s, "without"}
}

// AsSet returns a singleton set containing this What.
func (s Noun) AsSet() Set {
	return NewSet(s)
}

// String implements [fmt.Stringer].
func (s Noun) String() string {
	if int(s) >= len(names) {
		return names[0]
	}
	return names[s]
}

// All returns an iterator over all subjects.
func All() iter.Seq[Noun] {
	return func(yield func(Noun) bool) {
		for i := 0; i < total; i++ {
			if !yield(Noun(i)) {
				break
			}
		}
	}
}

// GoString implements [fmt.GoStringer].
//
// This exists to get pretty output out of the assert package.
func (s Noun) GoString() string {
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
