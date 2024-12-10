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

package taxa

var (
	// names is an array of user-visible names of all of the productions in this
	// package.
	names = [...]string{
		Unknown:      "<unknown>",
		Unrecognized: "unrecognized token",
		TopLevel:     "file scope",
		EOF:          "end-of-file",

		Decl:         "declaration",
		Empty:        "empty declaration",
		Syntax:       "`syntax` declaration",
		Edition:      "`edition` declaration",
		Package:      "`package` declaration",
		Import:       "import",
		WeakImport:   "weak import",
		PublicImport: "public import",
		Extensions:   "extension range",
		Reserved:     "reserved range",
		Body:         "definition body",

		Def:     "definition",
		Message: "message definition",
		Enum:    "enum definition",
		Service: "service definition",
		Extend:  "message extension block",
		Oneof:   "oneof definition",

		Option:       "option setting",
		CustomOption: "custom option setting",

		Field:     "message field",
		EnumValue: "enum value",
		Method:    "service method",

		CompactOptions: "compact options",
		MethodIns:      "method parameter list",
		MethodOuts:     "method return type",

		FieldTag:    "message field tag",
		OptionValue: "option setting value",

		QualifiedName:      "qualified name",
		FullyQualifiedName: "fully qualified name",
		ExtensionName:      "extension name",

		Expr:      "expression",
		Range:     "range expression",
		Array:     "array expression",
		Dict:      "message expression",
		DictField: "message field value",

		Type:       "type",
		TypePath:   "type name",
		TypeParams: "type parameters",

		Whitespace: "whitespace",
		Comment:    "comment",
		Ident:      "identifier",
		String:     "string literal",
		Float:      "floating-point literal",
		Int:        "integer literal",

		Semicolon: "`;`",
		Comma:     "`,`",
		Slash:     "`/`",
		Colon:     "`:`",
		Equals:    "`=`",
		Minus:     "`-`",
		Period:    "`.`",

		LParen:   "`(`",
		LBracket: "`[`",
		LBrace:   "`{`",
		LAngle:   "`<`",

		RParen:   "`)`",
		RBracket: "`]`",
		RBrace:   "`}`",
		RAngle:   "`>`",

		Parens:   "`(...)`",
		Brackets: "`[...]`",
		Braces:   "`{...}`",
		Angles:   "`<...>`",

		KeywordSyntax:  "`syntax`",
		KeywordEdition: "`edition`",
		KeywordImport:  "`import`",
		KeywordWeak:    "`weak`",
		KeywordPublic:  "`public`",
		KeywordPackage: "`package`",

		KeywordOption:  "`option`",
		KeywordMessage: "`message`",
		KeywordEnum:    "`enum`",
		KeywordService: "`service`",
		KeywordExtend:  "`extend`",
		KeywordOneof:   "`oneof`",

		KeywordExtensions: "`extensions`",
		KeywordReserved:   "`reserved`",
		KeywordTo:         "`to`",
		KeywordRPC:        "`rpc`",
		KeywordReturns:    "`returns`",

		KeywordOptional: "`optional`",
		KeywordRepeated: "`repeated`",
		KeywordRequired: "`required`",
		KeywordGroup:    "`group`",
		KeywordStream:   "`stream`",
	}

	// constNames is an array of the names of the constants, for use in GoString().
	constNames = [...]string{
		Unknown:      "Unknown",
		Unrecognized: "Unrecognized",
		TopLevel:     "TopLevel",
		EOF:          "EOF",

		Decl:         "Decl",
		Empty:        "Empty",
		Syntax:       "Syntax",
		Edition:      "Edition",
		Package:      "Package",
		Import:       "Import",
		WeakImport:   "WeakImport",
		PublicImport: "PublicImport",
		Extensions:   "Extensions",
		Reserved:     "Reserved",
		Body:         "Body",

		Def:     "Def",
		Message: "Message",
		Enum:    "Enum",
		Service: "Service",
		Extend:  "Extend",
		Oneof:   "Oneof",

		Option:       "Option",
		CustomOption: "CustomOption",

		Field:     "Field",
		EnumValue: "EnumValue",
		Method:    "Method",

		FieldTag:    "FieldTag",
		OptionValue: "OptionValue",

		CompactOptions: "CompactOptions",
		MethodIns:      "MethodIns",
		MethodOuts:     "MethodOuts",

		QualifiedName:      "QualifiedName",
		FullyQualifiedName: "FullyQualifiedName",
		ExtensionName:      "ExtensionName",

		Expr:      "Expr",
		Range:     "Range",
		Array:     "Array",
		Dict:      "Dict",
		DictField: "DictField",

		Type:       "Type",
		TypePath:   "TypePath",
		TypeParams: "TypeParams",

		Whitespace: "Whitespace",
		Comment:    "Comment",
		Ident:      "Ident",
		String:     "String",
		Float:      "Float",
		Int:        "Int",

		Semicolon: "Semicolon",
		Comma:     "Comma",
		Slash:     "Slash",
		Colon:     "Colon",
		Equals:    "Equals",
		Minus:     "Minus",
		Period:    "Period",

		LParen:   "LParen",
		LBracket: "LBracket",
		LBrace:   "LBrace",
		LAngle:   "LAngle",

		RParen:   "RParen",
		RBracket: "RBracket",
		RBrace:   "RBrace",
		RAngle:   "RAngle",

		Parens:   "Parens",
		Brackets: "Brackets",
		Braces:   "Braces",
		Angles:   "Angles",

		KeywordSyntax:  "KeywordSyntax",
		KeywordEdition: "KeywordEdition",
		KeywordImport:  "KeywordImport",
		KeywordWeak:    "KeywordWeak",
		KeywordPublic:  "KeywordPublic",
		KeywordPackage: "KeywordPackage",

		KeywordOption:  "KeywordOption",
		KeywordMessage: "KeywordMessage",
		KeywordEnum:    "KeywordEnum",
		KeywordService: "KeywordService",
		KeywordExtend:  "KeywordExtend",
		KeywordOneof:   "KeywordOneof",

		KeywordExtensions: "KeywordExtensions",
		KeywordReserved:   "KeywordReserved",
		KeywordTo:         "KeywordTo",
		KeywordRPC:        "KeywordRPC",
		KeywordReturns:    "KeywordReturns",

		KeywordOptional: "KeywordOptional",
		KeywordRepeated: "KeywordRepeated",
		KeywordRequired: "KeywordRequired",
		KeywordGroup:    "KeywordGroup",
		KeywordStream:   "KeywordStream",
	}
)
