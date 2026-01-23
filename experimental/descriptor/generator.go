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

package descriptor

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	descriptorv1 "buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go/buf/descriptor/v1"
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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
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
	path := new(path)
	g.path = path
	fdp.Name = addr(file.Path())

	if g.includeDebugInfo {
		g.sourceCodeInfo = new(descriptorpb.SourceCodeInfo)

		ct := new(commentTracker)
		g.commentTracker = ct
		ct.attributeComments(g.currentFile.AST().Stream().Cursor())

		fdp.SourceCodeInfo = g.sourceCodeInfo

		g.sourceCodeInfoExtn = new(descriptorv1.SourceCodeInfoExtension)
		proto.SetExtension(g.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension, g.sourceCodeInfoExtn)
	}

	fdp.Package = addr(string(file.Package()))
	g.addSourceLocation(
		file.AST().Package().Span(),
		[]int32{internal.FilePackageTag},
		[]token.ID{
			file.AST().Package().KeywordToken().ID(),
			file.AST().Package().Semicolon().ID(),
		},
	)

	if file.Syntax().IsEdition() {
		fdp.Syntax = addr("editions")
		fdp.Edition = descriptorpb.Edition(file.Syntax()).Enum()
	} else {
		fdp.Syntax = addr(file.Syntax().String())
	}
	g.addSourceLocation(
		file.AST().Syntax().Span(),
		// According to descriptor.proto and protoc behavior, the path is always set to [12] for
		// both syntax and editions.
		[]int32{internal.FileSyntaxTag},
		[]token.ID{
			file.AST().Syntax().KeywordToken().ID(),
			file.AST().Syntax().Semicolon().ID(),
		},
	)

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
		g.addSourceLocation(
			imp.Decl.Span(),
			[]int32{internal.FileDependencyTag, int32(i)},
			[]token.ID{
				imp.Decl.KeywordToken().ID(),
				imp.Decl.Semicolon().ID(),
			},
		)

		if imp.Public {
			fdp.PublicDependency = append(fdp.PublicDependency, int32(i))
			_, public := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Public
			})
			g.addSourceLocation(
				public.Span(),
				[]int32{internal.FilePublicDependencyTag, publicDepIndex},
				nil, // Comments are only attached to the top-level import source code info
			)
			publicDepIndex++
		}
		if imp.Weak {
			fdp.WeakDependency = append(fdp.WeakDependency, int32(i))
			_, weak := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Weak
			})
			g.addSourceLocation(
				weak.Span(),
				[]int32{internal.FileWeakDependencyTag, weakDepIndex},
				nil, // Comments are only attached to the top-level import source code info
			)
			weakDepIndex++
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
			g.enum(ty, edp, []int32{internal.FileEnumsTag, enumIndex})
			enumIndex++
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		fdp.MessageType = append(fdp.MessageType, mdp)
		g.message(ty, mdp, []int32{internal.FileMessagesTag, msgIndex})
		msgIndex++
	}

	for i, service := range seq.All(file.Services()) {
		sdp := new(descriptorpb.ServiceDescriptorProto)
		fdp.Service = append(fdp.Service, sdp)
		g.service(service, sdp, []int32{internal.FileServicesTag, int32(i)})
	}

	var extnIndex int32
	for extend := range seq.Values(file.Extends()) {
		g.addSourceLocation(
			extend.AST().Span(),
			[]int32{internal.FileExtensionsTag},
			[]token.ID{
				extend.AST().KeywordToken().ID(),
				extend.AST().Body().Braces().ID(),
			},
		)

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			fdp.Extension = append(fdp.Extension, fd)
			g.field(extn, fd, []int32{internal.FileExtensionsTag, extnIndex})
			extnIndex++
		}
	}

	if options := file.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FileOptions)
		for option := range file.AST().Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.FileOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, fdp.Options, []int32{internal.FileOptionsTag})
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

