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
)

type generator struct {
	currentFile      *ir.File
	includeDebugInfo bool
	exclude          func(*ir.File) bool

	path               *path
	sourceCodeInfo     *descriptorpb.SourceCodeInfo
	sourceCodeInfoExtn *descriptorv1.SourceCodeInfoExtension

	commentTracker *commentTracker
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
	fdp.Name = addr(file.Path())
	g.path = new(path)

	if g.includeDebugInfo {
		g.sourceCodeInfo = new(descriptorpb.SourceCodeInfo)
		fdp.SourceCodeInfo = g.sourceCodeInfo

		ct := new(commentTracker)
		g.commentTracker = ct
		ct.attributeComments(g.currentFile.AST().Stream().Cursor())

		g.sourceCodeInfoExtn = new(descriptorv1.SourceCodeInfoExtension)
		proto.SetExtension(g.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension, g.sourceCodeInfoExtn)
	}

	fdp.Package = addr(string(file.Package()))
	reset := g.path.with(internal.FilePackageTag)
	g.addSourceLocation(
		file.AST().Package().Span(),
		file.AST().Package().KeywordToken().ID(),
		file.AST().Package().Semicolon().ID(),
	)
	reset()

	if file.Syntax().IsEdition() {
		fdp.Syntax = addr("editions")
		fdp.Edition = descriptorpb.Edition(file.Syntax()).Enum()
	} else {
		fdp.Syntax = addr(file.Syntax().String())
	}
	// According to descriptor.proto and protoc behavior, the path is always set to [12] for
	// both syntax and editions.
	reset = g.path.with(internal.FileSyntaxTag)
	g.addSourceLocation(
		file.AST().Syntax().Span(),
		file.AST().Syntax().KeywordToken().ID(),
		file.AST().Syntax().Semicolon().ID(),
	)
	reset()

	if g.sourceCodeInfoExtn != nil {
		g.sourceCodeInfoExtn.IsSyntaxUnspecified = file.AST().Syntax().IsZero()
	}

	// Canonicalize import order so that it does not change whenever we refactor
	// internal structures.
	imports := seq.ToSlice(file.Imports())
	slices.SortFunc(imports, cmpx.Key(func(imp ir.Import) int {
		return imp.Decl.KeywordToken().Span().Start
	}))

	var publicDepIndex, weakDepIndex int32
	for i, imp := range imports {
		fdp.Dependency = append(fdp.Dependency, imp.Path())
		reset := g.path.with(internal.FileDependencyTag, int32(i))
		g.addSourceLocation(
			imp.Decl.Span(),
			imp.Decl.KeywordToken().ID(),
			imp.Decl.Semicolon().ID(),
		)
		reset()

		if imp.Public {
			fdp.PublicDependency = append(fdp.PublicDependency, int32(i))
			_, public := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Public
			})
			reset := g.path.with(internal.FilePublicDependencyTag, publicDepIndex)
			g.addSourceLocation(public.Span())
			reset()
			publicDepIndex++
		}
		if imp.Weak {
			fdp.WeakDependency = append(fdp.WeakDependency, int32(i))
			_, weak := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Weak
			})
			reset := g.path.with(internal.FileWeakDependencyTag, weakDepIndex)
			g.addSourceLocation(weak.Span())
			reset()
		}

		if g.sourceCodeInfoExtn != nil && !imp.Used {
			g.sourceCodeInfoExtn.UnusedDependency = append(g.sourceCodeInfoExtn.UnusedDependency, int32(i))
		}
	}

	var msgIndex, enumIndex int32
	for ty := range seq.Values(file.Types()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			fdp.EnumType = append(fdp.EnumType, edp)
			g.enum(ty, edp, internal.FileEnumsTag, enumIndex)
			enumIndex++
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		fdp.MessageType = append(fdp.MessageType, mdp)
		g.message(ty, mdp, internal.FileMessagesTag, msgIndex)
		msgIndex++
	}

	for i, service := range seq.All(file.Services()) {
		sdp := new(descriptorpb.ServiceDescriptorProto)
		fdp.Service = append(fdp.Service, sdp)
		g.service(service, sdp, internal.FileServicesTag, int32(i))
	}

	var extnIndex int32
	for extend := range seq.Values(file.Extends()) {
		reset := g.path.with(internal.FileExtensionsTag)
		g.addSourceLocation(
			extend.AST().Span(),
			extend.AST().KeywordToken().ID(),
			extend.AST().Body().Braces().ID(),
		)
		reset()

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			fdp.Extension = append(fdp.Extension, fd)
			g.field(extn, fd, internal.FileExtensionsTag, extnIndex)
			extnIndex++
		}
	}

	if options := file.Options(); !iterx.Empty(options.Fields()) {
		for option := range file.AST().Options() {
			reset := g.path.with(internal.FileOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}

		fdp.Options = new(descriptorpb.FileOptions)
		g.options(options, fdp.Options, internal.FileOptionsTag)
	}

	if g.sourceCodeInfoExtn != nil && iterx.Empty2(g.sourceCodeInfoExtn.ProtoReflect().Range) {
		proto.ClearExtension(g.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension)
	}

	if g.sourceCodeInfo != nil {
		slices.SortFunc(g.sourceCodeInfo.Location, func(a, b *descriptorpb.SourceCodeInfo_Location) int {
			return slices.Compare(a.Span, b.Span)
		})
		g.sourceCodeInfo.Location = append(
			[]*descriptorpb.SourceCodeInfo_Location{{Span: locationSpan(file.AST().Span())}},
			g.sourceCodeInfo.Location...,
		)
	}
}

