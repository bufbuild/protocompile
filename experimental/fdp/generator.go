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

package fdp

import (
	"math"
	"slices"
	"strconv"

	descriptorv1 "buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go/buf/descriptor/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/tags"
)

type generator struct {
	currentFile                  *ir.File
	generateExtraOptionLocations bool
	exclude                      func(*ir.File) bool

	debug *debug
}

func (g *generator) files(files []*ir.File, fds *descriptorpb.FileDescriptorSet) {
	// Build up all of the imported files. We can't just pull out the transitive
	// imports for each file because we want the result to be sorted
	// topologically.
	for file := range ir.TopoSort(files) {
		if g.exclude != nil && g.exclude(file) {
			continue
		}

		fdp := new(descriptorpb.FileDescriptorProto)
		fds.File = append(fds.File, fdp)

		g.file(file, fdp)
	}
}

func (g *generator) file(file *ir.File, fdp *descriptorpb.FileDescriptorProto) {
	g.currentFile = file
	g.debug.init(file)
	defer g.debug.done(&fdp.SourceCodeInfo)

	fdp.Name = addr(file.Path())

	if file.Package() != "" {
		fdp.Package = addr(string(file.Package()))
	}
	g.debug.comments(file.AST().Package(), tags.File_Package)

	// A syntax descriptor is only populated if the syntax is not proto2. Proto2 is considered
	// the default and is left empty, in conformance with protoc.
	// https://protobuf.com/docs/descriptors#file-descriptors
	if file.Syntax() != syntax.Proto2 {
		if file.Syntax().IsEdition() {
			fdp.Syntax = addr("editions")
			fdp.Edition = descriptorpb.Edition(file.Syntax()).Enum()
		} else {
			fdp.Syntax = addr(file.Syntax().String())
		}
	}

	// According to descriptor.proto and protoc behavior, the path is always set to [12]
	// for both syntax and editions.
	g.debug.comments(file.AST().Syntax(), tags.File_Syntax)

	g.debug.extensions(func(extn *descriptorv1.SourceCodeInfoExtension) {
		extn.IsSyntaxUnspecified = file.AST().Syntax().IsZero()
	})

	// Canonicalize import order so that it does not change whenever we refactor
	// internal structures.
	imports := seq.ToSlice(file.Imports())
	slices.SortFunc(imports, cmpx.Key(func(imp ir.Import) int {
		return imp.Decl.KeywordToken().Span().Start
	}))

	var publicDepIndex, weakDepIndex, optionDepIndex int32
	for i, imp := range imports {
		if !imp.Option {
			fdp.Dependency = append(fdp.Dependency, imp.Path())
			g.debug.comments(imp.Decl, tags.File_Dependency, int32(i))
		}

		switch {
		case imp.Public:
			fdp.PublicDependency = append(fdp.PublicDependency, int32(i))
			_, public := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Public
			})

			g.debug.span(public, tags.File_PublicDependency, publicDepIndex)
			publicDepIndex++

		case imp.Weak:
			fdp.WeakDependency = append(fdp.WeakDependency, int32(i))
			_, weak := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Weak
			})

			g.debug.span(weak, tags.File_WeakDependency, weakDepIndex)
			weakDepIndex++

		case imp.Option:
			fdp.OptionDependency = append(fdp.OptionDependency, imp.Path())

			g.debug.comments(imp.Decl, tags.File_OptionDependency, optionDepIndex)
			optionDepIndex++
		}

		if !imp.Used {
			g.debug.extensions(func(extn *descriptorv1.SourceCodeInfoExtension) {
				extn.UnusedDependency = append(extn.UnusedDependency, int32(i))
			})
		}
	}

	var msgIndex, enumIndex int32
	for ty := range seq.Values(file.Types()) {
		if ty.IsEnum() {
			g.debug.in(tags.File_EnumType, enumIndex)(func() {
				g.enum(ty, slicesx.PushNew(&fdp.EnumType))
			})

			enumIndex++
			continue
		}

		g.debug.in(tags.File_MessageType, msgIndex)(func() {
			g.message(ty, slicesx.PushNew(&fdp.MessageType))
		})
		msgIndex++
	}

	for i, service := range seq.All(file.Services()) {
		g.debug.in(tags.File_Service, int32(i))(func() {
			g.service(service, slicesx.PushNew(&fdp.Service))
		})
	}

	var extnIndex int32
	for extend := range seq.Values(file.Extends()) {
		g.debug.comments(extend.AST(), tags.File_Extension)

		for extn := range seq.Values(extend.Extensions()) {
			g.debug.in(tags.File_Extension, extnIndex)(func() {
				g.field(extn, slicesx.PushNew(&fdp.Extension))
			})
			extnIndex++
		}
	}

	if options := file.Options(); !options.IsEmpty() {
		g.debug.in(tags.File_Options)(func() {
			for option := range file.AST().Options() {
				g.debug.span(option)
			}

			fdp.Options = new(descriptorpb.FileOptions)
			g.options(options, fdp.Options)
		})
	}
}