func (g *generator) message(ty ir.Type, mdp *descriptorpb.DescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	messageAST := ty.AST().AsMessage()
	g.addSourceLocation(
		messageAST.Span(),
		sourcePath,
		[]token.ID{
			messageAST.Keyword.ID(),
			messageAST.Body.Braces().ID(),
		},
	)

	mdp.Name = addr(ty.Name())
	g.addSourceLocation(
		messageAST.Name.Span(),
		[]int32{internal.MessageNameTag},
		nil, // Comments are only attached to the top-level message source code info
	)

	for i, field := range seq.All(ty.Members()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Field = append(mdp.Field, fd)
		g.field(field, fd, []int32{internal.MessageFieldsTag, int32(i)})
	}

	var extnIndex int32
	for extend := range seq.Values(ty.Extends()) {
		g.addSourceLocation(
			extend.AST().Span(),
			[]int32{internal.MessageExtensionsTag},
			[]token.ID{
				extend.AST().KeywordToken().ID(),
				extend.AST().Body().Braces().ID(),
			},
		)

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			mdp.Extension = append(mdp.Extension, fd)
			g.field(extn, fd, []int32{internal.MessageExtensionsTag, extnIndex})
			extnIndex++
		}
	}

	var enumIndex, nestedMsgIndex int32
	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			mdp.EnumType = append(mdp.EnumType, edp)
			g.enum(ty, edp, []int32{internal.MessageEnumsTag, enumIndex})
			enumIndex++
			continue
		}

		nested := new(descriptorpb.DescriptorProto)
		mdp.NestedType = append(mdp.NestedType, nested)
		g.message(ty, nested, []int32{internal.MessageNestedMessagesTag, nestedMsgIndex})
		nestedMsgIndex++
	}

	for i, extensions := range seq.All(ty.ExtensionRanges()) {
		er := new(descriptorpb.DescriptorProto_ExtensionRange)
		mdp.ExtensionRange = append(mdp.ExtensionRange, er)

		start, end := extensions.Range()
		er.Start = addr(start)
		er.End = addr(end + 1) // Exclusive.

		g.addSourceLocation(
			extensions.DeclAST().Span(),
			[]int32{internal.MessageExtensionRangesTag},
			[]token.ID{
				extensions.DeclAST().KeywordToken().ID(),
				extensions.DeclAST().Semicolon().ID(),
			},
		)

		g.rangeSourceCodeInfo(
			extensions.AST(),
			internal.MessageExtensionRangesTag,
			internal.ExtensionRangeStartTag,
			internal.ExtensionRangeEndTag,
			int32(i),
		)

		if options := extensions.Options(); !iterx.Empty(options.Fields()) {
			er.Options = new(descriptorpb.ExtensionRangeOptions)
			g.addSourceLocation(
				extensions.DeclAST().Options().Span(),
				[]int32{internal.ExtensionRangeOptionsTag},
				nil, // Comments are attached to individual option spans
			)
			g.options(options, er.Options, []int32{internal.ExtensionRangeOptionsTag})
		}
	}

	var topLevelSourceLocation bool
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !topLevelSourceLocation {
			g.addSourceLocation(
				reserved.DeclAST().Span(),
				[]int32{internal.MessageReservedRangesTag},
				[]token.ID{
					reserved.DeclAST().KeywordToken().ID(),
					reserved.DeclAST().Semicolon().ID(),
				},
			)
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
			g.addSourceLocation(
				name.DeclAST().Span(),
				[]int32{internal.MessageReservedNamesTag},
				[]token.ID{
					name.DeclAST().KeywordToken().ID(),
					name.DeclAST().Semicolon().ID(),
				},
			)
			topLevelSourceLocation = true
		}

		mdp.ReservedName = append(mdp.ReservedName, name.Name())
		g.addSourceLocation(
			name.AST().Span(),
			[]int32{internal.MessageReservedNamesTag, int32(i)},
			nil, // Comments are only attached to the top-level reserved names source code info
		)
	}

	for i, oneof := range seq.All(ty.Oneofs()) {
		odp := new(descriptorpb.OneofDescriptorProto)
		mdp.OneofDecl = append(mdp.OneofDecl, odp)
		g.oneof(oneof, odp, []int32{internal.MessageOneofsTag, int32(i)})
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
		mdp.Options = new(descriptorpb.MessageOptions)
		for option := range messageAST.Body.Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.MessageOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, mdp.Options, []int32{internal.MessageOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
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

func (g *generator) field(f ir.Member, fdp *descriptorpb.FieldDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	fieldAST := f.AST().AsField()
	g.addSourceLocation(
		fieldAST.Span(),
		sourcePath,
		[]token.ID{
			token.ID(fieldAST.Type.ID()),
			fieldAST.Semicolon.ID(),
		},
	)

	fdp.Name = addr(f.Name())
	g.addSourceLocation(
		fieldAST.Name.Span(),
		[]int32{internal.FieldNameTag},
		nil, // Comments are only attached to the top-level field source code info
	)

	fdp.Number = addr(f.Number())
	g.addSourceLocation(
		fieldAST.Tag.Span(),
		[]int32{internal.FieldNumberTag},
		nil, // Comments are only attached to the top-level field source code info
	)

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
		g.addSourceLocation(
			prefix.PrefixToken().Span(),
			[]int32{internal.FieldLabelTag},
			nil, // Comments are only attached to the top-level field source code info
		)
	}

	fieldTypeSourcePath := []int32{internal.FieldTypeNameTag}
	if ty := f.Element(); !ty.IsZero() {
		if kind, _ := slicesx.Get(predeclaredToFDPType, ty.Predeclared()); kind != 0 {
			fdp.Type = kind.Enum()
			fieldTypeSourcePath = []int32{internal.FieldTypeTag}
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
	g.addSourceLocation(
		fieldAST.Type.RemovePrefixes().Span(),
		fieldTypeSourcePath,
		nil, // Comments are only attached to the top-level field source code info
	)

	if f.IsExtension() && f.Container().FullName() != "" {
		fdp.Extendee = addr(string(f.Container().FullName().ToAbsolute()))
		g.addSourceLocation(
			f.Extend().AST().Name().Span(),
			[]int32{internal.FieldExtendeeTag},
			nil, // Comments are only attached to the top-level field source code info
		)
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FieldOptions)
		g.addSourceLocation(
			fieldAST.Options.Span(),
			[]int32{internal.FieldOptionsTag},
			nil, // Comments are not attached from compact options
		)
		g.options(options, fdp.Options, []int32{internal.FieldOptionsTag})
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

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) oneof(o ir.Oneof, odp *descriptorpb.OneofDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	oneofAST := o.AST().AsOneof()
	g.addSourceLocation(
		oneofAST.Span(),
		sourcePath,
		[]token.ID{
			oneofAST.Keyword.ID(),
			oneofAST.Body.Braces().ID(),
		},
	)

	odp.Name = addr(o.Name())
	g.addSourceLocation(
		oneofAST.Name.Span(),
		[]int32{internal.OneofNameTag},
		nil, // Comments are only attached to the top-level oneof source code info
	)

	if options := o.Options(); !iterx.Empty(options.Fields()) {
		odp.Options = new(descriptorpb.OneofOptions)
		for option := range oneofAST.Body.Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.OneofOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, odp.Options, []int32{internal.OneofOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) enum(ty ir.Type, edp *descriptorpb.EnumDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	enumAST := ty.AST().AsEnum()
	g.addSourceLocation(
		enumAST.Span(),
		sourcePath,
		[]token.ID{
			enumAST.Keyword.ID(),
			enumAST.Body.Braces().ID(),
		},
	)

	edp.Name = addr(ty.Name())
	g.addSourceLocation(
		enumAST.Name.Span(),
		[]int32{internal.EnumNameTag},
		nil, // Comments are only attached to the top-level enum source code info
	)

	for i, enumValue := range seq.All(ty.Members()) {
		evd := new(descriptorpb.EnumValueDescriptorProto)
		edp.Value = append(edp.Value, evd)
		g.enumValue(enumValue, evd, []int32{internal.EnumValuesTag, int32(i)})
	}

	var topLevelSourceLocation bool
	for i, reserved := range seq.All(ty.ReservedRanges()) {
		if !topLevelSourceLocation {
			g.addSourceLocation(
				reserved.DeclAST().Span(),
				[]int32{internal.EnumReservedRangesTag},
				[]token.ID{
					reserved.DeclAST().KeywordToken().ID(),
					reserved.DeclAST().Semicolon().ID(),
				},
			)
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
			g.addSourceLocation(
				name.DeclAST().Span(),
				[]int32{internal.MessageReservedNamesTag},
				[]token.ID{
					name.DeclAST().KeywordToken().ID(),
					name.DeclAST().Semicolon().ID(),
				},
			)
			topLevelSourceLocation = true
		}

		edp.ReservedName = append(edp.ReservedName, name.Name())
		g.addSourceLocation(
			name.AST().Span(),
			[]int32{internal.EnumReservedNamesTag, int32(i)},
			nil, // Comments are only attached to the top-level reserved names source code info
		)
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		edp.Options = new(descriptorpb.EnumOptions)
		for option := range enumAST.Body.Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.EnumOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, edp.Options, []int32{internal.EnumOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) enumValue(f ir.Member, evdp *descriptorpb.EnumValueDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	enumValueAST := f.AST().AsEnumValue()
	g.addSourceLocation(
		enumValueAST.Span(),
		sourcePath,
		[]token.ID{
			enumValueAST.Name.ID(),
			enumValueAST.Semicolon.ID(),
		},
	)

	evdp.Name = addr(f.Name())
	g.addSourceLocation(
		enumValueAST.Name.Span(),
		[]int32{internal.EnumValNameTag},
		nil, // Comments are only attached to the top-level enum value source code info
	)
	evdp.Number = addr(f.Number())
	g.addSourceLocation(
		enumValueAST.Tag.Span(),
		[]int32{internal.EnumValNumberTag},
		nil, // Comments are only attached to the top-level enum value source code info
	)

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		evdp.Options = new(descriptorpb.EnumValueOptions)
		g.addSourceLocation(
			enumValueAST.Options.Span(),
			[]int32{internal.EnumValOptionsTag},
			nil, // Comments are not attached from compact options
		)

		g.options(options, evdp.Options, []int32{internal.EnumValOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) service(s ir.Service, sdp *descriptorpb.ServiceDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	serviceAST := s.AST().AsService()
	g.addSourceLocation(
		serviceAST.Span(),
		sourcePath,
		[]token.ID{
			serviceAST.Keyword.ID(),
			serviceAST.Body.Braces().ID(),
		},
	)

	sdp.Name = addr(s.Name())
	g.addSourceLocation(
		serviceAST.Name.Span(),
		[]int32{internal.ServiceNameTag},
		nil, // Comments are only attached to the top-level service source code info
	)

	for i, method := range seq.All(s.Methods()) {
		mdp := new(descriptorpb.MethodDescriptorProto)
		sdp.Method = append(sdp.Method, mdp)
		g.method(method, mdp, []int32{internal.ServiceMethodsTag, int32(i)})
	}

	if options := s.Options(); !iterx.Empty(options.Fields()) {
		sdp.Options = new(descriptorpb.ServiceOptions)
		for option := range serviceAST.Body.Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.ServiceOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, sdp.Options, []int32{internal.ServiceOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) method(m ir.Method, mdp *descriptorpb.MethodDescriptorProto, sourcePath []int32) {
	g.path.descend(len(sourcePath))

	methodAST := m.AST().AsMethod()
	g.addSourceLocation(
		methodAST.Span(),
		sourcePath,
		[]token.ID{
			methodAST.Keyword.ID(),
			methodAST.Body.Braces().ID(),
		},
	)

	mdp.Name = addr(m.Name())
	g.addSourceLocation(
		methodAST.Name.Span(),
		[]int32{internal.MethodNameTag},
		nil, // Comments are only attached to the top-level method source code info
	)

	in, inStream := m.Input()
	mdp.InputType = addr(string(in.FullName()))
	mdp.ClientStreaming = addr(inStream)

	// Methods only have a single input, see [descriptorpb.MethodDescriptorProto].
	inputAST := methodAST.Signature.Inputs().At(0)
	if prefixed := inputAST.AsPrefixed(); !prefixed.IsZero() {
		g.addSourceLocation(
			prefixed.PrefixToken().Span(),
			[]int32{internal.MethodInputStreamTag},
			nil, // Comments are only attached to the top-level method source code info
		)
	}
	g.addSourceLocation(
		inputAST.RemovePrefixes().Span(),
		[]int32{internal.MethodInputTag},
		nil, // Comments are only attached to the top-level method source code info
	)

	out, outStream := m.Output()
	mdp.OutputType = addr(string(out.FullName()))
	mdp.ServerStreaming = addr(outStream)

	// Methods only have a single output, see [descriptorpb.MethodDescriptorProto].
	outputAST := methodAST.Signature.Outputs().At(0)
	if prefixed := outputAST.AsPrefixed(); !prefixed.IsZero() {
		g.addSourceLocation(
			prefixed.PrefixToken().Span(),
			[]int32{internal.MethodOutputStreamTag},
			nil, // Comments are only attached to the top-level method source code info
		)
	}
	g.addSourceLocation(
		outputAST.RemovePrefixes().Span(),
		[]int32{internal.MethodOutputTag},
		nil, // Comments are only attached to the top-level method source code info
	)

	if options := m.Options(); !iterx.Empty(options.Fields()) {
		mdp.Options = new(descriptorpb.MethodOptions)
		for option := range methodAST.Body.Options() {
			g.addSourceLocation(
				option.Span(),
				[]int32{internal.MethodOptionsTag},
				nil, // Comments are attached to individual option spans
			)
		}
		g.options(options, mdp.Options, []int32{internal.MethodOptionsTag})
	}

	g.path.ascend(len(sourcePath))
	g.path.resetPath()
}

func (g *generator) options(v ir.MessageValue, target proto.Message, sourcePath []int32) {
	target.ProtoReflect().SetUnknown(v.Marshal(nil, nil))
	g.messageValueSourceCodeInfo(v, sourcePath)
}

func (g *generator) messageValueSourceCodeInfo(v ir.MessageValue, sourcePath []int32) {
	for field := range v.Fields() {
		var optionSpanIndex int32
		for optionSpan := range seq.Values(field.OptionSpans()) {
			if optionSpan == nil {
				continue
			}

			if messageField := field.AsMessage(); !messageField.IsZero() {
				g.messageValueSourceCodeInfo(messageField, append(sourcePath, field.Field().Number()))
				continue
			}

			// HACK: For each optionSpan, check around it for the option keyword and semicolon.
			// If the non-skippable token directly before this optionSpan is the option keyword,
			// and the non-skippable token directly after the optionSpan is the semicolon, we
			// include both tokens as part of the span.
			// We also check the option keyword token and semicolon tokens for comments.
			span := optionSpan.Span()
			var checkCommentTokens []token.ID
			keyword, semicolon := g.optionKeywordAndSemicolon(span)
			if !keyword.IsZero() && !semicolon.IsZero() {
				checkCommentTokens = []token.ID{keyword.ID(), semicolon.ID()}
				span = source.Span{
					File:  span.File,
					Start: keyword.Span().Start,
					End:   semicolon.Span().End,
				}
			}

			if field.Field().IsRepeated() {
				g.addSourceLocation(
					span,
					append(sourcePath, field.Field().Number(), optionSpanIndex),
					checkCommentTokens,
				)
				optionSpanIndex++
			} else {
				g.addSourceLocation(
					span,
					append(sourcePath, field.Field().Number()),
					checkCommentTokens,
				)
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
	g.addSourceLocation(
		rangeAST.Span(),
		[]int32{baseTag, index},
		nil, // Comments are only attached to the top-level declaration source code info
	)

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
		g.addSourceLocation(
			startSpan,
			[]int32{baseTag, index, startTag},
			nil, // Comments are only attached to the top-level declaration source code info
		)
	}

	if endTag != 0 {
		g.addSourceLocation(
			endSpan,
			[]int32{baseTag, index, endTag},
			nil, // Comments are only attached to the top-level declaration source code info
		)
	}
}

// addSourceLocation adds the source code info location based on the current path tracked
// by the [generator].
func (g *generator) addSourceLocation(
	span source.Span,
	pathElements []int32,
	checkForComments []token.ID,
) {
	if g.sourceCodeInfo != nil && !span.IsZero() {
		location := new(descriptorpb.SourceCodeInfo_Location)
		g.sourceCodeInfo.Location = append(g.sourceCodeInfo.Location, location)

		location.Span = locationSpan(span)
		g.path.appendElements(pathElements...)
		location.Path = internal.ClonePath(g.path.path)
		g.path.resetPath()

		stringify := func(tokens []token.Token) string {
			var str string
			for _, t := range tokens {
				text := t.Text()
				if t.Kind() == token.Comment {
					switch {
					case strings.HasPrefix(text, "//"):
						// For line comments, the leading "//" needs to be trimmed off.
						str += strings.TrimPrefix(text, "//")
					case strings.HasPrefix(text, "/*"):
						// For block comments, we iterate through each line and trim the leading "/*",
						// "*", and "*/".
						for _, line := range strings.SplitAfter(text, "\n") {
							switch {
							case strings.HasPrefix(line, "/*"):
								str += strings.TrimPrefix(line, "/*")
							case strings.HasPrefix(strings.TrimSpace(line), "*"):
								// We check the line with all spaces trimmed because of leading whitespace.
								str += strings.TrimPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "*")
							case strings.HasSuffix(line, "*/"):
								str += strings.TrimSuffix(line, "*/")
							}
						}
					}
				} else {
					str += text
				}
			}
			return str
		}

		// Comments are merged across the the provided [token.ID]s.
		for _, id := range checkForComments {
			comments, ok := g.commentTracker.donated[id]
			if ok {
				if leadingComments := stringify(comments.leading); leadingComments != "" {
					location.LeadingComments = addr(leadingComments)
				}
				if trailingComments := stringify(comments.trailing); trailingComments != "" {
					location.TrailingComments = addr(trailingComments)
				}
				for _, paragraph := range comments.detached {
					location.LeadingDetachedComments = append(location.LeadingDetachedComments, stringify(paragraph))
				}
			}
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
