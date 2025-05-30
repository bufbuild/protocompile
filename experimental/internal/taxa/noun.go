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

// Code generated by github.com/bufbuild/protocompile/internal/enum noun.yaml. DO NOT EDIT.

package taxa

import (
	"fmt"
	"iter"
)

// Noun is a syntactic or semantic element within the grammar that can be
// referred to within a diagnostic.
type Noun int

const (
	Unknown Noun = iota
	Unrecognized
	TopLevel
	EOF
	SyntaxMode
	EditionMode
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
	Group
	Option
	CustomOption
	FieldSelector
	PseudoOption
	Field
	Extension
	EnumValue
	Method
	CompactOptions
	MethodIns
	MethodOuts
	Signature
	FieldTag
	FieldNumber
	MessageSetNumber
	FieldName
	OptionValue
	QualifiedName
	FullyQualifiedName
	ExtensionName
	TypeURL
	Expr
	Range
	Array
	Dict
	DictField
	Type
	TypePath
	TypeParams
	TypePrefix
	MessageType
	EnumType
	ScalarType
	MapKey
	MapValue
	Whitespace
	Comment
	Ident
	String
	Float
	Int
	Number
	Semi
	Comma
	Slash
	Colon
	Equals
	Minus
	Dot
	Parens
	Brackets
	Braces
	Angles
	ReturnsParens
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
	PredeclaredMap
	PredeclaredMax
	total int = iota
)

// String implements [fmt.Stringer].
func (v Noun) String() string {
	if int(v) < 0 || int(v) > len(_table_Noun_String) {
		return fmt.Sprintf("Noun(%v)", int(v))
	}
	return _table_Noun_String[int(v)]
}

// GoString implements [fmt.GoStringer].
func (v Noun) GoString() string {
	if int(v) < 0 || int(v) > len(_table_Noun_GoString) {
		return fmt.Sprintf("taxaNoun(%v)", int(v))
	}
	return _table_Noun_GoString[int(v)]
}

var _table_Noun_String = [...]string{
	Unknown:            "<unknown>",
	Unrecognized:       "unrecognized token",
	TopLevel:           "file scope",
	EOF:                "end-of-file",
	SyntaxMode:         "syntax mode",
	EditionMode:        "editions mode",
	Decl:               "declaration",
	Empty:              "empty declaration",
	Syntax:             "`syntax` declaration",
	Edition:            "`edition` declaration",
	Package:            "`package` declaration",
	Import:             "import",
	WeakImport:         "weak import",
	PublicImport:       "public import",
	Extensions:         "extension range",
	Reserved:           "reserved range",
	Body:               "definition body",
	Def:                "definition",
	Message:            "message definition",
	Enum:               "enum definition",
	Service:            "service definition",
	Extend:             "message extension block",
	Oneof:              "oneof definition",
	Group:              "group definition",
	Option:             "option setting",
	CustomOption:       "custom option setting",
	FieldSelector:      "field selector",
	PseudoOption:       "pseudo-option",
	Field:              "message field",
	Extension:          "message extension",
	EnumValue:          "enum value",
	Method:             "service method",
	CompactOptions:     "compact options",
	MethodIns:          "method parameter list",
	MethodOuts:         "method return type",
	Signature:          "method signature",
	FieldTag:           "message field tag",
	FieldNumber:        "field number",
	MessageSetNumber:   "`MessageSet` extension number",
	FieldName:          "message field name",
	OptionValue:        "option setting value",
	QualifiedName:      "qualified name",
	FullyQualifiedName: "fully qualified name",
	ExtensionName:      "extension name",
	TypeURL:            "`Any` type URL",
	Expr:               "expression",
	Range:              "range expression",
	Array:              "array expression",
	Dict:               "message expression",
	DictField:          "message field value",
	Type:               "type",
	TypePath:           "type name",
	TypeParams:         "type parameters",
	TypePrefix:         "type modifier",
	MessageType:        "message type",
	EnumType:           "enum type",
	ScalarType:         "scalar type",
	MapKey:             "map key type",
	MapValue:           "map value type",
	Whitespace:         "whitespace",
	Comment:            "comment",
	Ident:              "identifier",
	String:             "string literal",
	Float:              "floating-point literal",
	Int:                "integer literal",
	Number:             "number literal",
	Semi:               "`;`",
	Comma:              "`,`",
	Slash:              "`/`",
	Colon:              "`:`",
	Equals:             "`=`",
	Minus:              "`-`",
	Dot:                "`.`",
	Parens:             "`(...)`",
	Brackets:           "`[...]`",
	Braces:             "`{...}`",
	Angles:             "`<...>`",
	ReturnsParens:      "`returns (...)`",
	KeywordSyntax:      "`syntax`",
	KeywordEdition:     "`edition`",
	KeywordImport:      "`import`",
	KeywordWeak:        "`weak`",
	KeywordPublic:      "`public`",
	KeywordPackage:     "`package`",
	KeywordOption:      "`option`",
	KeywordMessage:     "`message`",
	KeywordEnum:        "`enum`",
	KeywordService:     "`service`",
	KeywordExtend:      "`extend`",
	KeywordOneof:       "`oneof`",
	KeywordExtensions:  "`extensions`",
	KeywordReserved:    "`reserved`",
	KeywordTo:          "`to`",
	KeywordRPC:         "`rpc`",
	KeywordReturns:     "`returns`",
	KeywordOptional:    "`optional`",
	KeywordRepeated:    "`repeated`",
	KeywordRequired:    "`required`",
	KeywordGroup:       "`group`",
	KeywordStream:      "`stream`",
	PredeclaredMap:     "`map`",
	PredeclaredMax:     "`max`",
}