func (g *generator) message(ty ir.Type, mdp *descriptorpb.DescriptorProto) {
	if ty.IsMapEntry() {
		// Do not add source locations for map entry types.
		defer g.debug.suppress()()
	}

	ast := ty.AST().AsMessage()
	g.debug.comments(ast)

	mdp.Name = addr(ty.Name())
	g.debug.span(ast.Name, tags.Message_Name)

	for i, field := range seq.All(ty.Members()) {
		g.debug.in(tags.Message_Field, int32(i))(func() {
			g.field(field, slicesx.PushNew(&mdp.Field))
		})
	}

	var extnIndex int32
	for extend := range seq.Values(ty.Extends()) {
		g.debug.comments(extend.AST(), tags.Message_Extension)

		for extn := range seq.Values(extend.Extensions()) {
			g.debug.in(tags.Message_Extension, extnIndex)(func() {
				g.field(extn, slicesx.PushNew(&mdp.Extension))
			})
			extnIndex++
		}
	}

	var enumIndex, nestedMsgIndex int32
	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			g.debug.in(tags.Message_EnumType, enumIndex)(func() {
				g.enum(ty, slicesx.PushNew(&mdp.EnumType))
				enumIndex++
			})
			continue
		}

		g.debug.in(tags.Message_NestedType, nestedMsgIndex)(func() {
			g.message(ty, slicesx.PushNew(&mdp.NestedType))
		})
		nestedMsgIndex++
	}

	for i, extensions := range seq.All(ty.ExtensionRanges()) {
		g.debug.comments(extensions.DeclAST(), tags.Message_ExtensionRange)
		g.debug.in(tags.Message_ExtensionRange, int32(i))(func() {
			g.rangeSourceCodeInfo(extensions.AST(),
				tags.Message_ExtensionRange_Start,
				tags.Message_ExtensionRange_End)

			er := slicesx.PushNew(&mdp.ExtensionRange)
			start, end := extensions.Range()
			er.Start = addr(start)
			er.End = addr(end + 1) // Exclusive.

			if options := extensions.Options(); !options.IsEmpty() {
				g.debug.in(tags.Message_ExtensionRange_Options)(func() {
					g.debug.span(extensions.DeclAST().Options())

					er.Options = new(descriptorpb.ExtensionRangeOptions)
					g.options(options, er.Options)
				})
			}
		})
	}

	didFirst := false
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !didFirst {
			g.debug.comments(reserved.DeclAST(), tags.Message_ReservedRange)
			didFirst = true
		}

		g.debug.in(tags.Message_ReservedRange, int32(i))(func() {
			g.rangeSourceCodeInfo(reserved.AST(),
				tags.Message_ReservedRange_Start,
				tags.Message_ReservedRange_End)

			er := slicesx.PushNew(&mdp.ReservedRange)
			start, end := reserved.Range()
			er.Start = addr(start)
			er.End = addr(end + 1) // Exclusive.
		})
	}

	didFirst = false
	for i, name := range seq.All(ty.ReservedNames()) {
		if !didFirst {
			g.debug.comments(name.DeclAST(), tags.Message_ReservedName)
			didFirst = true
		}
		g.debug.span(name.AST(), tags.Message_ReservedName, int32(i))
		mdp.ReservedName = append(mdp.ReservedName, name.Name())
	}

	for i, oneof := range seq.All(ty.Oneofs()) {
		g.debug.in(tags.Message_OneofDecl, int32(i))(func() {
			g.oneof(oneof, slicesx.PushNew(&mdp.OneofDecl))
		})
	}

	if g.currentFile.Syntax() == syntax.Proto3 {
		// Only now that we have added all of the normal oneofs do we add the
		// synthetic oneofs.
		for i, field := range seq.All(ty.Members()) {
			oneof := field.SyntheticOneofName()
			if oneof == "" {
				continue
			}

			fdp := mdp.Field[i]
			fdp.Proto3Optional = addr(true)
			fdp.OneofIndex = addr(int32(len(mdp.OneofDecl)))
			slicesx.PushNew(&mdp.OneofDecl).Name = addr(oneof)
		}
	}

	if options := ty.Options(); !options.IsEmpty() {
		g.debug.in(tags.Message_Options)(func() {
			for option := range ast.Body.Options() {
				g.debug.span(option)
			}
			mdp.Options = new(descriptorpb.MessageOptions)
			g.options(options, mdp.Options)
		})
	}

	mdp.Visibility = visibility(ty)
}

