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

package taxa

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// IsFloat checks whether or not tok is intended to be a floating-point literal.
func IsFloat(tok token.Token) bool {
	return tok.AsNumber().IsFloat()
}

// IsFloatText checks whether or not the given number text is intended to be
// a floating-point literal.
func IsFloatText(digits string) bool {
	needle := ".EePp"
	if strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X") {
		needle = ".Pp"
	}
	return strings.ContainsAny(digits, needle)
}

// Classify attempts to classify node for use in a diagnostic.
func Classify(node source.Spanner) Noun {
	// This is a giant type switch on every AST and token type in the compiler.
	switch node := node.(type) {
	case token.Token:
		return classifyToken(node)

	case *ast.File:
		return TopLevel
	case ast.Path:
		if first, ok := iterx.OnlyOne(node.Components); ok && first.Separator().IsZero() {
			if id := first.AsIdent(); !id.IsZero() {
				return classifyToken(id)
			}
			if !first.AsExtension().IsZero() {
				return ExtensionName
			}
		}

		if node.Absolute() {
			return FullyQualifiedName
		}

		return QualifiedName

	case ast.DeclAny:
		switch node.Kind() {
		case ast.DeclKindEmpty:
			return Classify(node.AsEmpty())
		case ast.DeclKindSyntax:
			return Classify(node.AsSyntax())
		case ast.DeclKindPackage:
			return Classify(node.AsPackage())
		case ast.DeclKindImport:
			return Classify(node.AsImport())
		case ast.DeclKindRange:
			return Classify(node.AsRange())
		case ast.DeclKindBody:
			return Classify(node.AsBody())
		case ast.DeclKindDef:
			return Classify(node.AsDef())
		default:
			return Decl
		}
	case ast.DeclEmpty:
		return Empty
	case ast.DeclSyntax:
		if node.IsEdition() {
			return Edition
		}
		return Syntax
	case ast.DeclPackage:
		return Package
	case ast.DeclImport:
		return Import
	case ast.DeclRange:
		if node.IsExtensions() {
			return Extensions
		}
		return Reserved
	case ast.DeclBody:
		return Body

	case ast.DeclDef:
		switch node.Classify() {
		case ast.DefKindMessage:
			return Classify(node.AsMessage())
		case ast.DefKindEnum:
			return Classify(node.AsEnum())
		case ast.DefKindService:
			return Classify(node.AsService())
		case ast.DefKindExtend:
			return Classify(node.AsExtend())
		case ast.DefKindOption:
			return Classify(node.AsOption())
		case ast.DefKindField:
			return Classify(node.AsField())
		case ast.DefKindEnumValue:
			return Classify(node.AsEnumValue())
		case ast.DefKindMethod:
			return Classify(node.AsMethod())
		case ast.DefKindOneof:
			return Classify(node.AsOneof())
		case ast.DefKindGroup:
			return Classify(node.AsGroup())
		default:
			return Def
		}
	case ast.DefMessage:
		return Message
	case ast.DefEnum:
		return Enum
	case ast.DefService:
		return Service
	case ast.DefExtend:
		return Extend
	case ast.DefOption:
		var first ast.PathComponent
		node.Path.Components(func(pc ast.PathComponent) bool {
			first = pc
			return false
		})
		if !first.AsExtension().IsZero() {
			return CustomOption
		}
		return Option
	case ast.DefField:
		return Field
	case ast.DefGroup:
		return Group
	case ast.DefEnumValue:
		return EnumValue
	case ast.DefMethod:
		return Method
	case ast.DefOneof:
		return Oneof

	case ast.ExprAny:
		switch node.Kind() {
		case ast.ExprKindLiteral:
			return Classify(node.AsLiteral())
		case ast.ExprKindPrefixed:
			return Classify(node.AsPrefixed())
		case ast.ExprKindPath:
			return Classify(node.AsPath())
		case ast.ExprKindRange:
			return Classify(node.AsRange())
		case ast.ExprKindArray:
			return Classify(node.AsArray())
		case ast.ExprKindDict:
			return Classify(node.AsDict())
		case ast.ExprKindField:
			return Classify(node.AsField())
		default:
			return Expr
		}
	case ast.ExprLiteral:
		return Classify(node.Token)
	case ast.ExprPrefixed:
		// This ensures that e.g. -1 is described as a number rather than as
		// an "expression".
		return Classify(node.Expr())
	case ast.ExprPath:
		return Classify(node.Path)
	case ast.ExprRange:
		return Range
	case ast.ExprArray:
		return Array
	case ast.ExprDict:
		return Dict
	case ast.ExprField:
		return DictField

	case ast.TypeAny:
		switch node.Kind() {
		case ast.TypeKindPath:
			return Classify(node.AsPath())
		default:
			return Type
		}

	case ast.TypePath:
		return TypePath
	case ast.TypePrefixed, ast.TypeGeneric:
		return Type

	case ast.CompactOptions:
		return CompactOptions

	case ast.Signature:
		switch {
		case node.Inputs().IsZero() == node.Outputs().IsZero():
			return Signature
		case !node.Inputs().IsZero():
			return MethodIns
		default:
			return MethodOuts
		}
	}

	return Unknown
}