func (g *generator) message(ty ir.Type, mdp *descriptorpb.DescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	messageAST := ty.AST().AsMessage()
	g.addSourceLocation(messageAST.Span(), messageAST.Keyword.ID(), messageAST.Body.Braces().ID())

	mdp.Name = addr(ty.Name())
	reset := g.path.with(internal.MessageNameTag)
	g.addSourceLocation(messageAST.Name.Span())
	reset()

	for i, field := range seq.All(ty.Members()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Field = append(mdp.Field, fd)
		g.field(field, fd, internal.MessageFieldsTag, int32(i))
	}

	var extnIndex int32
	for extend := range seq.Values(ty.Extends()) {
		reset := g.path.with(internal.MessageExtensionsTag)
		g.addSourceLocation(
			extend.AST().Span(),
			extend.AST().KeywordToken().ID(),
			extend.AST().Body().Braces().ID(),
		)
		reset()

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			mdp.Extension = append(mdp.Extension, fd)
			g.field(extn, fd, internal.MessageExtensionsTag, extnIndex)
			extnIndex++
		}
	}

	var enumIndex, nestedMsgIndex int32
	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			mdp.EnumType = append(mdp.EnumType, edp)
			g.enum(ty, edp, internal.MessageEnumsTag, enumIndex)
			enumIndex++
			continue
		}

		nested := new(descriptorpb.DescriptorProto)
		mdp.NestedType = append(mdp.NestedType, nested)
		g.message(ty, nested, internal.MessageNestedMessagesTag, nestedMsgIndex)
		nestedMsgIndex++
	}

	for i, extensions := range seq.All(ty.ExtensionRanges()) {
		er := new(descriptorpb.DescriptorProto_ExtensionRange)
		mdp.ExtensionRange = append(mdp.ExtensionRange, er)

		start, end := extensions.Range()
		er.Start = addr(start)
		er.End = addr(end + 1) // Exclusive.

		reset := g.path.with(internal.MessageExtensionRangesTag)
		g.addSourceLocation(
			extensions.DeclAST().Span(),
			extensions.DeclAST().KeywordToken().ID(),
			extensions.DeclAST().Semicolon().ID(),
		)
		reset()

		g.rangeSourceCodeInfo(
			extensions.AST(),
			internal.MessageExtensionRangesTag,
			internal.ExtensionRangeStartTag,
			internal.ExtensionRangeEndTag,
			int32(i),
		)

		if options := extensions.Options(); !iterx.Empty(options.Fields()) {
			reset := g.path.with(internal.ExtensionRangeOptionsTag)
			g.addSourceLocation(extensions.DeclAST().Options().Span())
			reset()

			er.Options = new(descriptorpb.ExtensionRangeOptions)
			g.options(options, er.Options, internal.ExtensionRangeOptionsTag)
		}
	}

	var topLevelSourceLocation bool
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !topLevelSourceLocation {
			reset := g.path.with(internal.MessageReservedRangesTag)
			g.addSourceLocation(
				reserved.DeclAST().Span(),
				reserved.DeclAST().KeywordToken().ID(),
				reserved.DeclAST().Semicolon().ID(),
			)
			reset()
			topLevelSourceLocation = true
		}

		rr := new(descriptorpb.DescriptorProto_ReservedRange)
		mdp.ReservedRange = append(mdp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end + 1) // Exclusive.

		g.rangeSourceCodeInfo(
			reserved.AST(),
			internal.MessageReservedRangesTag,
			internal.ReservedRangeStartTag,
			internal.ReservedRangeEndTag,
			int32(i),
		)
	}

	topLevelSourceLocation = false
	for i, name := range seq.All(ty.ReservedNames()) {
		if !topLevelSourceLocation {
			reset := g.path.with(internal.MessageReservedNamesTag)
			g.addSourceLocation(
				name.DeclAST().Span(),
				name.DeclAST().KeywordToken().ID(),
				name.DeclAST().Semicolon().ID(),
			)
			reset()
			topLevelSourceLocation = true
		}

		mdp.ReservedName = append(mdp.ReservedName, name.Name())
		reset := g.path.with(internal.MessageReservedNamesTag, int32(i))
		g.addSourceLocation(name.AST().Span())
		reset()
	}

	for i, oneof := range seq.All(ty.Oneofs()) {
		odp := new(descriptorpb.OneofDescriptorProto)
		mdp.OneofDecl = append(mdp.OneofDecl, odp)
		g.oneof(oneof, odp, internal.MessageOneofsTag, int32(i))
	}

	if g.currentFile.Syntax() == syntax.Proto3 {
		var names ir.SyntheticNames

		// Only now that we have added all of the normal oneofs do we add the
		// synthetic oneofs.
		for i, field := range seq.All(ty.Members()) {
			if field.Presence() != presence.Explicit ||
				!field.Oneof().IsZero() {
				continue
			}

			fdp := mdp.Field[i]
			fdp.Proto3Optional = addr(true)
			fdp.OneofIndex = addr(int32(len(mdp.OneofDecl)))
			mdp.OneofDecl = append(mdp.OneofDecl, &descriptorpb.OneofDescriptorProto{
				Name: addr(names.Generate(field.Name(), ty)),
			})
		}
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		for option := range messageAST.Body.Options() {
			reset := g.path.with(internal.MessageOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}

		mdp.Options = new(descriptorpb.MessageOptions)
		g.options(options, mdp.Options, internal.MessageOptionsTag)
	}
}