func (g *generator) field(f ir.Member, fdp *descriptorpb.FieldDescriptorProto) {
	if f.IsSynthetic() {
		defer g.debug.suppress()()
	}

	// If a field is a proto2 group field, the leading comments of the field are instead
	// attributed to the synthetic message for the group field, rather than the field itself,
	// so no tokens are checked for comments.
	ast := f.AST().AsField()
	g.debug.maybeComments(ast, !f.IsGroup())

	fdp.Name = addr(f.Name())
	g.debug.span(ast.Name, tags.Field_Name)

	fdp.Number = addr(f.Number())
	g.debug.span(ast.Tag, tags.Field_Number)

	switch label := f.Presence(); label {
	case presence.Explicit, presence.Implicit, presence.Shared:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
		if label == presence.Explicit && g.currentFile.Syntax() == syntax.Proto3 {
			// For proto3, if the presence is set explicitly with "optional", we need to set
			// "proto3_optional" field to true.
			fdp.Proto3Optional = addr(true)
		}
	case presence.Repeated:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	case presence.Required:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()
	}

	// Note: for specifically Protobuf fields, we expect a single prefix. The protocompile
	// AST allows for arbitrary nesting of prefixes, so the API returns an iterator, but
	// [descriptorpb.FieldDescriptorProto] expects a single label.
	for prefix := range ast.Type.Prefixes() {
		g.debug.span(prefix.PrefixToken(), tags.Field_Label)
	}

	if ty := f.Element(); !ty.IsZero() {
		tag := int32(tags.Field_Type)
		fdp.Type = f.FDPType().Enum()
		if !ty.IsPredeclared() {
			fdp.TypeName = addr(string(ty.FullName().ToAbsolute()))
			tag = tags.Field_TypeName
		}
		g.debug.span(ast.Type.RemovePrefixes(), tag)
	}

	if f.IsExtension() && f.Container().FullName() != "" {
		fdp.Extendee = addr(string(f.Container().FullName().ToAbsolute()))
		g.debug.span(f.Extend().AST().Name(), tags.Field_Extendee)
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	if options := f.Options(); !options.IsEmpty() {
		g.debug.in(tags.Field_Options)(func() {
			g.debug.span(ast.Options)
			fdp.Options = new(descriptorpb.FieldOptions)
			g.options(options, fdp.Options)
		})
	}

	// If this field is part of a map entry type, we need to grab all the explicitly set
	// feature options from the originating map field and add them to this field.
	//
	// There should be no options on the synthetic field prior to this.
	if mapField := f.Container().MapField(); !mapField.IsZero() {
		if mapFieldFeatures := mapField.FeatureSet().Options(); !mapFieldFeatures.IsZero() {
			fdp.Options = new(descriptorpb.FieldOptions)
			fdp.Options.Features = new(descriptorpb.FeatureSet)
			fdp.Options.Features.ProtoReflect().SetUnknown(mapFieldFeatures.Marshal(nil, nil))
		}
	}

	fdp.JsonName = addr(f.JSONName())
	if jsonName := f.PseudoOptions().JSONName; !jsonName.IsZero() {
		g.debug.span(jsonName.OptionSpan(), tags.Field_JsonName)
	}

	if d := f.PseudoOptions().Default; !d.IsZero() {
		if v, ok := d.AsBool(); ok {
			fdp.DefaultValue = addr(strconv.FormatBool(v))
		} else if v := d.AsEnum(); !v.IsZero() {
			fdp.DefaultValue = addr(v.Name())
		} else if v, ok := d.AsInt(); ok {
			fdp.DefaultValue = addr(strconv.FormatInt(v, 10))
		} else if v, ok := d.AsUInt(); ok {
			fdp.DefaultValue = addr(strconv.FormatUint(v, 10))
		} else if v, ok := d.AsFloat(); ok {
			switch {
			case math.IsInf(v, 1):
				fdp.DefaultValue = addr("inf")
			case math.IsInf(v, -1):
				fdp.DefaultValue = addr("-inf")
			case math.IsNaN(v):
				fdp.DefaultValue = addr("nan") // Goodbye NaN payload. :(
			default:
				fdp.DefaultValue = addr(strconv.FormatFloat(v, 'g', -1, 64))
			}
		} else if v, ok := d.AsString(); ok {
			if f.Element().Predeclared() == predeclared.Bytes {
				// For bytes fields, the default value needs to be escaped.
				// Reference for default value encoding:
				// https://protobuf.com/docs/descriptors#encoding-default-values
				v = internal.EscapeBytes(v)
			}

			fdp.DefaultValue = addr(v)
		}

		g.debug.span(d.OptionSpan(), tags.Field_DefaultValue)
	}
}

func (g *generator) oneof(o ir.Oneof, odp *descriptorpb.OneofDescriptorProto) {
	ast := o.AST().AsOneof()
	g.debug.comments(ast)

	odp.Name = addr(o.Name())
	g.debug.span(ast.Name, tags.Oneof_Name)

	if options := o.Options(); !options.IsEmpty() {
		g.debug.in(tags.Oneof_Options)(func() {
			for option := range ast.Body.Options() {
				g.debug.span(option)
			}
			odp.Options = new(descriptorpb.OneofOptions)
			g.options(options, odp.Options)
		})
	}
}

func (g *generator) enum(ty ir.Type, edp *descriptorpb.EnumDescriptorProto) {
	ast := ty.AST().AsEnum()
	g.debug.comments(ast)

	edp.Name = addr(ty.Name())
	g.debug.span(ast.Name, tags.Enum_Name)

	for i, v := range seq.All(ty.Members()) {
		g.debug.in(tags.Enum_Value, int32(i))(func() {
			g.enumValue(v, slicesx.PushNew(&edp.Value))
		})
	}

	didFirst := false
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !didFirst {
			g.debug.comments(reserved.DeclAST(), tags.Enum_ReservedRange)
			didFirst = true
		}

		g.debug.in(tags.Enum_ReservedRange, int32(i))(func() {
			g.rangeSourceCodeInfo(reserved.AST(),
				tags.Enum_ReservedRange_Start,
				tags.Enum_ReservedRange_End)

			er := slicesx.PushNew(&edp.ReservedRange)
			start, end := reserved.Range()
			er.Start = addr(start)
			er.End = addr(end) // Inclusive, not exclusive like the one for messages!
		})
	}

	didFirst = false
	for i, name := range seq.All(ty.ReservedNames()) {
		if !didFirst {
			g.debug.comments(name.DeclAST(), tags.Enum_ReservedName)
			didFirst = true
		}
		g.debug.span(name.AST(), tags.Enum_ReservedName, int32(i))
		edp.ReservedName = append(edp.ReservedName, name.Name())
	}

	if options := ty.Options(); !options.IsEmpty() {
		g.debug.in(tags.Enum_Options)(func() {
			for option := range ast.Body.Options() {
				g.debug.span(option)
			}
			edp.Options = new(descriptorpb.EnumOptions)
			g.options(options, edp.Options)
		})
	}

	edp.Visibility = visibility(ty)
}

