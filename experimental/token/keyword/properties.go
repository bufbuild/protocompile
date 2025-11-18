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

package keyword

type property uint16

const (
	valid property = 1 << iota

	punct
	word
	brackets

	protobuf
	cel

	modField
	modType
	modImport
	modMethodType
	pseudoOption
)

func (k Keyword) properties() property {
	if int(k) < len(properties) {
		return properties[k]
	}
	return 0
}

// properties is a table of keyword properties, stored as bitsets.
var properties = [...]property{
	Syntax:     valid | word | protobuf,
	Edition:    valid | word | protobuf,
	Import:     valid | word | protobuf | cel,
	Weak:       valid | word | protobuf | modImport,
	Public:     valid | word | protobuf | modImport,
	Package:    valid | word | protobuf | cel,
	Message:    valid | word | protobuf,
	Enum:       valid | word | protobuf,
	Service:    valid | word | protobuf,
	Extend:     valid | word | protobuf,
	Option:     valid | word | protobuf | modImport,
	Group:      valid | word | protobuf,
	Oneof:      valid | word | protobuf,
	Extensions: valid | word | protobuf,
	Reserved:   valid | word | protobuf,
	RPC:        valid | word | protobuf,
	Returns:    valid | word | protobuf,
	To:         valid | word | protobuf,
	In:         valid | word | cel,

	Repeated: valid | word | protobuf | modField,
	Optional: valid | word | protobuf | modField,
	Required: valid | word | protobuf | modField,
	Stream:   valid | word | protobuf | modMethodType,
	Export:   valid | word | protobuf | modType,
	Local:    valid | word | protobuf | modType,

	Int32:    valid | word | protobuf,
	Int64:    valid | word | protobuf,
	UInt32:   valid | word | protobuf,
	UInt64:   valid | word | protobuf,
	SInt32:   valid | word | protobuf,
	SInt64:   valid | word | protobuf,
	Fixed32:  valid | word | protobuf,
	Fixed64:  valid | word | protobuf,
	SFixed32: valid | word | protobuf,
	SFixed64: valid | word | protobuf,
	Float:    valid | word | protobuf,
	Double:   valid | word | protobuf,
	Bool:     valid | word | protobuf,
	String:   valid | word | protobuf,
	Bytes:    valid | word | protobuf,

	Inf: valid | word | protobuf,
	NAN: valid | word | protobuf,

	True:  valid | word | protobuf | cel,
	False: valid | word | protobuf | cel,
	Null:  valid | word | protobuf | cel,

	Map:      valid | word | protobuf,
	Max:      valid | word | protobuf,
	Default:  valid | word | protobuf | pseudoOption,
	JsonName: valid | word | protobuf | pseudoOption,

	Semi:    valid | punct | protobuf,
	Comma:   valid | punct | protobuf | cel,
	Dot:     valid | punct | protobuf | cel,
	Colon:   valid | punct | protobuf | cel,
	Eq:      valid | punct | protobuf,
	Plus:    valid | punct | cel,
	Minus:   valid | punct | cel,
	Star:    valid | punct | cel,
	Slash:   valid | punct | protobuf | cel,
	Percent: valid | punct | cel,
	Bang:    valid | punct | cel,
	Ask:     valid | punct | cel,

	LParen:   valid | punct | protobuf | cel,
	RParen:   valid | punct | protobuf | cel,
	LBracket: valid | punct | protobuf | cel,
	RBracket: valid | punct | protobuf | cel,
	LBrace:   valid | punct | protobuf | cel,
	RBrace:   valid | punct | protobuf | cel,

	Comment:  valid | punct | protobuf | cel,
	LComment: valid | punct | protobuf | cel,
	RComment: valid | punct | protobuf | cel,

	Less:      valid | punct | protobuf | cel,
	Greater:   valid | punct | protobuf | cel,
	LessEq:    valid | punct,
	GreaterEq: valid | punct,
	EqEq:      valid | punct,
	BangEq:    valid | punct,

	OrOr:   valid | punct | protobuf | cel,
	AndAnd: valid | punct | protobuf | cel,

	Parens:       valid | punct | protobuf | cel,
	Brackets:     valid | punct | protobuf | cel,
	Braces:       valid | punct | protobuf | cel,
	Angles:       valid | punct | protobuf,
	BlockComment: valid | punct | protobuf | cel,
}

var braces = [...][3]Keyword{
	LParen: {LParen, RParen, Parens},
	RParen: {LParen, RParen, Parens},
	Parens: {LParen, RParen, Parens},

	LBracket: {LBracket, RBracket, Brackets},
	RBracket: {LBracket, RBracket, Brackets},
	Brackets: {LBracket, RBracket, Brackets},

	LBrace: {LBrace, RBrace, Braces},
	RBrace: {LBrace, RBrace, Braces},
	Braces: {LBrace, RBrace, Braces},

	Less:    {Less, Greater, Angles},
	Greater: {Less, Greater, Angles},
	Angles:  {Less, Greater, Angles},

	LComment:     {LComment, RComment, BlockComment},
	RComment:     {LComment, RComment, BlockComment},
	BlockComment: {LComment, RComment, BlockComment},
}
