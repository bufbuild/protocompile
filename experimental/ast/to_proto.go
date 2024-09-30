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

package ast

import (
	"fmt"
	"reflect"

	compilerv1 "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1"
	"google.golang.org/protobuf/proto"
)

// FileToProto converts file into a Protobuf representation, which may be serialized.
//
// Note that the ast package does not support deserialization from this proto; instead,
// you will need to reparse the text file included in the message. This is because the
// AST is much richer than what is stored in this message; the message only provides
// enough information for further semantic analysis and diagnostic generation, but not
// for pretty-printing.
func FileToProto(file File) proto.Message {
	return fileToProto(file)
}

func fileToProto(file File) *compilerv1.File {
	proto := &compilerv1.File{
		File: &compilerv1.Report_File{
			Path: file.Context().Path(),
			Text: []byte(file.Context().Text()),
		},
	}
	file.Iter(func(_ int, d Decl) bool {
		proto.Decls = append(proto.Decls, declToProto(d))
		return true
	})
	return proto
}

func spanToProto(s Spanner) *compilerv1.Span {
	if s == nil {
		return nil
	}

	span := s.Span()
	if span.Nil() {
		return nil
	}

	start, end := span.Offsets()
	return &compilerv1.Span{
		Start: uint32(start),
		End:   uint32(end),
	}
}

func pathToProto(path Path) *compilerv1.Path {
	if path.Nil() {
		return nil
	}

	proto := &compilerv1.Path{
		Span: spanToProto(path),
	}
	path.Components(func(c PathComponent) bool {
		component := new(compilerv1.Path_Component)
		switch c.Separator().Text() {
		case ".":
			component.Separator = compilerv1.Path_Component_SEPARATOR_DOT
		case "/":
			component.Separator = compilerv1.Path_Component_SEPARATOR_SLASH
		}
		component.SeparatorSpan = spanToProto(c.Separator())

		if c.IsExtension() {
			extn := c.AsExtension()
			component.Component = &compilerv1.Path_Component_Extension{Extension: pathToProto(extn)}
			component.ComponentSpan = spanToProto(extn)
		} else {
			ident := c.AsIdent()
			component.Component = &compilerv1.Path_Component_Ident{Ident: ident.Name()}
			component.ComponentSpan = spanToProto(ident)
		}

		proto.Components = append(proto.Components, component)
		return true
	})
	return proto
}

