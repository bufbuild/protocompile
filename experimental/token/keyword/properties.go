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

	Repeated: valid | word | protobuf | modField,
	Optional: valid | word | protobuf | modField,
	Required: valid | word | protobuf | modField,
	Stream:   valid | word | protobuf | modMethodType,
	Export:   valid | word | protobuf | modType,
	Local:    valid | word | protobuf | modType,

	Int32:    valid | word | protobuf,
	Int64:    valid | word | protobuf,
	Uint32:   valid | word | protobuf,
	Uint64:   valid | word | protobuf,
	Sint32:   valid | word | protobuf,
	Sint64:   valid | word | protobuf,
	Fixed32:  valid | word | protobuf,
	Fixed64:  valid | word | protobuf,
	Sfixed32: valid | word | protobuf,
	Sfixed64: valid | word | protobuf,
	Float:    valid | word | protobuf,
	Double:   valid | word | protobuf,
	Bool:     valid | word | protobuf,
	String:   valid | word | protobuf,
	Bytes:    valid | word | protobuf,

	Inf: valid | word | protobuf,
	NaN: valid | word | protobuf,

	True:  valid | word | protobuf | cel,
	False: valid | word | protobuf | cel,
	Null:  valid | word | protobuf | cel,

	Map: valid | word | protobuf,
	Max: valid | word | protobuf,

	Return:   valid | word,
	Break:    valid | word,
	Continue: valid | word,
	Yield:    valid | word,

	Defer: valid | word,
	Try:   valid | word,
	Catch: valid | word,

	If:     valid | word,
	Unless: valid | word,
	Else:   valid | word,

	Loop:  valid | word,
	While: valid | word,
	Do:    valid | word,
	For:   valid | word,
	In:    valid | word | cel,

	Switch: valid | word,
	Match:  valid | word,
	Case:   valid | word,

	As:     valid | word,
	Func:   valid | word,
	Const:  valid | word,
	Let:    valid | word,
	Var:    valid | word,
	Type:   valid | word,
	Extern: valid | word,

	And: valid | word,
	Or:  valid | word,
	Not: valid | word,

	Default:  valid | word | protobuf | pseudoOption,
	JsonName: valid | word | protobuf | pseudoOption,

	Semi:    valid | punct | protobuf,
	Comma:   valid | punct | protobuf | cel,
	Dot:     valid | punct | protobuf | cel,
	Colon:   valid | punct | protobuf | cel,
	Newline: valid | punct,
	At:      valid | punct | protobuf,
	Hash:    valid | punct | protobuf,
	Dollar:  valid | punct | protobuf,
	Twiddle: valid | punct | protobuf,

	Add:  valid | punct | cel,
	Sub:  valid | punct | cel,
	Mul:  valid | punct | cel,
	Div:  valid | punct | protobuf | cel,
	Rem:  valid | punct | cel,
	Amp:  valid | punct,
	Pipe: valid | punct,
	Xor:  valid | punct,
	Shl:  valid | punct,
	Shr:  valid | punct,

	Bang:  valid | punct | cel,
	Bangs: valid | punct,
	Ask:   valid | punct | cel,
	Asks:  valid | punct,

	Amps:  valid | punct | cel,
	Pipes: valid | punct | cel,

	Assign:     valid | punct | protobuf,
	AssignNew:  valid | punct,
	AssignAdd:  valid | punct,
	AssignSub:  valid | punct,
	AssignMul:  valid | punct,
	AssignDiv:  valid | punct,
	AssignRem:  valid | punct,
	AssignAmp:  valid | punct,
	AssignPipe: valid | punct,
	AssignXor:  valid | punct,
	AssignShl:  valid | punct,
	AssignShr:  valid | punct,

	Range:   valid | punct,
	RangeEq: valid | punct,

	LParen:   valid | punct | protobuf | cel,
	RParen:   valid | punct | protobuf | cel,
	LBracket: valid | punct | protobuf | cel,
	RBracket: valid | punct | protobuf | cel,
	LBrace:   valid | punct | protobuf | cel,
	RBrace:   valid | punct | protobuf | cel,

	Comment:  valid | punct | protobuf | cel,
	LComment: valid | punct | protobuf | cel,
	RComment: valid | punct | protobuf | cel,

	Lt: valid | punct | protobuf | cel,
	Gt: valid | punct | protobuf | cel,
	Le: valid | punct,
	Ge: valid | punct,
	Eq: valid | punct,
	Ne: valid | punct,

	Parens:       valid | punct | brackets | protobuf | cel,
	Brackets:     valid | punct | brackets | protobuf | cel,
	Braces:       valid | punct | brackets | protobuf | cel,
	Angles:       valid | punct | brackets | protobuf,
	BlockComment: valid | punct | brackets | protobuf | cel,
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

	Lt:     {Lt, Gt, Angles},
	Gt:     {Lt, Gt, Angles},
	Angles: {Lt, Gt, Angles},

	LComment:     {LComment, RComment, BlockComment},
	RComment:     {LComment, RComment, BlockComment},
	BlockComment: {LComment, RComment, BlockComment},
}