func classifyToken(tok token.Token) Noun {
	switch tok.Kind() {
	case token.Space:
		return Whitespace
	case token.Comment:
		return Comment
	case token.Ident:
		if kw := Keyword(tok.Keyword()); kw != Unknown {
			return kw
		}
		return Ident
	case token.String:
		return String
	case token.Number:
		if IsFloat(tok) {
			return Float
		}
		return Int
	case token.Punct:
		return Keyword(tok.Keyword())
	default:
		return Unrecognized
	}
}

// Keyword maps a keyword to its [Noun], if it has one.
func Keyword(kw keyword.Keyword) Noun {
	n, _ := slicesx.Get(kwToNoun, kw)
	return n
}

var kwToNoun = []Noun{
	keyword.Syntax:     KeywordSyntax,
	keyword.Edition:    KeywordEdition,
	keyword.Import:     KeywordImport,
	keyword.Weak:       KeywordWeak,
	keyword.Public:     KeywordPublic,
	keyword.Package:    KeywordPackage,
	keyword.Message:    KeywordMessage,
	keyword.Enum:       KeywordEnum,
	keyword.Service:    KeywordService,
	keyword.Extend:     KeywordExtend,
	keyword.Option:     KeywordOption,
	keyword.Group:      KeywordGroup,
	keyword.Oneof:      KeywordOneof,
	keyword.Extensions: KeywordExtensions,
	keyword.Reserved:   KeywordReserved,
	keyword.RPC:        KeywordRPC,
	keyword.Returns:    KeywordReturns,
	keyword.To:         KeywordTo,
	keyword.Repeated:   KeywordRepeated,
	keyword.Optional:   KeywordOptional,
	keyword.Required:   KeywordRequired,
	keyword.Stream:     KeywordStream,
	keyword.Export:     KeywordExport,
	keyword.Local:      KeywordLocal,

	// TODO: Give these taxa if it turns out they wind up in diagnostics.
	keyword.Int32:    Unknown,
	keyword.Int64:    Unknown,
	keyword.UInt32:   Unknown,
	keyword.UInt64:   Unknown,
	keyword.SInt32:   Unknown,
	keyword.SInt64:   Unknown,
	keyword.Fixed32:  Unknown,
	keyword.Fixed64:  Unknown,
	keyword.SFixed32: Unknown,
	keyword.SFixed64: Unknown,
	keyword.Float:    Unknown,
	keyword.Double:   Unknown,
	keyword.Bool:     Unknown,
	keyword.String:   Unknown,
	keyword.Bytes:    Unknown,
	keyword.Inf:      Unknown,
	keyword.NAN:      Unknown,
	keyword.True:     Unknown,
	keyword.False:    Unknown,

	keyword.Map:      Unknown,
	keyword.Max:      Unknown,
	keyword.Default:  Unknown,
	keyword.JsonName: Unknown,

	keyword.Semi:     Semi,
	keyword.Comma:    Comma,
	keyword.Dot:      Dot,
	keyword.Slash:    Slash,
	keyword.Colon:    Colon,
	keyword.Eq:       Equals,
	keyword.Minus:    Minus,
	keyword.Parens:   Parens,
	keyword.Brackets: Brackets,
	keyword.Braces:   Braces,
	keyword.Angles:   Angles,
}
