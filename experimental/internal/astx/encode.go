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

package astx

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
)

// ToProtoOptions contains options for the [File.ToProto] function.
type ToProtoOptions struct {
	// If set, no spans will be serialized.
	//
	// This operation only destroys non-semantic information.
	OmitSpans bool

	// If set, the contents of the file the AST was parsed from will not
	// be serialized.
	OmitFile bool
}

// ToProto converts this AST into a Protobuf representation, which may be
// serialized.
//
// Note that package ast does not support deserialization from this proto;
// instead, you will need to re-parse the text file included in the message.
// This is because the AST is much richer than what is stored in this message;
// the message only provides enough information for further semantic analysis
// and diagnostic generation, but not for pretty-printing.
//
// Panics if the AST contains a cycle (e.g. a message that contains itself as
// a nested message). Parsed ASTs will never contain cycles, but users may
// modify them into a cyclic state.
func ToProto(f *ast.File, options ToProtoOptions) proto.Message {
	return (&protoEncoder{ToProtoOptions: options}).file(f) // See codec.go
}

// protoEncoder is the state needed for converting an AST node into a Protobuf message.
type protoEncoder struct {
	ToProtoOptions

	stack    []source.Spanner
	stackMap map[source.Spanner]struct{}
}

// checkCycle panics if v is visited cyclically.
//
// Should be called like this, so that on function exit the entry is popped:
//
//	defer c.checkCycle(v)()
func (c *protoEncoder) checkCycle(v source.Spanner) func() {
	// By default, we just perform a linear search, because inserting into
	// a map is extremely slow. However, if the stack gets tall enough, we
	// switch to using the map to avoid going quadratic.
	if len(c.stack) > 32 {
		c.stackMap = make(map[source.Spanner]struct{})
		for _, v := range c.stack {
			c.stackMap[v] = struct{}{}
		}
		c.stack = nil
	}

	var cycle bool
	if c.stackMap != nil {
		_, cycle = c.stackMap[v]
		c.stackMap[v] = struct{}{}
	} else {
		cycle = slices.Contains(c.stack, v)
		c.stack = append(c.stack, v)
	}

	if cycle {
		panic(fmt.Sprintf("protocompile/ast: called File.ToProto on a cyclic AST %v", v.Span()))
	}

	return func() {
		if c.stackMap != nil {
			delete(c.stackMap, v)
		} else {
			c.stack = c.stack[len(c.stack)-1:]
		}
	}
}

func (c *protoEncoder) file(file *ast.File) *compilerpb.File {
	proto := &compilerpb.File{
		Decls: slices.Collect(seq.Map(file.Decls(), c.decl)),
	}
	if !c.OmitFile {
		proto.File = &compilerpb.Report_File{
			Path: file.Stream().Path(),
			Text: []byte(file.Stream().Text()),
		}
	}
	return proto
}

func (c *protoEncoder) span(s source.Spanner) *compilerpb.Span {
	if c.OmitSpans || s == nil {
		return nil
	}

	span := s.Span()
	if span.IsZero() {
		return nil
	}

	return &compilerpb.Span{
		Start: uint32(span.Start),
		End:   uint32(span.End),
	}
}

// commas is a non-generic subinterface of Commas[T].
type commas interface {
	Len() int
	Comma(int) token.Token
}

func (c *protoEncoder) commas(cs commas) []*compilerpb.Span {
	if c.OmitSpans {
		return nil
	}

	spans := make([]*compilerpb.Span, cs.Len())
	for i := range spans {
		spans[i] = c.span(cs.Comma(i))
	}
	return spans
}

func (c *protoEncoder) path(path ast.Path) *compilerpb.Path {
	if path.IsZero() {
		return nil
	}
	defer c.checkCycle(path)()

	proto := &compilerpb.Path{
		Span: c.span(path),
	}
	for pc := range path.Components {
		component := new(compilerpb.Path_Component)
		switch pc.Separator().Text() {
		case ".":
			component.Separator = compilerpb.Path_Component_SEPARATOR_DOT
		case "/":
			component.Separator = compilerpb.Path_Component_SEPARATOR_SLASH
		}
		component.SeparatorSpan = c.span(pc.Separator())

		if extn := pc.AsExtension(); !extn.IsZero() {
			extn := pc.AsExtension()
			component.Component = &compilerpb.Path_Component_Extension{Extension: c.path(extn)}
			component.ComponentSpan = c.span(extn)
		} else if ident := pc.AsIdent(); !ident.IsZero() {
			component.Component = &compilerpb.Path_Component_Ident{Ident: ident.Name()}
			component.ComponentSpan = c.span(ident)
		}

		proto.Components = append(proto.Components, component)
	}
	return proto
}