var predeclaredToFDPType = []descriptorpb.FieldDescriptorProto_Type{
	predeclared.Int32:  descriptorpb.FieldDescriptorProto_TYPE_INT32,
	predeclared.Int64:  descriptorpb.FieldDescriptorProto_TYPE_INT64,
	predeclared.UInt32: descriptorpb.FieldDescriptorProto_TYPE_UINT32,
	predeclared.UInt64: descriptorpb.FieldDescriptorProto_TYPE_UINT64,
	predeclared.SInt32: descriptorpb.FieldDescriptorProto_TYPE_SINT32,
	predeclared.SInt64: descriptorpb.FieldDescriptorProto_TYPE_SINT64,

	predeclared.Fixed32:  descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
	predeclared.Fixed64:  descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
	predeclared.SFixed32: descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
	predeclared.SFixed64: descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,

	predeclared.Float32: descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
	predeclared.Float64: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,

	predeclared.Bool:   descriptorpb.FieldDescriptorProto_TYPE_BOOL,
	predeclared.String: descriptorpb.FieldDescriptorProto_TYPE_STRING,
	predeclared.Bytes:  descriptorpb.FieldDescriptorProto_TYPE_BYTES,
}

func (g *generator) field(f ir.Member, fdp *descriptorpb.FieldDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	fieldAST := f.AST().AsField()
	g.addSourceLocation(fieldAST.Span(), token.ID(fieldAST.Type.ID()), fieldAST.Semicolon.ID())

	fdp.Name = addr(f.Name())
	reset := g.path.with(internal.FieldNameTag)
	g.addSourceLocation(fieldAST.Name.Span())
	reset()

	fdp.Number = addr(f.Number())
	reset = g.path.with(internal.FieldNumberTag)
	g.addSourceLocation(fieldAST.Tag.Span())
	reset()

	switch f.Presence() {
	case presence.Explicit, presence.Implicit, presence.Shared:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	case presence.Repeated:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	case presence.Required:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()
	}

	// Note: for specifically protobuf fields, we expect a single prefix. The protocompile
	// AST allows for arbitrary nesting of prefixes, so the API returns an iterator, but
	// [descriptorpb.FieldDescriptorProto] expects a single label.
	for prefix := range fieldAST.Type.Prefixes() {
		reset := g.path.with(internal.FieldLabelTag)
		g.addSourceLocation(prefix.PrefixToken().Span())
		reset()
	}

	fieldTypeSourcePathElement := internal.FieldTypeNameTag
	if ty := f.Element(); !ty.IsZero() {
		if kind, _ := slicesx.Get(predeclaredToFDPType, ty.Predeclared()); kind != 0 {
			fdp.Type = kind.Enum()
			fieldTypeSourcePathElement = internal.FieldTypeTag
		} else {
			fdp.TypeName = addr(string(ty.FullName().ToAbsolute()))
			switch {
			case ty.IsEnum():
				fdp.Type = descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum()
			case f.IsGroup():
				fdp.Type = descriptorpb.FieldDescriptorProto_TYPE_GROUP.Enum()
			default:
				fdp.Type = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
			}
		}
	}
	reset = g.path.with(int32(fieldTypeSourcePathElement))
	g.addSourceLocation(fieldAST.Type.RemovePrefixes().Span())
	reset()

	if f.IsExtension() && f.Container().FullName() != "" {
		fdp.Extendee = addr(string(f.Container().FullName().ToAbsolute()))
		reset := g.path.with(internal.FieldExtendeeTag)
		g.addSourceLocation(f.Extend().AST().Name().Span())
		reset()
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		reset := g.path.with(internal.FieldOptionsTag)
		g.addSourceLocation(fieldAST.Options.Span())
		reset()

		fdp.Options = new(descriptorpb.FieldOptions)
		g.options(options, fdp.Options, internal.FieldOptionsTag)
	}

	fdp.JsonName = addr(f.JSONName())

	d := f.PseudoOptions().Default
	if !d.IsZero() {
		if v, ok := d.AsBool(); ok {
			fdp.DefaultValue = addr(strconv.FormatBool(v))
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
			fdp.DefaultValue = addr(v)
		}
	}
}