func declToProto(decl Decl) *compilerv1.Decl {
	if decl == nil {
		return nil
	}

	switch decl := decl.(type) {
	case DeclEmpty:
		return &compilerv1.Decl{Decl: &compilerv1.Decl_Empty_{Empty: &compilerv1.Decl_Empty{
			Span: spanToProto(decl),
		}}}

	case DeclSyntax:
		var kind compilerv1.Decl_Syntax_Kind
		if decl.IsSyntax() {
			kind = compilerv1.Decl_Syntax_KIND_SYNTAX
		} else if decl.IsEdition() {
			kind = compilerv1.Decl_Syntax_KIND_EDITION
		}

		return &compilerv1.Decl{Decl: &compilerv1.Decl_Syntax_{Syntax: &compilerv1.Decl_Syntax{
			Kind:          kind,
			Value:         exprToProto(decl.Value()),
			Span:          spanToProto(decl),
			KeywordSpan:   spanToProto(decl.Keyword()),
			EqualsSpan:    spanToProto(decl.Equals()),
			SemicolonSpan: spanToProto(decl.Semicolon()),
		}}}

	case DeclPackage:
		return &compilerv1.Decl{Decl: &compilerv1.Decl_Package_{Package: &compilerv1.Decl_Package{
			Path:          pathToProto(decl.Path()),
			Span:          spanToProto(decl),
			KeywordSpan:   spanToProto(decl.Keyword()),
			SemicolonSpan: spanToProto(decl.Semicolon()),
		}}}

	case DeclImport:
		var mod compilerv1.Decl_Import_Modifier
		if decl.IsWeak() {
			mod = compilerv1.Decl_Import_MODIFIER_WEAK
		} else if decl.IsPublic() {
			mod = compilerv1.Decl_Import_MODIFIER_PUBLIC
		}

		file, _ := decl.ImportPath().AsString()
		return &compilerv1.Decl{Decl: &compilerv1.Decl_Import_{Import: &compilerv1.Decl_Import{
			Modifier:       mod,
			ImportPath:     file,
			Span:           spanToProto(decl),
			KeywordSpan:    spanToProto(decl.Keyword()),
			ModifierSpan:   spanToProto(decl.Modifier()),
			ImportPathSpan: spanToProto(decl.ImportPath()),
			SemicolonSpan:  spanToProto(decl.Semicolon()),
		}}}

	case DeclBody:
		proto := &compilerv1.Decl_Body{
			Span: spanToProto(decl),
		}
		decl.Iter(func(_ int, d Decl) bool {
			proto.Decls = append(proto.Decls, declToProto(d))
			return true
		})
		return &compilerv1.Decl{Decl: &compilerv1.Decl_Body_{Body: proto}}

	case DeclRange:
		var kind compilerv1.Decl_Range_Kind
		if decl.IsExtensions() {
			kind = compilerv1.Decl_Range_KIND_EXTENSIONS
		} else if decl.IsReserved() {
			kind = compilerv1.Decl_Range_KIND_RESERVED

		}

		proto := &compilerv1.Decl_Range{
			Kind:          kind,
			Options:       optionsToProto(decl.Options()),
			Span:          spanToProto(decl),
			KeywordSpan:   spanToProto(decl.Keyword()),
			SemicolonSpan: spanToProto(decl.Semicolon()),
		}

		decl.Iter(func(_ int, e Expr) bool {
			proto.Ranges = append(proto.Ranges, exprToProto(e))
			return true
		})

		return &compilerv1.Decl{Decl: &compilerv1.Decl_Range_{Range: proto}}

	case DeclDef:
		var kind compilerv1.Def_Kind
		switch decl.Classify().(type) {
		case DefMessage:
			kind = compilerv1.Def_KIND_MESSAGE
		case DefEnum:
			kind = compilerv1.Def_KIND_ENUM
		case DefService:
			kind = compilerv1.Def_KIND_SERVICE
		case DefExtend:
			kind = compilerv1.Def_KIND_EXTEND
		case DefField:
			kind = compilerv1.Def_KIND_FIELD
		case DefEnumValue:
			kind = compilerv1.Def_KIND_ENUM_VALUE
		case DefOneof:
			kind = compilerv1.Def_KIND_ONEOF
		case DefGroup:
			kind = compilerv1.Def_KIND_GROUP
		case DefMethod:
			kind = compilerv1.Def_KIND_METHOD
		case DefOption:
			kind = compilerv1.Def_KIND_OPTION
		}

		proto := &compilerv1.Def{
			Kind:          kind,
			Name:          pathToProto(decl.Name()),
			Value:         exprToProto(decl.Value()),
			Options:       optionsToProto(decl.Options()),
			Span:          spanToProto(decl),
			KeywordSpan:   spanToProto(decl.Keyword()),
			EqualsSpan:    spanToProto(decl.Equals()),
			SemicolonSpan: spanToProto(decl.Semicolon()),
		}

		if kind == compilerv1.Def_KIND_FIELD || kind == compilerv1.Def_KIND_UNSPECIFIED {
			proto.Type = typeToProto(decl.Type())
		}

		if signature := decl.Signature(); !signature.Nil() {
			proto.Signature = &compilerv1.Def_Signature{
				Span:        spanToProto(signature),
				InputSpan:   spanToProto(signature.Inputs()),
				ReturnsSpan: spanToProto(signature.Returns()),
				OutputSpan:  spanToProto(signature.Outputs()),
			}

			signature.Inputs().Iter(func(_ int, t Type) bool {
				proto.Signature.Inputs = append(proto.Signature.Inputs, typeToProto(t))
				return true
			})
			signature.Outputs().Iter(func(_ int, t Type) bool {
				proto.Signature.Outputs = append(proto.Signature.Outputs, typeToProto(t))
				return true
			})
		}

		if body := decl.Body(); !body.Nil() {
			proto.Body = &compilerv1.Decl_Body{
				Span: spanToProto(decl.Body()),
			}
			body.Iter(func(_ int, d Decl) bool {
				proto.Body.Decls = append(proto.Body.Decls, declToProto(d))
				return true
			})
		}

		return &compilerv1.Decl{Decl: &compilerv1.Decl_Def{Def: proto}}
	}

	panic(fmt.Sprint("typeToProto: unknown Decl implementation:", reflect.TypeOf(expr)))
}