func (c *protoEncoder) decl(decl ast.DeclAny) *compilerpb.Decl {
	if decl.IsZero() {
		return nil
	}
	defer c.checkCycle(decl)()

	switch k := decl.Kind(); k {
	case ast.DeclKindEmpty:
		decl := decl.AsEmpty()
		return &compilerpb.Decl{Decl: &compilerpb.Decl_Empty_{Empty: &compilerpb.Decl_Empty{
			Span: c.span(decl),
		}}}

	case ast.DeclKindSyntax:
		decl := decl.AsSyntax()

		var kind compilerpb.Decl_Syntax_Kind
		switch {
		case decl.IsSyntax():
			kind = compilerpb.Decl_Syntax_KIND_SYNTAX
		case decl.IsEdition():
			kind = compilerpb.Decl_Syntax_KIND_EDITION
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Syntax_{Syntax: &compilerpb.Decl_Syntax{
			Kind:          kind,
			Value:         c.expr(decl.Value()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.KeywordToken()),
			EqualsSpan:    c.span(decl.Equals()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}}}

	case ast.DeclKindPackage:
		decl := decl.AsPackage()

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Package_{Package: &compilerpb.Decl_Package{
			Path:          c.path(decl.Path()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.KeywordToken()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}}}

	case ast.DeclKindImport:
		decl := decl.AsImport()

		var mods []compilerpb.Decl_Import_Modifier
		var modSpans []*compilerpb.Span

		for mod := range seq.Values(decl.ModifierTokens()) {
			switch mod.Keyword() {
			case keyword.Public:
				mods = append(mods, compilerpb.Decl_Import_MODIFIER_PUBLIC)
			case keyword.Weak:
				mods = append(mods, compilerpb.Decl_Import_MODIFIER_WEAK)
			default: // Add support for keyword.Option whenever it gets added to ast.proto.
				mods = append(mods, compilerpb.Decl_Import_MODIFIER_UNSPECIFIED)
			}

			modSpans = append(modSpans, c.span(mod))
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Import_{Import: &compilerpb.Decl_Import{
			Modifier:       mods,
			ImportPath:     c.expr(decl.ImportPath()),
			Options:        c.options(decl.Options()),
			Span:           c.span(decl),
			KeywordSpan:    c.span(decl.KeywordToken()),
			ModifierSpan:   modSpans,
			ImportPathSpan: c.span(decl.ImportPath()),
			SemicolonSpan:  c.span(decl.Semicolon()),
		}}}

	case ast.DeclKindBody:
		decl := decl.AsBody()
		return &compilerpb.Decl{Decl: &compilerpb.Decl_Body_{Body: &compilerpb.Decl_Body{
			Span:  c.span(decl),
			Decls: slices.Collect(seq.Map(decl.Decls(), c.decl)),
		}}}

	case ast.DeclKindRange:
		decl := decl.AsRange()

		var kind compilerpb.Decl_Range_Kind
		if decl.IsExtensions() {
			kind = compilerpb.Decl_Range_KIND_EXTENSIONS
		} else if decl.IsReserved() {
			kind = compilerpb.Decl_Range_KIND_RESERVED
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Range_{Range: &compilerpb.Decl_Range{
			Kind:          kind,
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.KeywordToken()),
			SemicolonSpan: c.span(decl.Semicolon()),
			Ranges:        slices.Collect(seq.Map(decl.Ranges(), c.expr)),
		}}}

	case ast.DeclKindDef:
		decl := decl.AsDef()

		var kind compilerpb.Def_Kind
		switch decl.Classify() {
		case ast.DefKindMessage:
			kind = compilerpb.Def_KIND_MESSAGE
		case ast.DefKindEnum:
			kind = compilerpb.Def_KIND_ENUM
		case ast.DefKindService:
			kind = compilerpb.Def_KIND_SERVICE
		case ast.DefKindExtend:
			kind = compilerpb.Def_KIND_EXTEND
		case ast.DefKindField:
			kind = compilerpb.Def_KIND_FIELD
		case ast.DefKindEnumValue:
			kind = compilerpb.Def_KIND_ENUM_VALUE
		case ast.DefKindOneof:
			kind = compilerpb.Def_KIND_ONEOF
		case ast.DefKindGroup:
			kind = compilerpb.Def_KIND_GROUP
		case ast.DefKindMethod:
			kind = compilerpb.Def_KIND_METHOD
		case ast.DefKindOption:
			kind = compilerpb.Def_KIND_OPTION
		}

		proto := &compilerpb.Def{
			Kind:          kind,
			Name:          c.path(decl.Name()),
			Value:         c.expr(decl.Value()),
			Options:       c.options(decl.Options()),
			Span:          c.span(decl),
			KeywordSpan:   c.span(decl.KeywordToken()),
			EqualsSpan:    c.span(decl.Equals()),
			SemicolonSpan: c.span(decl.Semicolon()),
		}

		if kind == compilerpb.Def_KIND_FIELD ||
			kind == compilerpb.Def_KIND_GROUP ||
			kind == compilerpb.Def_KIND_UNSPECIFIED {
			proto.Type = c.type_(decl.Type())
		}

		if signature := decl.Signature(); !signature.IsZero() {
			proto.Signature = &compilerpb.Def_Signature{
				Span:        c.span(signature),
				InputSpan:   c.span(signature.Inputs()),
				ReturnsSpan: c.span(signature.Returns()),
				OutputSpan:  c.span(signature.Outputs()),
				Inputs:      slices.Collect(seq.Map(signature.Inputs(), c.type_)),
				Outputs:     slices.Collect(seq.Map(signature.Outputs(), c.type_)),
			}
		}

		if body := decl.Body(); !body.IsZero() {
			proto.Body = &compilerpb.Decl_Body{
				Span:  c.span(decl.Body()),
				Decls: slices.Collect(seq.Map(body.Decls(), c.decl)),
			}
		}

		return &compilerpb.Decl{Decl: &compilerpb.Decl_Def{Def: proto}}

	default:
		panic(fmt.Sprintf("protocompile/ast: unknown DeclKind: %d", k))
	}
}