func (g *generator) oneof(o ir.Oneof, odp *descriptorpb.OneofDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	oneofAST := o.AST().AsOneof()
	g.addSourceLocation(oneofAST.Span(), oneofAST.Keyword.ID(), oneofAST.Body.Braces().ID())

	odp.Name = addr(o.Name())
	reset := g.path.with(internal.OneofNameTag)
	g.addSourceLocation(oneofAST.Name.Span())
	reset()

	if options := o.Options(); !iterx.Empty(options.Fields()) {
		for option := range oneofAST.Body.Options() {
			reset := g.path.with(internal.OneofOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}

		odp.Options = new(descriptorpb.OneofOptions)
		g.options(options, odp.Options, internal.OneofOptionsTag)
	}
}

func (g *generator) enum(ty ir.Type, edp *descriptorpb.EnumDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	enumAST := ty.AST().AsEnum()
	g.addSourceLocation(enumAST.Span(), enumAST.Keyword.ID(), enumAST.Body.Braces().ID())

	edp.Name = addr(ty.Name())
	reset := g.path.with(internal.EnumNameTag)
	g.addSourceLocation(enumAST.Name.Span())
	reset()

	for i, enumValue := range seq.All(ty.Members()) {
		evd := new(descriptorpb.EnumValueDescriptorProto)
		edp.Value = append(edp.Value, evd)
		g.enumValue(enumValue, evd, internal.EnumValuesTag, int32(i))
	}

	var topLevelSourceLocation bool
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !topLevelSourceLocation {
			reset := g.path.with(internal.EnumReservedRangesTag)
			g.addSourceLocation(
				reserved.DeclAST().Span(),
				reserved.DeclAST().KeywordToken().ID(),
				reserved.DeclAST().Semicolon().ID(),
			)
			reset()
			topLevelSourceLocation = true
		}

		rr := new(descriptorpb.EnumDescriptorProto_EnumReservedRange)
		edp.ReservedRange = append(edp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end) // Inclusive, not exclusive like the one for messages!

		g.rangeSourceCodeInfo(
			reserved.AST(),
			internal.EnumReservedRangesTag,
			internal.ReservedRangeStartTag,
			internal.ReservedRangeEndTag,
			int32(i),
		)
	}

	topLevelSourceLocation = false
	for i, name := range seq.All(ty.ReservedNames()) {
		if !topLevelSourceLocation {
			reset := g.path.with(internal.EnumReservedNamesTag)
			g.addSourceLocation(
				name.DeclAST().Span(),
				name.DeclAST().KeywordToken().ID(),
				name.DeclAST().Semicolon().ID(),
			)
			reset()
			topLevelSourceLocation = true
		}

		edp.ReservedName = append(edp.ReservedName, name.Name())
		reset := g.path.with(internal.EnumReservedNamesTag, int32(i))
		g.addSourceLocation(name.AST().Span())
		reset()
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		for option := range enumAST.Body.Options() {
			reset := g.path.with(internal.EnumOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}

		edp.Options = new(descriptorpb.EnumOptions)
		g.options(options, edp.Options, internal.EnumOptionsTag)
	}
}

func (g *generator) enumValue(f ir.Member, evdp *descriptorpb.EnumValueDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	enumValueAST := f.AST().AsEnumValue()
	g.addSourceLocation(enumValueAST.Span(), enumValueAST.Name.ID(), enumValueAST.Semicolon.ID())

	evdp.Name = addr(f.Name())
	reset := g.path.with(internal.EnumValNameTag)
	g.addSourceLocation(enumValueAST.Name.Span())
	reset()

	evdp.Number = addr(f.Number())
	reset = g.path.with(internal.EnumValNumberTag)
	g.addSourceLocation(enumValueAST.Tag.Span())
	reset()

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		reset := g.path.with(internal.EnumValOptionsTag)
		g.addSourceLocation(enumValueAST.Options.Span())
		reset()

		evdp.Options = new(descriptorpb.EnumValueOptions)
		g.options(options, evdp.Options, internal.EnumValOptionsTag)
	}
}