func optionsToProto(options Options) *compilerv1.Options {
	if options.Nil() {
		return nil
	}

	proto := &compilerv1.Options{
		Span: spanToProto(options),
	}

	options.Iter(func(_ int, o Option) bool {
		proto.Entries = append(proto.Entries, &compilerv1.Options_Entry{
			Path:       pathToProto(o.Path),
			Value:      exprToProto(o.Value),
			EqualsSpan: spanToProto(o.Equals),
		})
		return true
	})

	return proto
}

func exprToProto(expr Expr) *compilerv1.Expr {
	if expr == nil {
		return nil
	}

	switch expr := expr.(type) {
	case ExprLiteral:
		proto := &compilerv1.Expr_Literal{
			Span: spanToProto(expr),
		}
		if v, ok := expr.Token.AsInt(); ok {
			proto.Value = &compilerv1.Expr_Literal_IntValue{IntValue: v}
		} else if v, ok := expr.Token.AsFloat(); ok {
			proto.Value = &compilerv1.Expr_Literal_FloatValue{FloatValue: v}
		} else if v, ok := expr.Token.AsString(); ok {
			proto.Value = &compilerv1.Expr_Literal_StringValue{StringValue: v}
		}
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Literal_{Literal: proto}}

	case ExprPath:
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Path{Path: pathToProto(expr.Path)}}

	case ExprPrefixed:
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Prefixed_{Prefixed: &compilerv1.Expr_Prefixed{
			Prefix:     compilerv1.Expr_Prefixed_Prefix(expr.Prefix()),
			Expr:       exprToProto(expr.Expr()),
			Span:       spanToProto(expr),
			PrefixSpan: spanToProto(expr.PrefixToken()),
		}}}

	case ExprRange:
		start, end := expr.Bounds()
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Range_{Range: &compilerv1.Expr_Range{
			Start:  exprToProto(start),
			End:    exprToProto(end),
			Span:   spanToProto(expr),
			ToSpan: spanToProto(expr.Keyword()),
		}}}

	case ExprArray:
		proto := &compilerv1.Expr_Array{
			Span: spanToProto(expr),
		}
		expr.Iter(func(_ int, e Expr) bool {
			proto.Elements = append(proto.Elements, exprToProto(e))
			return true
		})
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Array_{Array: proto}}

	case ExprDict:
		proto := &compilerv1.Expr_Dict{
			Span: spanToProto(expr),
		}
		expr.Iter(func(_ int, e ExprKV) bool {
			proto.Entries = append(proto.Entries, exprKVToProto(e))
			return true
		})
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Dict_{Dict: proto}}

	case ExprKV:
		return &compilerv1.Expr{Expr: &compilerv1.Expr_Kv_{Kv: exprKVToProto(expr)}}
	}

	panic(fmt.Sprint("typeToProto: unknown Expr implementation:", reflect.TypeOf(expr)))
}

func exprKVToProto(expr ExprKV) *compilerv1.Expr_Kv {
	if expr.Nil() {
		return nil
	}

	return &compilerv1.Expr_Kv{
		Key:       exprToProto(expr.Key()),
		Value:     exprToProto(expr.Value()),
		Span:      spanToProto(expr),
		ColonSpan: spanToProto(expr.Colon()),
	}
}

func typeToProto(ty Type) *compilerv1.Type {
	if ty == nil {
		return nil
	}

	switch ty := ty.(type) {
	case TypePath:
		return &compilerv1.Type{Type: &compilerv1.Type_Path{Path: pathToProto(ty.Path)}}

	case TypePrefixed:
		return &compilerv1.Type{Type: &compilerv1.Type_Prefixed_{Prefixed: &compilerv1.Type_Prefixed{
			Prefix:     compilerv1.Type_Prefixed_Prefix(ty.Prefix()),
			Type:       typeToProto(ty.Type()),
			Span:       spanToProto(ty),
			PrefixSpan: spanToProto(ty.PrefixToken()),
		}}}

	case TypeGeneric:
		generic := &compilerv1.Type_Generic{
			Path:        pathToProto(ty.Path()),
			Span:        spanToProto(ty),
			BracketSpan: spanToProto(ty.Args()),
		}
		ty.Args().Iter(func(_ int, t Type) bool {
			generic.Args = append(generic.Args, typeToProto(t))
			return true
		})
		return &compilerv1.Type{Type: &compilerv1.Type_Generic_{Generic: generic}}
	}

	panic(fmt.Sprint("typeToProto: unknown Type implementation:", reflect.TypeOf(ty)))
}