func (c *protoEncoder) options(options ast.CompactOptions) *compilerpb.Options {
	if options.IsZero() {
		return nil
	}
	defer c.checkCycle(options)()

	return &compilerpb.Options{
		Span: c.span(options),
		Entries: slices.Collect(seq.Map(options.Entries(), func(o ast.Option) *compilerpb.Options_Entry {
			return &compilerpb.Options_Entry{
				Path:       c.path(o.Path),
				Value:      c.expr(o.Value),
				EqualsSpan: c.span(o.Equals),
			}
		})),
	}
}

func (c *protoEncoder) expr(expr ast.ExprAny) *compilerpb.Expr {
	if expr.IsZero() {
		return nil
	}
	defer c.checkCycle(expr)()

	switch k := expr.Kind(); k {
	case ast.ExprKindLiteral:
		expr := expr.AsLiteral()

		proto := &compilerpb.Expr_Literal{
			Span: c.span(expr),
		}
		switch expr.Kind() {
		case token.Number:
			if v, exact := expr.Token.AsNumber().Int(); exact {
				proto.Value = &compilerpb.Expr_Literal_IntValue{IntValue: v}
			} else {
				v, _ := expr.Token.AsNumber().Float()
				proto.Value = &compilerpb.Expr_Literal_FloatValue{FloatValue: v}
			}

		case token.String:
			proto.Value = &compilerpb.Expr_Literal_StringValue{StringValue: expr.AsString().Text()}

		default:
			panic(fmt.Sprintf("protocompile/ast: ExprLiteral contains neither string nor int: %v", expr.Token))
		}

		return &compilerpb.Expr{Expr: &compilerpb.Expr_Literal_{Literal: proto}}

	case ast.ExprKindPath:
		expr := expr.AsPath()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Path{Path: c.path(expr.Path)}}

	case ast.ExprKindPrefixed:
		expr := expr.AsPrefixed()

		var prefix compilerpb.Expr_Prefixed_Prefix
		if expr.Prefix() == keyword.Minus {
			prefix = compilerpb.Expr_Prefixed_PREFIX_MINUS
		}

		return &compilerpb.Expr{Expr: &compilerpb.Expr_Prefixed_{Prefixed: &compilerpb.Expr_Prefixed{
			Prefix:     prefix,
			Expr:       c.expr(expr.Expr()),
			Span:       c.span(expr),
			PrefixSpan: c.span(expr.PrefixToken()),
		}}}

	case ast.ExprKindRange:
		expr := expr.AsRange()

		start, end := expr.Bounds()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Range_{Range: &compilerpb.Expr_Range{
			Start:  c.expr(start),
			End:    c.expr(end),
			Span:   c.span(expr),
			ToSpan: c.span(expr.Keyword()),
		}}}

	case ast.ExprKindArray:
		expr := expr.AsArray()
		a, b := expr.Brackets().StartEnd()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Array_{Array: &compilerpb.Expr_Array{
			Span:       c.span(expr),
			OpenSpan:   c.span(a.LeafSpan()),
			CloseSpan:  c.span(b.LeafSpan()),
			CommaSpans: c.commas(expr.Elements()),
			Elements:   slices.Collect(seq.Map(expr.Elements(), c.expr)),
		}}}

	case ast.ExprKindDict:
		expr := expr.AsDict()
		a, b := expr.Braces().StartEnd()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Dict_{Dict: &compilerpb.Expr_Dict{
			Span:       c.span(expr),
			OpenSpan:   c.span(a.LeafSpan()),
			CloseSpan:  c.span(b.LeafSpan()),
			CommaSpans: c.commas(expr.Elements()),
			Entries:    slices.Collect(seq.Map(expr.Elements(), c.exprField)),
		}}}

	case ast.ExprKindField:
		expr := expr.AsField()
		return &compilerpb.Expr{Expr: &compilerpb.Expr_Field_{Field: c.exprField(expr)}}

	default:
		panic(fmt.Sprintf("protocompile/ast: unknown ExprKind: %d", k))
	}
}