var _table_Noun_GoString = [...]string{
	Unknown:            "Unknown",
	Unrecognized:       "Unrecognized",
	TopLevel:           "TopLevel",
	EOF:                "EOF",
	SyntaxMode:         "SyntaxMode",
	EditionMode:        "EditionMode",
	Decl:               "Decl",
	Empty:              "Empty",
	Syntax:             "Syntax",
	Edition:            "Edition",
	Package:            "Package",
	Import:             "Import",
	WeakImport:         "WeakImport",
	PublicImport:       "PublicImport",
	Extensions:         "Extensions",
	Reserved:           "Reserved",
	Body:               "Body",
	Def:                "Def",
	Message:            "Message",
	Enum:               "Enum",
	Service:            "Service",
	Extend:             "Extend",
	Oneof:              "Oneof",
	Group:              "Group",
	Option:             "Option",
	CustomOption:       "CustomOption",
	FieldSelector:      "FieldSelector",
	PseudoOption:       "PseudoOption",
	Field:              "Field",
	Extension:          "Extension",
	EnumValue:          "EnumValue",
	Method:             "Method",
	CompactOptions:     "CompactOptions",
	MethodIns:          "MethodIns",
	MethodOuts:         "MethodOuts",
	Signature:          "Signature",
	FieldTag:           "FieldTag",
	FieldNumber:        "FieldNumber",
	MessageSetNumber:   "MessageSetNumber",
	FieldName:          "FieldName",
	OptionValue:        "OptionValue",
	QualifiedName:      "QualifiedName",
	FullyQualifiedName: "FullyQualifiedName",
	ExtensionName:      "ExtensionName",
	TypeURL:            "TypeURL",
	Expr:               "Expr",
	Range:              "Range",
	Array:              "Array",
	Dict:               "Dict",
	DictField:          "DictField",
	Type:               "Type",
	TypePath:           "TypePath",
	TypeParams:         "TypeParams",
	TypePrefix:         "TypePrefix",
	MessageType:        "MessageType",
	EnumType:           "EnumType",
	ScalarType:         "ScalarType",
	MapKey:             "MapKey",
	MapValue:           "MapValue",
	Whitespace:         "Whitespace",
	Comment:            "Comment",
	Ident:              "Ident",
	String:             "String",
	Float:              "Float",
	Int:                "Int",
	Number:             "Number",
	Semi:               "Semi",
	Comma:              "Comma",
	Slash:              "Slash",
	Colon:              "Colon",
	Equals:             "Equals",
	Minus:              "Minus",
	Dot:                "Dot",
	Parens:             "Parens",
	Brackets:           "Brackets",
	Braces:             "Braces",
	Angles:             "Angles",
	ReturnsParens:      "ReturnsParens",
	KeywordSyntax:      "KeywordSyntax",
	KeywordEdition:     "KeywordEdition",
	KeywordImport:      "KeywordImport",
	KeywordWeak:        "KeywordWeak",
	KeywordPublic:      "KeywordPublic",
	KeywordPackage:     "KeywordPackage",
	KeywordOption:      "KeywordOption",
	KeywordMessage:     "KeywordMessage",
	KeywordEnum:        "KeywordEnum",
	KeywordService:     "KeywordService",
	KeywordExtend:      "KeywordExtend",
	KeywordOneof:       "KeywordOneof",
	KeywordExtensions:  "KeywordExtensions",
	KeywordReserved:    "KeywordReserved",
	KeywordTo:          "KeywordTo",
	KeywordRPC:         "KeywordRPC",
	KeywordReturns:     "KeywordReturns",
	KeywordOptional:    "KeywordOptional",
	KeywordRepeated:    "KeywordRepeated",
	KeywordRequired:    "KeywordRequired",
	KeywordGroup:       "KeywordGroup",
	KeywordStream:      "KeywordStream",
	PredeclaredMap:     "PredeclaredMap",
	PredeclaredMax:     "PredeclaredMax",
}
var _ iter.Seq[int] // Mark iter as used.
