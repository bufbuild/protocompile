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
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum

// Noun is a syntactic or semantic element within the grammar that can be
// referred to within a diagnostic.
//
//enum:string
//enum:gostring
type Noun int

const (
	Unknown      Noun = iota //enum:string "<unknown>"
	Unrecognized             //enum:string "unrecognized token"
	TopLevel                 //enum:string "file scope"
	EOF                      //enum:string "end-of-file"

	Decl         //enum:string "declaration"
	Empty        //enum:string "empty declaration"
	Syntax       //enum:string "`syntax` declaration"
	Edition      //enum:string "`edition` declaration"
	Package      //enum:string "`package` declaration"
	Import       //enum:string "import"
	WeakImport   //enum:string "weak import"
	PublicImport //enum:string "public import"
	Extensions   //enum:string "extension range"
	Reserved     //enum:string "reserved range"
	Body         //enum:string "definition body"

	Def     //enum:string "definition"
	Message //enum:string "message definition"
	Enum    //enum:string "enum definition"
	Service //enum:string "service definition"
	Extend  //enum:string "message extension block"
	Oneof   //enum:string "oneof definition"

	Option       //enum:string "option setting"
	CustomOption //enum:string "custom option setting"

	Field     //enum:string "message field"
	EnumValue //enum:string "enum value"
	Method    //enum:string "service method"

	CompactOptions //enum:string "compact options"
	MethodIns      //enum:string "method parameter list"
	MethodOuts     //enum:string "method return type"

	FieldTag    //enum:string "message field tag"
	OptionValue //enum:string "option setting value"

	QualifiedName      //enum:string "qualified name"
	FullyQualifiedName //enum:string "fully qualified name"
	ExtensionName      //enum:string "extension name"

	Expr      //enum:string "expression"
	Range     //enum:string "range expression"
	Array     //enum:string "array expression"
	Dict      //enum:string "message expression"
	DictField //enum:string "message field value"

	Type       //enum:string "type"
	TypePath   //enum:string "type name"
	TypeParams //enum:string "type parameters"

	Whitespace //enum:string "whitespace"
	Comment    //enum:string "comment"
	Ident      //enum:string "identifier"
	String     //enum:string "string literal"
	Float      //enum:string "floating-point literal"
	Int        //enum:string "integer literal"

	Semicolon //enum:string "`;`"
	Comma     //enum:string "`,`"
	Slash     //enum:string "`/`"
	Colon     //enum:string "`:`"
	Equals    //enum:string "`=`"
	Minus     //enum:string "`-`"
	Period    //enum:string "`.`"

	LParen   //enum:string "`(`"
	LBracket //enum:string "`[`"
	LBrace   //enum:string "`{`"
	LAngle   //enum:string "`<`"

	RParen   //enum:string "`)`"
	RBracket //enum:string "`]`"
	RBrace   //enum:string "`}`"
	RAngle   //enum:string "`>`"

	Parens   //enum:string "`(...)`"
	Brackets //enum:string "`[...]`"
	Braces   //enum:string "`{...}`"
	Angles   //enum:string "`<...>`"

	KeywordSyntax  //enum:string "`syntax`"
	KeywordEdition //enum:string "`edition`"
	KeywordImport  //enum:string "`import`"
	KeywordWeak    //enum:string "`weak`"
	KeywordPublic  //enum:string "`public`"
	KeywordPackage //enum:string "`package`"

	KeywordOption  //enum:string "`option`"
	KeywordMessage //enum:string "`message`"
	KeywordEnum    //enum:string "`enum`"
	KeywordService //enum:string "`service`"
	KeywordExtend  //enum:string "`extend`"
	KeywordOneof   //enum:string "`oneof`"

	KeywordExtensions //enum:string "`extensions`"
	KeywordReserved   //enum:string "`reserved`"
	KeywordTo         //enum:string "`to`"
	KeywordRPC        //enum:string "`rpc`"
	KeywordReturns    //enum:string "`returns`"

	KeywordOptional //enum:string "`optional`"
	KeywordRepeated //enum:string "`repeated`"
	KeywordRequired //enum:string "`required`"
	KeywordGroup    //enum:string "`group`"
	KeywordStream   //enum:string "`stream`"

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
