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

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
)

// codec is the state needed for converting an AST node into a Protobuf message.
type codec struct {
	*ToProtoOptions
	values map[report.Spanner]bool
}

// check panics if v is visited cyclically.
//
// Should be called like this:
//
//	defer c.check(v)()
func (c *codec) check(v report.Spanner) func() {
	if c.values == nil {
		c.values = map[report.Spanner]bool{v: true}
	} else {
		if c.values[v] {
			panic(fmt.Sprintf("protocompile/ast: called File.ToProto on a cyclic AST %v", v.Span()))
		}
		c.values[v] = true
	}
	return func() { delete(c.values, v) }
}

func (c *codec) file(file File) *compilerpb.File {
	proto := new(compilerpb.File)
	if !c.ElideFile {
		proto.File = &compilerpb.Report_File{
			Path: file.Context().Stream().Path(),
			Text: []byte(file.Context().Stream().Text()),
		}
	}

	file.Iter(func(_ int, d DeclAny) bool {
		proto.Decls = append(proto.Decls, c.decl(d))
		return true
	})
	return proto
}

func (c *codec) span(s report.Spanner) *compilerpb.Span {
	if c.ElideSpans || internal.Nil(s) {
		return nil
	}

	span := s.Span()
	if span.IndexedFile == nil {
		return nil
	}

	return &compilerpb.Span{
		Start: uint32(span.Start),
		End:   uint32(span.End),
	}
}

func (c *codec) path(path Path) *compilerpb.Path {
	if path.Nil() {
		return nil
	}
	defer c.check(path)()

	proto := &compilerpb.Path{
		Span: c.span(path),
	}
	path.Components(func(pc PathComponent) bool {
		component := new(compilerpb.Path_Component)
		switch pc.Separator().Text() {
		case ".":
			component.Separator = compilerpb.Path_Component_SEPARATOR_DOT
		case "/":
			component.Separator = compilerpb.Path_Component_SEPARATOR_SLASH
		}
		component.SeparatorSpan = c.span(pc.Separator())

		if extn := pc.AsExtension(); !extn.Nil() {
			extn := pc.AsExtension()
			component.Component = &compilerpb.Path_Component_Extension{Extension: c.path(extn)}
			component.ComponentSpan = c.span(extn)
		} else if ident := pc.AsIdent(); !ident.Nil() {
			component.Component = &compilerpb.Path_Component_Ident{Ident: ident.Name()}
			component.ComponentSpan = c.span(ident)
		}

		proto.Components = append(proto.Components, component)
		return true
	})
	return proto
}