func (c *protoEncoder) exprField(expr ast.ExprField) *compilerpb.Expr_Field {
	if expr.IsZero() {
		return nil
	}

	return &compilerpb.Expr_Field{
		Key:       c.expr(expr.Key()),
		Value:     c.expr(expr.Value()),
		Span:      c.span(expr),
		ColonSpan: c.span(expr.Colon()),
	}
}

//nolint:revive // "method type_ should be type" is incorrect because type is a keyword.
func (c *protoEncoder) type_(ty ast.TypeAny) *compilerpb.Type {
	if ty.IsZero() {
		return nil
	}
	defer c.checkCycle(ty)()

	switch k := ty.Kind(); k {
	case ast.TypeKindPath:
		ty := ty.AsPath()
		return &compilerpb.Type{Type: &compilerpb.Type_Path{Path: c.path(ty.Path)}}

	case ast.TypeKindPrefixed:
		ty := ty.AsPrefixed()

		var prefix compilerpb.Type_Prefixed_Prefix
		switch ty.Prefix() {
		case keyword.Optional:
			prefix = compilerpb.Type_Prefixed_PREFIX_OPTIONAL
		case keyword.Repeated:
			prefix = compilerpb.Type_Prefixed_PREFIX_REPEATED
		case keyword.Required:
			prefix = compilerpb.Type_Prefixed_PREFIX_REQUIRED
		case keyword.Stream:
			prefix = compilerpb.Type_Prefixed_PREFIX_STREAM
		}

		return &compilerpb.Type{Type: &compilerpb.Type_Prefixed_{Prefixed: &compilerpb.Type_Prefixed{
			Prefix:     prefix,
			Type:       c.type_(ty.Type()),
			Span:       c.span(ty),
			PrefixSpan: c.span(ty.PrefixToken()),
		}}}

	case ast.TypeKindGeneric:
		ty := ty.AsGeneric()

		a, b := ty.Args().Brackets().StartEnd()
		return &compilerpb.Type{Type: &compilerpb.Type_Generic_{Generic: &compilerpb.Type_Generic{
			Path:       c.path(ty.Path()),
			Span:       c.span(ty),
			OpenSpan:   c.span(a.LeafSpan()),
			CloseSpan:  c.span(b.LeafSpan()),
			CommaSpans: c.commas(ty.Args()),
			Args:       slices.Collect(seq.Map(ty.Args(), c.type_)),
		}}}

	default:
		panic(fmt.Sprintf("protocompile/ast: unknown TypeKind: %d", k))
	}
}