func (g *generator) enumValue(f ir.Member, evdp *descriptorpb.EnumValueDescriptorProto) {
	ast := f.AST().AsEnumValue()
	g.debug.comments(ast)

	evdp.Name = addr(f.Name())
	g.debug.span(ast.Name, tags.EnumValue_Name)

	evdp.Number = addr(f.Number())
	g.debug.span(ast.Tag, tags.EnumValue_Number)

	if options := f.Options(); !options.IsEmpty() {
		g.debug.in(tags.EnumValue_Options)(func() {
			g.debug.span(ast.Options)
			evdp.Options = new(descriptorpb.EnumValueOptions)
			g.options(options, evdp.Options)
		})
	}
}

func (g *generator) service(s ir.Service, sdp *descriptorpb.ServiceDescriptorProto) {
	ast := s.AST().AsService()
	g.debug.comments(ast)

	sdp.Name = addr(s.Name())
	g.debug.span(ast.Name, tags.Service_Name)

	for i, method := range seq.All(s.Methods()) {
		g.debug.in(tags.Service_Method, int32(i))(func() {
			g.method(method, slicesx.PushNew(&sdp.Method))
		})
	}

	if options := s.Options(); !options.IsEmpty() {
		g.debug.in(tags.Service_Options)(func() {
			for option := range ast.Body.Options() {
				g.debug.span(option)
			}
			sdp.Options = new(descriptorpb.ServiceOptions)
			g.options(options, sdp.Options)
		})
	}
}