func (g *generator) service(s ir.Service, sdp *descriptorpb.ServiceDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	serviceAST := s.AST().AsService()
	g.addSourceLocation(serviceAST.Span(), serviceAST.Keyword.ID(), serviceAST.Body.Braces().ID())

	sdp.Name = addr(s.Name())
	reset := g.path.with(internal.ServiceNameTag)
	g.addSourceLocation(serviceAST.Name.Span())
	reset()

	for i, method := range seq.All(s.Methods()) {
		mdp := new(descriptorpb.MethodDescriptorProto)
		sdp.Method = append(sdp.Method, mdp)
		g.method(method, mdp, internal.ServiceMethodsTag, int32(i))
	}

	if options := s.Options(); !iterx.Empty(options.Fields()) {
		sdp.Options = new(descriptorpb.ServiceOptions)
		for option := range serviceAST.Body.Options() {
			reset := g.path.with(internal.ServiceOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}
		g.options(options, sdp.Options, internal.ServiceOptionsTag)
	}
}

func (g *generator) method(m ir.Method, mdp *descriptorpb.MethodDescriptorProto, sourcePath ...int32) {
	topLevelReset := g.path.with(sourcePath...)
	defer topLevelReset()

	methodAST := m.AST().AsMethod()

	// Comment attribution for tokens is unique. The behavior in protoc for method leading
	// comments is as follows for methods without a body:
	//
	// service FooService {
	//   // I'm the leading comment for GetFoo
	//   rpc GetFoo (GetFooRequest) returns (GetFooResponse); // I'm the trailing comment for GetFoo
	// }
	//
	// And for methods with a body:
	//
	// service FooService {
	//   // I'm still the leading comment for GetFoo
	//   rpc GetFoo (GetFooRequest) returns (GetFooResponse) { // I'm the trailing comment for GetFoo
	//   }; // I am NOT the trailing comment for GetFoo, and am instead dropped.
	// }
	//
	closingToken := m.AST().Semicolon().ID()
	if !methodAST.Body.Braces().IsZero() {
		closingToken = methodAST.Body.Braces().ID()
	}

	g.addSourceLocation(methodAST.Span(), methodAST.Keyword.ID(), closingToken)

	mdp.Name = addr(m.Name())
	reset := g.path.with(internal.MethodNameTag)
	g.addSourceLocation(methodAST.Name.Span())
	reset()

	in, inStream := m.Input()
	mdp.InputType = addr(string(in.FullName()))
	mdp.ClientStreaming = addr(inStream)

	// Methods only have a single input, see [descriptorpb.MethodDescriptorProto].
	inputAST := methodAST.Signature.Inputs().At(0)
	if prefixed := inputAST.AsPrefixed(); !prefixed.IsZero() {
		reset := g.path.with(internal.MethodInputStreamTag)
		g.addSourceLocation(prefixed.PrefixToken().Span())
		reset()
	}
	reset = g.path.with(internal.MethodInputTag)
	g.addSourceLocation(inputAST.RemovePrefixes().Span())
	reset()

	out, outStream := m.Output()
	mdp.OutputType = addr(string(out.FullName()))
	mdp.ServerStreaming = addr(outStream)

	// Methods only have a single output, see [descriptorpb.MethodDescriptorProto].
	outputAST := methodAST.Signature.Outputs().At(0)
	if prefixed := outputAST.AsPrefixed(); !prefixed.IsZero() {
		reset := g.path.with(internal.MethodOutputStreamTag)
		g.addSourceLocation(prefixed.PrefixToken().Span())
		reset()
	}
	reset = g.path.with(internal.MethodOutputTag)
	g.addSourceLocation(outputAST.RemovePrefixes().Span())
	reset()

	if options := m.Options(); !iterx.Empty(options.Fields()) {
		mdp.Options = new(descriptorpb.MethodOptions)
		for option := range methodAST.Body.Options() {
			reset := g.path.with(internal.MethodOptionsTag)
			g.addSourceLocation(option.Span())
			reset()
		}
		g.options(options, mdp.Options, internal.MethodOptionsTag)
	}
}

func (g *generator) options(v ir.MessageValue, target proto.Message, sourcePathElement int32) {
	target.ProtoReflect().SetUnknown(v.Marshal(nil, nil))
	g.messageValueSourceCodeInfo(v, sourcePathElement)
}

func (g *generator) messageValueSourceCodeInfo(v ir.MessageValue, sourcePath ...int32) {
	for field := range v.Fields() {
		var optionSpanIndex int32
		for optionSpan := range seq.Values(field.OptionSpans()) {
			if optionSpan == nil {
				continue
			}

			if messageField := field.AsMessage(); !messageField.IsZero() {
				g.messageValueSourceCodeInfo(messageField, append(sourcePath, field.Field().Number())...)
				continue
			}

			span := optionSpan.Span()
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
			//
			var checkCommentTokens []token.ID
			keyword, semicolon := g.optionKeywordAndSemicolon(span)
			if !keyword.IsZero() && !semicolon.IsZero() {
				checkCommentTokens = []token.ID{keyword.ID(), semicolon.ID()}
				span = source.Between(keyword.Span(), semicolon.Span())
			}

			if field.Field().IsRepeated() {
				reset := g.path.with(append(sourcePath, field.Field().Number(), optionSpanIndex)...)
				g.addSourceLocation(span, checkCommentTokens...)
				reset()
				optionSpanIndex++
			} else {
				reset := g.path.with(append(sourcePath, field.Field().Number())...)
				g.addSourceLocation(span, checkCommentTokens...)
				reset()
			}
		}
	}
}

// optionKeywordAndSemicolon is a helper function that checks the non-skippable tokens
// before and after the given span. If the non-skippable token before is the option keyword
// and the non-skippable token after is the semicolon, then both are returned.
func (g *generator) optionKeywordAndSemicolon(optionSpan source.Span) (token.Token, token.Token) {
	_, start := g.currentFile.AST().Stream().Around(optionSpan.Start)
	before := token.NewCursorAt(start)
	prev := before.Prev()
	if prev.Keyword() != keyword.Option {
		return token.Zero, token.Zero
	}
	_, end := g.currentFile.AST().Stream().Around(optionSpan.End)
	after := token.NewCursorAt(end)
	next := after.Next()
	if next.Keyword() != keyword.Semi {
		return token.Zero, token.Zero
	}
	return prev, next
}

func (g *generator) rangeSourceCodeInfo(rangeAST ast.ExprAny, baseTag, startTag, endTag, index int32) {
	reset := g.path.with(baseTag, index)
	defer reset()
	g.addSourceLocation(rangeAST.Span())

	var startSpan, endSpan source.Span
	switch rangeAST.Kind() {
	case ast.ExprKindLiteral, ast.ExprKindPath:
		startSpan = rangeAST.Span()
		endSpan = rangeAST.Span()
	case ast.ExprKindRange:
		start, end := rangeAST.AsRange().Bounds()
		startSpan = start.Span()
		endSpan = end.Span()
	}

	if startTag != 0 {
		reset := g.path.with(startTag)
		g.addSourceLocation(startSpan)
		reset()
	}

	if endTag != 0 {
		reset := g.path.with(endTag)
		g.addSourceLocation(endSpan)
		reset()
	}
}

// addSourceLocation adds the source code info location based on the current path tracked
// by the [generator]. It also checks the given token IDs for comments.
func (g *generator) addSourceLocation(span source.Span, checkForComments ...token.ID) {
	if g.sourceCodeInfo == nil || span.IsZero() {
		return
	}

	location := new(descriptorpb.SourceCodeInfo_Location)
	g.sourceCodeInfo.Location = append(g.sourceCodeInfo.Location, location)

	location.Span = locationSpan(span)
	location.Path = g.path.clone()

	// Comments are merged across the provided [token.ID]s.
	for _, id := range checkForComments {
		comments, ok := g.commentTracker.attributed[id]
		if !ok {
			continue
		}
		if leadingComment := comments.leadingComment(); leadingComment != "" {
			location.LeadingComments = addr(leadingComment)
		}
		if trailingComment := comments.trailingComment(); trailingComment != "" {
			location.TrailingComments = addr(trailingComment)
		}
		if detachedComments := comments.detachedComments(); len(detachedComments) > 0 {
			location.LeadingDetachedComments = detachedComments
		}
	}
}

// addr is a helper for creating a pointer out of any type, because Go is
// missing the syntax &"foo", etc.
func addr[T any](v T) *T { return &v }

// locationSpan is a helper function for returning the [descriptorpb.SourceCodeInfo_Location]
// span for the given [source.Span].
//
// The span for [descriptorpb.SourceCodeInfo_Location] always has exactly three or four:
// start line, start column, end line (optional, otherwise assumed same as start line),
// and end column. The line and column numbers are zero-based.
func locationSpan(span source.Span) []int32 {
	start, end := span.StartLoc(), span.EndLoc()
	if start.Line == end.Line {
		return []int32{
			int32(start.Line) - 1,
			int32(start.Column) - 1,
			int32(end.Column) - 1,
		}
	}
	return []int32{
		int32(start.Line) - 1,
		int32(start.Column) - 1,
		int32(end.Line) - 1,
		int32(end.Column) - 1,
	}
}