func (c *codec) decl(decl DeclAny) *compilerpb.Decl {
	if decl.Nil() {
		return nil
	}
	defer c.check(decl)()

	switch k := decl.Kind(); k {
	case DeclKindEmpty:
		decl := decl.AsEmpty()
		return &compilerpb.Decl{Decl: &compilerpb.Decl_Empty_{Empty: &compilerpb.Decl_Empty{
			Span: c.span(decl),
		}}}

	case DeclKindSyntax:
		decl := decl.AsSyntax()

		var kind compilerpb.Decl_Syntax_Kind
		if decl.IsSyntax() {
			kind = compilerpb.Decl_Syntax_KIND_SYNTAX
		} else if decl.IsEdition() {
			kind = compilerpb.Decl_Syntax_KIND_EDITION
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Syntax_{Syntax: &compilerpb.Decl_Syntax{
			Kind:          kind,
			Value:         c.expr(decl.Value()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.Keyword()),
			EqualsSpan:    c.span(decl.Equals()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}}}

	case DeclKindPackage:
		decl := decl.AsPackage()

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Package_{Package: &compilerpb.Decl_Package{
			Path:          c.path(decl.Path()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.Keyword()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}}}

	case DeclKindImport:
		decl := decl.AsImport()

		var mod compilerpb.Decl_Import_Modifier
		if decl.IsWeak() {
			mod = compilerpb.Decl_Import_MODIFIER_WEAK
		} else if decl.IsPublic() {
			mod = compilerpb.Decl_Import_MODIFIER_PUBLIC
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Import_{Import: &compilerpb.Decl_Import{
			Modifier:       mod,
			ImportPath:     c.expr(decl.ImportPath()),
			Options:        c.options(decl.Options()),
			Span:           c.span(decl),
			KeywordSpan:    c.span(decl.Keyword()),
			ModifierSpan:   c.span(decl.Modifier()),
			ImportPathSpan: c.span(decl.ImportPath()),
			SemicolonSpan:  c.span(decl.Semicolon()),
		}}}

	case DeclKindBody:
		decl := decl.AsBody()

		proto := &compilerpb.Decl_Body{
			Span: c.span(decl),
		}
		decl.Iter(func(_ int, d DeclAny) bool {
			proto.Decls = append(proto.Decls, c.decl(d))
			return true
		})
		return &compilerpb.Decl{Decl: &compilerpb.Decl_Body_{Body: proto}}

	case DeclKindRange:
		decl := decl.AsRange()

		var kind compilerpb.Decl_Range_Kind
		if decl.IsExtensions() {
			kind = compilerpb.Decl_Range_KIND_EXTENSIONS
		} else if decl.IsReserved() {
			kind = compilerpb.Decl_Range_KIND_RESERVED
		}

		proto := &compilerpb.Decl_Range{
			Kind:          kind,
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.Keyword()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}

		decl.Iter(func(_ int, e ExprAny) bool {
			proto.Ranges = append(proto.Ranges, c.expr(e))
			return true
		})

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Range_{Range: proto}}

	case DeclKindDef:
		decl := decl.AsDef()

		var kind compilerpb.Def_Kind
		switch decl.Classify() {
		case DefKindMessage:
			kind = compilerpb.Def_KIND_MESSAGE
		case DefKindEnum:
			kind = compilerpb.Def_KIND_ENUM
		case DefKindService:
			kind = compilerpb.Def_KIND_SERVICE
		case DefKindExtend:
			kind = compilerpb.Def_KIND_EXTEND
		case DefKindField:
			kind = compilerpb.Def_KIND_FIELD
		case DefKindEnumValue:
			kind = compilerpb.Def_KIND_ENUM_VALUE
		case DefKindOneof:
			kind = compilerpb.Def_KIND_ONEOF
		case DefKindGroup:
			kind = compilerpb.Def_KIND_GROUP
		case DefKindMethod:
			kind = compilerpb.Def_KIND_METHOD
		case DefKindOption:
			kind = compilerpb.Def_KIND_OPTION
		}

		proto := &compilerpb.Def{
			Kind:          kind,
			Name:          c.path(decl.Name()),
			Value:         c.expr(decl.Value()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.Keyword()),
			EqualsSpan:    c.span(decl.Equals()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}

		if kind == compilerpb.Def_KIND_FIELD || kind == compilerpb.Def_KIND_UNSPECIFIED {
			proto.Type = c.type_(decl.Type())
		}

		if signature := decl.Signature(); !signature.Nil() {
			proto.Signature = &compilerpb.Def_Signature{
				Span:        c.span(signature),
				InputSpan:   c.span(signature.Inputs()),
				ReturnsSpan: c.span(signature.Returns()),
				OutputSpan:  c.span(signature.Outputs()),
			}

			signature.Inputs().Iter(func(_ int, t TypeAny) bool {
				proto.Signature.Inputs = append(proto.Signature.Inputs, c.type_(t))
				return true
			})
			signature.Outputs().Iter(func(_ int, t TypeAny) bool {
				proto.Signature.Outputs = append(proto.Signature.Outputs, c.type_(t))
				return true
			})
		}

		if body := decl.Body(); !body.Nil() {
			proto.Body = &compilerpb.Decl_Body{
				Span: c.span(decl.Body()),
			}
			body.Iter(func(_ int, d DeclAny) bool {
				proto.Body.Decls = append(proto.Body.Decls, c.decl(d))
				return true
			})
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Def{Def: proto}}

	default:
		panic(fmt.Sprintf("typeToProto: unknown DeclKind: %d", k))
	}
}

func (c *codec) options(options CompactOptions) *compilerpb.Options {
	if options.Nil() {
		return nil
	}
	defer c.check(options)()

	proto := &compilerpb.Options{
		Span: c.span(options),
	}

	options.Iter(func(_ int, o Option) bool {
		proto.Entries = append(proto.Entries, &compilerpb.Options_Entry{
			Path:       c.path(o.Path),
			Value:      c.expr(o.Value),
			EqualsSpan: c.span(o.Equals),
		})
		return true
	})

	return proto
}

func (c *codec) expr(expr ExprAny) *compilerpb.Expr {
	if expr.Nil() {
		return nil
	}
	defer c.check(expr)()

	switch k := expr.Kind(); k {
	case ExprKindLiteral:
		expr := expr.AsLiteral()

		proto := &compilerpb.Expr_Literal{
			Span: c.span(expr),
		}
		if v, ok := expr.Token.AsInt(); ok {
			proto.Value = &compilerpb.Expr_Literal_IntValue{IntValue: v}
		} else if v, ok := expr.Token.AsFloat(); ok {
			proto.Value = &compilerpb.Expr_Literal_FloatValue{FloatValue: v}
		} else if v, ok := expr.Token.AsString(); ok {
			proto.Value = &compilerpb.Expr_Literal_StringValue{StringValue: v}
		}
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Literal_{Literal: proto}}

	case ExprKindPath:
		expr := expr.AsPath()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Path{Path: c.path(expr.Path)}}

	case ExprKindPrefixed:
		expr := expr.AsPrefixed()

		return &compilerpb.Expr{Expr: &compilerpb.Expr_Prefixed_{Prefixed: &compilerpb.Expr_Prefixed{
			Prefix:     compilerpb.Expr_Prefixed_Prefix(expr.Prefix()),
			Expr:       c.expr(expr.Expr()),
			Span:       c.span(expr),
			PrefixSpan: c.span(expr.PrefixToken()),
		}}}

	case ExprKindRange:
		expr := expr.AsRange()

		start, end := expr.Bounds()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Range_{Range: &compilerpb.Expr_Range{
			Start:  c.expr(start),
			End:    c.expr(end),
			Span:   c.span(expr),
			ToSpan: c.span(expr.Keyword()),
		}}}

	case ExprKindArray:
		expr := expr.AsArray()

		proto := &compilerpb.Expr_Array{
			Span: c.span(expr),
		}
		expr.Iter(func(_ int, e ExprAny) bool {
			proto.Elements = append(proto.Elements, c.expr(e))
			return true
		})
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Array_{Array: proto}}

	case ExprKindDict:
		expr := expr.AsDict()

		proto := &compilerpb.Expr_Dict{
			Span: c.span(expr),
		}
		expr.Iter(func(_ int, e ExprField) bool {
			proto.Entries = append(proto.Entries, c.exprField(e))
			return true
		})
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Dict_{Dict: proto}}

	case ExprKindField:
		expr := expr.AsField()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Kv_{Kv: c.exprField(expr)}}
	}

	panic(fmt.Sprint("typeToProto: unknown Expr implementation:", reflect.TypeOf(expr)))
}