func (g *generator) method(m ir.Method, mdp *descriptorpb.MethodDescriptorProto) {
	doTy := func(
		tys ast.TypeList,
		ty ir.Type, stream bool,
		typeTag, streamTag int32,
		typeField **string,
		streamField **bool,
	) {
		*typeField = addr(string(ty.FullName().ToAbsolute()))
		if stream {
			*streamField = addr(stream)
		}

		// Methods only have a single input/output, see [descriptorpb.MethodDescriptorProto].
		ast := tys.At(0)
		if prefixed := ast.AsPrefixed(); !prefixed.IsZero() {
			g.debug.span(prefixed.PrefixToken(), streamTag)
		}
		g.debug.span(ast.RemovePrefixes(), typeTag)
	}

	ast := m.AST().AsMethod()
	g.debug.comments(ast)

	mdp.Name = addr(m.Name())
	g.debug.span(ast.Name, tags.Method_Name)

	ty, stream := m.Input()
	doTy(ast.Signature.Inputs(), ty, stream,
		tags.Method_InputType, tags.Method_ClientStreaming,
		&mdp.InputType, &mdp.ClientStreaming)

	ty, stream = m.Output()
	doTy(ast.Signature.Outputs(), ty, stream,
		tags.Method_OutputType, tags.Method_ServerStreaming,
		&mdp.OutputType, &mdp.ServerStreaming)

	// protoc populates options as long as the body is non-zero, even if options are empty.
	if options := m.Options(); !ast.Body.IsZero() {
		g.debug.in(tags.Method_Options)(func() {
			for option := range ast.Body.Options() {
				g.debug.span(option)
			}
			mdp.Options = new(descriptorpb.MethodOptions)
			g.options(options, mdp.Options)
		})
	}
}