func (c *codec) exprField(expr ExprField) *compilerpb.Expr_Kv {
	if expr.Nil() {
		return nil
	}

	return &compilerpb.Expr_Kv{
		Key:       c.expr(expr.Key()),
		Value:     c.expr(expr.Value()),
		Span:      c.span(expr),
		ColonSpan: c.span(expr.Colon()),
	}
}

func (c *codec) type_(ty TypeAny) *compilerpb.Type {
	if ty.Nil() {
		return nil
	}
	defer c.check(ty)()

	switch k := ty.Kind(); k {
	case TypeKindPath:
		ty := ty.AsPath()
		return &compilerpb.Type{Type: &compilerpb.Type_Path{Path: c.path(ty.Path)}}

	case TypeKindPrefixed:
		ty := ty.AsPrefixed()
		return &compilerpb.Type{Type: &compilerpb.Type_Prefixed_{Prefixed: &compilerpb.Type_Prefixed{
			Prefix:     compilerpb.Type_Prefixed_Prefix(ty.Prefix()),
			Type:       c.type_(ty.Type()),
			Span:       c.span(ty),
			PrefixSpan: c.span(ty.PrefixToken()),
		}}}

	case TypeKindGeneric:
		ty := ty.AsGeneric()
		generic := &compilerpb.Type_Generic{
			Path:        c.path(ty.Path()),
			Span:        c.span(ty),
			BracketSpan: c.span(ty.Args()),
		}
		ty.Args().Iter(func(_ int, t TypeAny) bool {
			generic.Args = append(generic.Args, c.type_(t))
			return true
		})
		return &compilerpb.Type{Type: &compilerpb.Type_Generic_{Generic: generic}}
	}

	panic(fmt.Sprint("typeToProto: unknown Type implementation:", reflect.TypeOf(ty)))
}