func (g *generator) options(v ir.MessageValue, target proto.Message) {
	target.ProtoReflect().SetUnknown(v.Marshal(nil, nil))
	if g.debug == nil {
		return
	}

	var rec func(ir.MessageValue)
	rec = func(v ir.MessageValue) {
		for field := range v.Fields() {
			var optionSpanIndex int32
			for ast := range seq.Values(field.OptionSpans()) {
				if ast == nil {
					continue
				}

				if messageField := field.AsMessage(); !messageField.IsZero() {
					if field.IsTopLevel() {
						// If this is a top-level option declaration for a message type with a message
						// literal, we add a location for the declaration.
						g.debug.span(ast, field.Field().Number())

						if !g.generateExtraOptionLocations {
							// If the option [GenerateExtraOptionLocations] is not set, then continue
							// without adding source locations for elements within the value.
							continue
						}
					}
					g.debug.in(field.Field().Number())(func() {
						rec(messageField)
					})
					continue
				}

				// For declarations with bodies, e.g. messages, enums, services, methods, files,
				// leading and trailing comments are attributed on the option declarations based on
				// the option keyword and semicolon, respectively, e.g.
				//
				// message Foo {
				//   // Leading comment for the following option declaration, (a) = 10.
				//   option (a) = 10;
				//   option (b) = 20; // Trailing comment for the option declaration (b) = 20.
				// }
				//
				// However, the optionSpan in the IR does not capture the keyword and semicolon
				// tokens. In addition to the comments, the span including the option keyword and
				// semicolon is needed for the source location.
				//
				// So this hack checks the non-skippable token directly before and after the
				// optionSpan for the option keyword and semicolon tokens respectively.
				//
				// For declarations with compact options, e.g. fields, enum values, there are no
				// comments attributed to the option spans, e.g.
				//
				// message Foo {
				//   string name = 1 [
				//     // This is dropped.
				//     (c) = 15, // This is also dropped.
				//   ]
				// }
				span := ast.Span()
				keyword, semicolon := g.optionKeywordAndSemicolon(span)
				checkForComments := false
				if !keyword.IsZero() && !semicolon.IsZero() {
					span = source.Between(keyword.Span(), semicolon.Span())
					checkForComments = true
				}

				if field.Field().IsRepeated() {
					g.debug.maybeComments(span, checkForComments, field.Field().Number(), optionSpanIndex)
					optionSpanIndex++
				} else {
					g.debug.maybeComments(span, checkForComments, field.Field().Number())
				}
			}
		}
	}

	rec(v)
}

// optionKeywordAndSemicolon is a helper function that checks the non-skippable tokens
// before and after the given span. If the non-skippable token before is the option keyword
// and the non-skippable token after is the semicolon, then both are returned.
func (g *generator) optionKeywordAndSemicolon(span source.Span) (token.Token, token.Token) {
	_, start := g.currentFile.AST().Stream().Around(span.Start)
	before := token.NewCursorAt(start)
	prev := before.Prev()
	if prev.Keyword() != keyword.Option {
		return token.Zero, token.Zero
	}

	_, end := g.currentFile.AST().Stream().Around(span.End)
	after := token.NewCursorAt(end)
	next := after.Next()
	if next.Keyword() != keyword.Semi {
		return token.Zero, token.Zero
	}

	return prev, next
}

func (g *generator) rangeSourceCodeInfo(expr ast.ExprAny, startTag, endTag int32) {
	g.debug.span(expr)

	start := expr.Span()
	end := expr.Span()
	if expr.Kind() == ast.ExprKindRange {
		a, b := expr.AsRange().Bounds()
		start, end = a.Span(), b.Span()
	}

	g.debug.span(start, startTag)
	g.debug.span(end, endTag)
}

func visibility(ty ir.Type) *descriptorpb.SymbolVisibility {
	switch exported, explicit := ty.IsExported(); {
	case !explicit:
		return nil
	case exported:
		return descriptorpb.SymbolVisibility_VISIBILITY_EXPORT.Enum()
	default:
		return descriptorpb.SymbolVisibility_VISIBILITY_LOCAL.Enum()
	}
}

// addr is a helper for creating a pointer out of any type, because Go is
// missing the syntax &"foo", etc.
func addr[T any](v T) *T { return &v }
