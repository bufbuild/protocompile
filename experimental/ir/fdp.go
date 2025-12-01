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

package ir

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	descriptorv1 "buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go/buf/descriptor/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/id"
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

// DescriptorSetBytes generates a FileDescriptorSet for the given files, and returns the
// result as an encoded byte slice.
//
// The resulting FileDescriptorSet is always fully linked: it contains all dependencies except
// the WKTs, and all names are fully-qualified.
func DescriptorSetBytes(files []*File, options ...DescriptorOption) ([]byte, error) {
	var dg descGenerator
	for _, opt := range options {
		if opt != nil {
			opt(&dg)
		}
	}

	fds := new(descriptorpb.FileDescriptorSet)
	dg.files(files, fds)
	return proto.Marshal(fds)
}

// DescriptorProtoBytes generates a single FileDescriptorProto for file, and returns the
// result as an encoded byte slice.
//
// The resulting FileDescriptorProto is fully linked: all names are fully-qualified.
func DescriptorProtoBytes(file *File, options ...DescriptorOption) ([]byte, error) {
	var dg descGenerator
	for _, opt := range options {
		if opt != nil {
			opt(&dg)
		}
	}

	fdp := new(descriptorpb.FileDescriptorProto)
	dg.file(file, fdp)
	return proto.Marshal(fdp)
}

// DescriptorOption is an option to pass to [DescriptorSetBytes] or [DescriptorProtoBytes].
type DescriptorOption func(*descGenerator)

// IncludeDebugInfo sets whether or not to include google.protobuf.SourceCodeInfo in
// the output.
func IncludeSourceCodeInfo(flag bool) DescriptorOption {
	return func(dg *descGenerator) {
		dg.includeDebugInfo = flag
	}
}

// ExcludeFiles excludes the given files from the output of [DescriptorSetBytes].
func ExcludeFiles(exclude func(*File) bool) DescriptorOption {
	return func(dg *descGenerator) {
		dg.exclude = exclude
	}
}

type descGenerator struct {
	currentFile      *File
	includeDebugInfo bool
	exclude          func(*File) bool
	path             []int32

	sourceCodeInfo     *descriptorpb.SourceCodeInfo
	sourceCodeInfoExtn *descriptorv1.SourceCodeInfoExtension

	commentTracker *commentTracker
}

func (dg *descGenerator) files(files []*File, fds *descriptorpb.FileDescriptorSet) {
	// Build up all of the imported files. We can't just pull out the transitive
	// imports for each file because we want the result to be sorted
	// topologically.
	for file := range topoSort(files) {
		if dg.exclude != nil && dg.exclude(file) {
			continue
		}

		fdp := new(descriptorpb.FileDescriptorProto)
		fds.File = append(fds.File, fdp)

		dg.file(file, fdp)
	}
}

func (dg *descGenerator) file(file *File, fdp *descriptorpb.FileDescriptorProto) {
	dg.currentFile = file
	dg.path = nil
	if dg.includeDebugInfo {
		dg.sourceCodeInfo = new(descriptorpb.SourceCodeInfo)

		ct := new(commentTracker)
		dg.commentTracker = ct
		ct.attributeComments(dg.currentFile.AST().Stream().Cursor())

		fdp.SourceCodeInfo = dg.sourceCodeInfo

		dg.sourceCodeInfoExtn = new(descriptorv1.SourceCodeInfoExtension)
		proto.SetExtension(dg.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension, dg.sourceCodeInfoExtn)
	}

	fdp.Name = addr(file.Path())
	fdp.Package = addr(string(file.Package()))
	dg.path = []int32{internal.FilePackageTag}
	dg.addSourceLocation(
		file.AST().Package().Span(),
		file.AST().Package().KeywordToken().ID(),
		file.AST().Package().Semicolon().ID(),
	)
	dg.path = nil

	// According to descriptor.proto and protoc behavior, the path is always set to [12] for
	// both syntax and editions.
	dg.path = []int32{internal.FileSyntaxTag}
	if file.Syntax().IsEdition() {
		fdp.Syntax = addr("editions")
		fdp.Edition = descriptorpb.Edition(file.Syntax()).Enum()
	} else {
		fdp.Syntax = addr(file.Syntax().String())
	}
	dg.addSourceLocation(
		file.AST().Syntax().Span(),
		file.AST().Syntax().KeywordToken().ID(),
		file.AST().Syntax().Semicolon().ID(),
	)
	dg.path = nil

	if dg.sourceCodeInfoExtn != nil {
		dg.sourceCodeInfoExtn.IsSyntaxUnspecified = file.AST().Syntax().IsZero()
	}

	// Canonicalize import order so that it does not change whenever we refactor
	// internal structures.
	imports := seq.ToSlice(file.Imports())
	slices.SortFunc(imports, cmpx.Key(func(imp Import) int {
		return imp.Decl.KeywordToken().Span().Start
	}))
	var publicDepIndex, weakDepIndex int32
	for i, imp := range imports {
		fdp.Dependency = append(fdp.Dependency, imp.Path())
		dg.path = []int32{internal.FileDependencyTag, int32(i)}
		dg.addSourceLocation(
			imp.Decl.Span(),
			imp.Decl.KeywordToken().ID(),
			imp.Decl.Semicolon().ID(),
		)

		if imp.Public {
			fdp.PublicDependency = append(fdp.PublicDependency, int32(i))
			dg.path = []int32{internal.FilePublicDependencyTag, publicDepIndex}
			_, public := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Public
			})
			// Comments are only attached to the top-level import source code info
			dg.addSourceLocation(public.Span())
			publicDepIndex++
		}
		if imp.Weak {
			fdp.WeakDependency = append(fdp.WeakDependency, int32(i))
			dg.path = []int32{internal.FileWeakDependencyTag, weakDepIndex}
			_, weak := iterx.Find(seq.Values(imp.Decl.ModifierTokens()), func(t token.Token) bool {
				return t.Keyword() == keyword.Weak
			})
			// Comments are only attached to the top-level import source code info
			dg.addSourceLocation(weak.Span())
			weakDepIndex++
		}

		if dg.sourceCodeInfoExtn != nil && !imp.Used {
			dg.sourceCodeInfoExtn.UnusedDependency = append(dg.sourceCodeInfoExtn.UnusedDependency, int32(i))
		}

		dg.path = nil
	}

	var msgIndex, enumIndex int32
	for ty := range seq.Values(file.Types()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			fdp.EnumType = append(fdp.EnumType, edp)
			dg.path = []int32{internal.FileEnumsTag, enumIndex}
			dg.enum(ty, edp)
			dg.path = nil
			enumIndex++
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		fdp.MessageType = append(fdp.MessageType, mdp)
		dg.path = []int32{internal.FileMessagesTag, msgIndex}
		dg.message(ty, mdp)
		dg.path = nil
		msgIndex++
	}

	var svcIndex int32
	for service := range seq.Values(file.Services()) {
		sdp := new(descriptorpb.ServiceDescriptorProto)
		fdp.Service = append(fdp.Service, sdp)
		dg.path = []int32{internal.FileServicesTag, svcIndex}
		dg.service(service, sdp)
		dg.path = nil
		svcIndex++
	}

	var extnIndex int32
	for extend := range seq.Values(file.Extends()) {
		dg.path = []int32{internal.FileExtensionsTag}
		dg.addSourceLocation(
			extend.AST().Span(),
			extend.AST().KeywordToken().ID(),
			extend.AST().Body().Braces().ID(),
		)
		dg.path = nil

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			fdp.Extension = append(fdp.Extension, fd)
			dg.path = append(dg.path, internal.FileExtensionsTag, extnIndex)
			dg.field(extn, fd)
			dg.path = nil
			extnIndex++
		}
	}

	if options := file.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FileOptions)
		dg.path = []int32{internal.FileOptionsTag}
		for option := range file.AST().Options() {
			dg.addSourceLocation(option.Span())
		}
		dg.options(options, fdp.Options)
		dg.path = nil
	}

	if dg.sourceCodeInfoExtn != nil && iterx.Empty2(dg.sourceCodeInfoExtn.ProtoReflect().Range) {
		proto.ClearExtension(dg.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension)
	}

	if dg.sourceCodeInfo != nil {
		slices.SortFunc(dg.sourceCodeInfo.Location, func(a, b *descriptorpb.SourceCodeInfo_Location) int {
			return slices.Compare(a.Span, b.Span)
		})
		dg.sourceCodeInfo.Location = append(
			[]*descriptorpb.SourceCodeInfo_Location{{Span: locationSpan(file.AST().Span())}},
			dg.sourceCodeInfo.Location...,
		)
	}
}

func (dg *descGenerator) message(ty Type, mdp *descriptorpb.DescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	messageAST := ty.AST().AsMessage()
	dg.addSourceLocation(
		messageAST.Span(),
		messageAST.Keyword.ID(),
		messageAST.Body.Braces().ID(),
	)

	mdp.Name = addr(ty.Name())
	dg.path = append(dg.path, internal.MessageNameTag)
	dg.addSourceLocation(messageAST.Name.Span())
	resetPath()

	var fieldIndex int32
	for field := range seq.Values(ty.Members()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Field = append(mdp.Field, fd)
		dg.path = append(dg.path, internal.MessageFieldsTag, fieldIndex)
		dg.field(field, fd)
		resetPath()
		fieldIndex++
	}

	var extnIndex int32
	for extend := range seq.Values(ty.Extends()) {
		dg.path = append(dg.path, internal.MessageExtensionsTag)
		dg.addSourceLocation(
			extend.AST().Span(),
			extend.AST().KeywordToken().ID(),
			extend.AST().Body().Braces().ID(),
		)
		resetPath()

		for extn := range seq.Values(extend.Extensions()) {
			fd := new(descriptorpb.FieldDescriptorProto)
			mdp.Extension = append(mdp.Extension, fd)
			dg.path = append(dg.path, internal.MessageExtensionsTag, extnIndex)
			dg.field(extn, fd)
			resetPath()
			extnIndex++
		}
	}

	var enumIndex, nestedMsgIndex int32
	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			mdp.EnumType = append(mdp.EnumType, edp)
			dg.path = append(dg.path, internal.MessageEnumsTag, enumIndex)
			dg.enum(ty, edp)
			resetPath()
			enumIndex++
			continue
		}

		nested := new(descriptorpb.DescriptorProto)
		mdp.NestedType = append(mdp.NestedType, nested)
		dg.path = append(dg.path, internal.MessageNestedMessagesTag, nestedMsgIndex)
		dg.message(ty, nested)
		resetPath()
		nestedMsgIndex++
	}

	var erIndex int32
	for extensions := range seq.Values(ty.ExtensionRanges()) {
		er := new(descriptorpb.DescriptorProto_ExtensionRange)
		mdp.ExtensionRange = append(mdp.ExtensionRange, er)

		start, end := extensions.Range()
		er.Start = addr(start)
		er.End = addr(end + 1) // Exclusive.

		dg.path = append(dg.path, internal.MessageExtensionRangesTag)
		dg.addSourceLocation(
			extensions.DeclAST().Span(),
			extensions.DeclAST().KeywordToken().ID(),
			extensions.DeclAST().Semicolon().ID(),
		)
		resetPath()

		dg.rangeSourceCodeInfo(
			extensions.AST(),
			internal.MessageExtensionRangesTag,
			internal.ExtensionRangeStartTag,
			internal.ExtensionRangeEndTag,
			erIndex,
		)

		if options := extensions.Options(); !iterx.Empty(options.Fields()) {
			er.Options = new(descriptorpb.ExtensionRangeOptions)
			dg.path = append(dg.path, internal.ExtensionRangeOptionsTag)
			dg.options(options, er.Options)
			resetPath()
		}

		resetPath()
		erIndex++
	}

	var rrIndex int32
	setRRDeclAST := false
	for reserved := range seq.Values(ty.ReservedRanges()) {
		if !setRRDeclAST {
			dg.path = append(dg.path, internal.MessageReservedRangesTag)
			dg.addSourceLocation(
				reserved.DeclAST().Span(),
				reserved.DeclAST().KeywordToken().ID(),
				reserved.DeclAST().Semicolon().ID(),
			)
			resetPath()
			setRRDeclAST = true
		}

		rr := new(descriptorpb.DescriptorProto_ReservedRange)
		mdp.ReservedRange = append(mdp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end + 1) // Exclusive.

		dg.rangeSourceCodeInfo(
			reserved.AST(),
			internal.MessageReservedRangesTag,
			internal.ReservedRangeStartTag,
			internal.ReservedRangeEndTag,
			rrIndex,
		)

		resetPath()
		rrIndex++
	}

	var rnIndex int32
	setRNDeclAST := false
	for name := range seq.Values(ty.ReservedNames()) {
		if !setRNDeclAST {
			dg.path = append(dg.path, internal.MessageReservedNamesTag)
			dg.addSourceLocation(
				name.DeclAST().Span(),
				name.DeclAST().KeywordToken().ID(),
				name.DeclAST().Semicolon().ID(),
			)
			resetPath()
			setRNDeclAST = true
		}

		mdp.ReservedName = append(mdp.ReservedName, name.Name())

		dg.path = append(dg.path, internal.MessageReservedNamesTag, rnIndex)
		dg.addSourceLocation(name.AST().Span())
		resetPath()
		rnIndex++
	}

	var oneofIndex int32
	for oneof := range seq.Values(ty.Oneofs()) {
		odp := new(descriptorpb.OneofDescriptorProto)
		mdp.OneofDecl = append(mdp.OneofDecl, odp)
		dg.path = append(dg.path, internal.MessageOneofsTag, oneofIndex)
		dg.oneof(oneof, odp)
		resetPath()
		oneofIndex++
	}

	if dg.currentFile.Syntax() == syntax.Proto3 {
		var names syntheticNames

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
				Name: addr(names.generate(field.Name(), ty)),
			})
		}
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		mdp.Options = new(descriptorpb.MessageOptions)
		dg.path = append(dg.path, internal.MessageOptionsTag)
		dg.options(options, mdp.Options)
		resetPath()
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

func (dg *descGenerator) field(f Member, fdp *descriptorpb.FieldDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	fieldAST := f.AST().AsField()
	dg.addSourceLocation(
		fieldAST.Span(),
		token.ID(fieldAST.Type.ID()),
		fieldAST.Semicolon.ID(),
	)

	fdp.Name = addr(f.Name())
	dg.path = append(dg.path, internal.FieldNameTag)
	dg.addSourceLocation(fieldAST.Name.Span())
	resetPath()

	fdp.Number = addr(f.Number())
	dg.path = append(dg.path, internal.FieldNumberTag)
	dg.addSourceLocation(fieldAST.Tag.Span())
	resetPath()

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
		dg.path = append(dg.path, internal.FieldLabelTag)
		dg.addSourceLocation(prefix.PrefixToken().Span())
		resetPath()
	}

	if ty := f.Element(); !ty.IsZero() {
		if kind, _ := slicesx.Get(predeclaredToFDPType, ty.Predeclared()); kind != 0 {
			fdp.Type = kind.Enum()
			dg.path = append(dg.path, internal.FieldTypeTag)
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
			dg.path = append(dg.path, internal.FieldTypeNameTag)
		}
	}
	dg.addSourceLocation(fieldAST.Type.RemovePrefixes().Span())
	resetPath()

	if f.IsExtension() && f.Container().FullName() != "" {
		fdp.Extendee = addr(string(f.Container().FullName().ToAbsolute()))

		dg.path = append(dg.path, internal.FieldExtendeeTag)
		dg.addSourceLocation(f.Extend().AST().Name().Span())
		resetPath()
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FieldOptions)
		dg.path = append(dg.path, internal.FieldOptionsTag)
		dg.addSourceLocation(fieldAST.Options.Span())
		dg.options(options, fdp.Options)
		resetPath()
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

func (dg *descGenerator) oneof(o Oneof, odp *descriptorpb.OneofDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	oneofAST := o.AST().AsOneof()
	dg.addSourceLocation(
		oneofAST.Span(),
		oneofAST.Keyword.ID(),
		oneofAST.Body.Braces().ID(),
	)

	odp.Name = addr(o.Name())
	dg.path = append(dg.path, internal.OneofNameTag)
	dg.addSourceLocation(oneofAST.Name.Span())
	resetPath()

	if options := o.Options(); !iterx.Empty(options.Fields()) {
		odp.Options = new(descriptorpb.OneofOptions)
		dg.path = append(dg.path, internal.OneofOptionsTag)
		dg.options(options, odp.Options)
		resetPath()
	}
}

func (dg *descGenerator) enum(ty Type, edp *descriptorpb.EnumDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	enumAST := ty.AST().AsEnum()
	dg.addSourceLocation(
		enumAST.Span(),
		enumAST.Keyword.ID(),
		enumAST.Body.Braces().ID(),
	)

	edp.Name = addr(ty.Name())
	dg.path = append(dg.path, internal.EnumNameTag)
	dg.addSourceLocation(enumAST.Name.Span())
	resetPath()

	var enumValueIndex int32
	for field := range seq.Values(ty.Members()) {
		evd := new(descriptorpb.EnumValueDescriptorProto)
		edp.Value = append(edp.Value, evd)
		dg.path = append(dg.path, internal.EnumValuesTag, enumValueIndex)
		dg.enumValue(field, evd)
		resetPath()
		enumValueIndex++
	}

	var rrIndex int32
	setRRDeclAST := false
	for reserved := range seq.Values(ty.ReservedRanges()) {
		if !setRRDeclAST {
			dg.path = append(dg.path, internal.EnumReservedRangesTag)
			dg.addSourceLocation(
				reserved.DeclAST().Span(),
				reserved.DeclAST().KeywordToken().ID(),
				reserved.DeclAST().Semicolon().ID(),
			)
			resetPath()
			setRRDeclAST = true
		}
		rr := new(descriptorpb.EnumDescriptorProto_EnumReservedRange)
		edp.ReservedRange = append(edp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end) // Inclusive, not exclusive like the one for messages!

		dg.rangeSourceCodeInfo(
			reserved.AST(),
			internal.EnumReservedRangesTag,
			internal.ReservedRangeStartTag,
			internal.ReservedRangeEndTag,
			rrIndex,
		)

		resetPath()
		rrIndex++
	}

	var rnIndex int32
	for name := range seq.Values(ty.ReservedNames()) {
		edp.ReservedName = append(edp.ReservedName, name.Name())

		dg.path = append(dg.path, internal.EnumReservedNamesTag, rnIndex)
		dg.addSourceLocation(name.AST().Span())
		resetPath()
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		edp.Options = new(descriptorpb.EnumOptions)
		dg.path = append(dg.path, internal.EnumOptionsTag)
		dg.options(options, edp.Options)
		resetPath()
	}
}

func (dg *descGenerator) enumValue(f Member, evdp *descriptorpb.EnumValueDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	enumValueAST := f.AST().AsEnumValue()
	dg.addSourceLocation(
		enumValueAST.Span(),
		enumValueAST.Name.ID(),
		enumValueAST.Semicolon.ID(),
	)

	evdp.Name = addr(f.Name())
	evdp.Number = addr(f.Number())

	dg.path = append(dg.path, internal.EnumValNameTag)
	dg.addSourceLocation(enumValueAST.Name.Span())
	resetPath()

	dg.path = append(dg.path, internal.EnumValNumberTag)
	dg.addSourceLocation(enumValueAST.Tag.Span())
	resetPath()

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		evdp.Options = new(descriptorpb.EnumValueOptions)
		dg.path = append(dg.path, internal.EnumValOptionsTag)
		dg.options(options, evdp.Options)
		resetPath()
	}
}

func (dg *descGenerator) service(s Service, sdp *descriptorpb.ServiceDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	serviceAST := s.AST().AsService()
	dg.addSourceLocation(
		serviceAST.Span(),
		serviceAST.Keyword.ID(),
		serviceAST.Body.Braces().ID(),
	)

	sdp.Name = addr(s.Name())
	dg.path = append(dg.path, internal.ServiceNameTag)
	dg.addSourceLocation(serviceAST.Name.Span())
	resetPath()

	var methodIndex int32
	for method := range seq.Values(s.Methods()) {
		mdp := new(descriptorpb.MethodDescriptorProto)
		sdp.Method = append(sdp.Method, mdp)
		dg.path = append(dg.path, internal.ServiceMethodsTag, methodIndex)
		dg.method(method, mdp)
		resetPath()
		methodIndex++
	}

	if options := s.Options(); !iterx.Empty(options.Fields()) {
		sdp.Options = new(descriptorpb.ServiceOptions)
		dg.path = append(dg.path, internal.ServiceOptionsTag)
		dg.options(options, sdp.Options)
		resetPath()
	}
}

func (dg *descGenerator) method(m Method, mdp *descriptorpb.MethodDescriptorProto) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	methodAST := m.AST().AsMethod()
	dg.addSourceLocation(
		methodAST.Span(),
		methodAST.Keyword.ID(),
		methodAST.Body.Braces().ID(),
	)

	mdp.Name = addr(m.Name())
	dg.path = append(dg.path, internal.MethodNameTag)
	dg.addSourceLocation(methodAST.Name.Span())
	resetPath()

	in, inStream := m.Input()
	mdp.InputType = addr(string(in.FullName()))
	mdp.ClientStreaming = addr(inStream)

	out, outStream := m.Output()
	mdp.OutputType = addr(string(out.FullName()))
	mdp.ServerStreaming = addr(outStream)

	// Methods only have a single input and output, see [descriptorpb.MethodDescriptorProto].
	inputAST := methodAST.Signature.Inputs().At(0)
	if prefixed := inputAST.AsPrefixed(); !prefixed.IsZero() {
		dg.path = append(dg.path, internal.MethodInputStreamTag)
		dg.addSourceLocation(prefixed.PrefixToken().Span())
		resetPath()
	}
	dg.path = append(dg.path, internal.MethodInputTag)
	dg.addSourceLocation(inputAST.RemovePrefixes().Span())

	outputAST := methodAST.Signature.Outputs().At(0)
	if prefixed := outputAST.AsPrefixed(); !prefixed.IsZero() {
		dg.path = append(dg.path, internal.MethodOutputStreamTag)
		dg.addSourceLocation(prefixed.PrefixToken().Span())
		resetPath()
	}
	dg.path = append(dg.path, internal.MethodOutputTag)
	dg.addSourceLocation(outputAST.RemovePrefixes().Span())
	resetPath()

	if options := m.Options(); !iterx.Empty(options.Fields()) {
		mdp.Options = new(descriptorpb.MethodOptions)
		dg.path = append(dg.path, internal.MethodOptionsTag)
		dg.options(options, mdp.Options)
		resetPath()
	}
}

func (dg *descGenerator) options(v MessageValue, target proto.Message) {
	target.ProtoReflect().SetUnknown(v.Marshal(nil, nil))
	dg.messageValueSourceCodeInfo(v)
}

// addr is a helper for creating a pointer out of any type, because Go is
// missing the syntax &"foo", etc.
func addr[T any](v T) *T { return &v }

func (dg *descGenerator) messageValueSourceCodeInfo(v MessageValue) {
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}

	for field := range v.Fields() {
		var fieldIndex int32
		for optionSpan := range seq.Values(field.OptionSpans()) {
			if optionSpan == nil {
				continue
			}

			dg.path = append(dg.path, field.Field().Number())
			if messageField := field.AsMessage(); !messageField.IsZero() {
				dg.messageValueSourceCodeInfo(messageField)
				continue
			}

			// HACK: For each optionSpan, check around it for the option keyword and semicolon.
			// If the non-skippable token directly before this optionSpan is the option keyword,
			// and the non-skippable token directly after the optionSpan is the semicolon, we
			// include both tokens as part of the span.
			// We also check the option keyword token and semicolon tokens for comments.
			span := optionSpan.Span()
			var checkCommentTokens []token.ID
			keyword, semicolon := dg.optionKeywordAndSemicolon(span)
			if !keyword.IsZero() && !semicolon.IsZero() {
				checkCommentTokens = []token.ID{keyword.ID(), semicolon.ID()}
				span = source.Span{
					File:  span.File,
					Start: keyword.Span().Start,
					End:   semicolon.Span().End,
				}
			}

			if field.Field().IsRepeated() {
				dg.path = append(dg.path, fieldIndex)
				dg.addSourceLocation(span, checkCommentTokens...)
				dg.path = dg.path[:len(dg.path)-1]
				fieldIndex++
			} else {
				dg.addSourceLocation(span, checkCommentTokens...)
			}
			resetPath()
		}
		resetPath()
	}
}

// optionKeywordAndSemicolon is a helper function that checks the non-skippable tokens
// before and after the given span. If the non-skippable token before is the option keyword
// and the non-skippable token after is the semicolon, then both are returned.
func (dg *descGenerator) optionKeywordAndSemicolon(optionSpan source.Span) (token.Token, token.Token) {
	_, start := dg.currentFile.AST().Stream().Around(optionSpan.Start)
	before := token.NewCursorAt(start)
	prev := before.Prev()
	if prev.Keyword() != keyword.Option {
		return token.Zero, token.Zero
	}
	_, end := dg.currentFile.AST().Stream().Around(optionSpan.End)
	after := token.NewCursorAt(end)
	next := after.Next()
	if next.Keyword() != keyword.Semi {
		return token.Zero, token.Zero
	}
	return prev, next
}

func (dg *descGenerator) rangeSourceCodeInfo(rangeAST ast.ExprAny, baseTag, startTag, endTag, index int32) {
	dg.path = append(dg.path, baseTag, index)
	base := len(dg.path)
	resetPath := func() {
		dg.path = dg.path[:base]
	}
	dg.addSourceLocation(rangeAST.Span()) // Comments on ranges are dropped

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
		dg.path = append(dg.path, startTag)
		dg.addSourceLocation(startSpan) // Comments on ranges are dropped
		resetPath()
	}

	if endTag != 0 {
		dg.path = append(dg.path, endTag)
		dg.addSourceLocation(endSpan) // Comments on ranges are dropped
		resetPath()
	}
}

// addSourceCodeInfo adds the source code info location based on the current path tracked
// by the [descGenerator].
//
// It also optionally takes [token.ID]s for checking comments on. It looks up the given
// [token.ID]'s in the [commentTracker] and sets the comments accordingly. If comments are
// set across multiple tokens, the last token "wins", so it is up to the caller to manage
// the token IDs checked.
func (dg *descGenerator) addSourceLocation(span source.Span, checkForComments ...token.ID) {
	if dg.sourceCodeInfo != nil && !span.IsZero() {
		location := new(descriptorpb.SourceCodeInfo_Location)
		dg.sourceCodeInfo.Location = append(dg.sourceCodeInfo.Location, location)
		location.Path = internal.ClonePath(dg.path)
		location.Span = locationSpan(span)

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
								str += strings.TrimPrefix(strings.TrimLeftFunc(line, func(r rune) bool { return unicode.IsSpace(r) }), "*")
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
		for _, id := range checkForComments {
			comments, ok := dg.commentTracker.donated[id]
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

// commentTracker is used to track and attribute comments in a token stream. All attributed
// comments are stored in [commentTracker].donated for easy look-up by [token.ID].
type commentTracker struct {
	currentCursor *token.Cursor
	donated       map[token.ID]comments
	tracked       []paragraph

	current                []token.Token
	prev                   token.ID // The last non-skippable token.
	firstCommentOnSameLine bool
}

// A paragraph is a group of comment and whitespace tokens that make up a single paragraph comment.
type paragraph []token.Token

// Comments are the leading, trailing, and detached comments associated with a token.
type comments struct {
	leading  []token.Token
	trailing []token.Token
	detached []paragraph
}

// attributeComments walks the given token stream and groups comment and space tokens
// into [paragraph]s and "donates" them to non-skippable tokens as leading, trailing, and
// detached comments.
func (ct *commentTracker) attributeComments(cursor *token.Cursor) {
	ct.currentCursor = cursor
	t := cursor.NextSkippable()
	for !t.IsZero() {
		switch t.Kind() {
		case token.Comment:
			ct.handleCommentToken(t)
		case token.Space:
			ct.handleSpaceToken(t)
		default:
			ct.handleNonSkippableToken(t)
		}
		if !t.IsLeaf() {
			ct.attributeComments(t.Children())
			_, end := t.StartEnd()
			ct.handleNonSkippableToken(end)
			ct.currentCursor = cursor
		}
		t = cursor.NextSkippable()
	}
}

// For comment tokens, we need to determine whether to start a new comment or track it as
// part of an existing comment.
//
// For line comments, we track whether or not it is on the same line as the previous
// non-skippable token. A line comment is always in a [paragraph] starting with itself if
// there are no newlines between it and the previous non-skippable tokens.
//
// A block comment cannot be made into a paragraph with other tokens, so we need to close
// out the [paragraph] we are currently tracking and track it as its own paragraph.
//
// We always track the first comment token since the last non-skippable token.
func (ct *commentTracker) handleCommentToken(t token.Token) {
	prev := id.Wrap(ct.currentCursor.Context(), ct.prev)
	isLineComment := strings.HasPrefix(t.Text(), "//")

	if !isLineComment {
		// Block comments are their own paragraph, so we close the current paragraph and track
		// the current block comment as its own paragraph.
		ct.closeParagraph()
		ct.current = append(ct.current, t)
		ct.closeParagraph()
		return
	}

	if ct.current == nil {
		ct.current = append(ct.current, t)

		if !prev.IsZero() && ct.newLinesBetween(prev, t, 1) == 0 {
			// This first comment is always in a paragraph by itself if there are no newlines
			// between it and the previous non-skippable token.
			ct.closeParagraph()
			ct.firstCommentOnSameLine = true
		}
		return
	}

	// Track the current comment token.
	ct.current = append(ct.current, t)
}

// For space tokens, we need to determine whether this space token is part of a comment or
// if it requires us to break the current paragraph.
//
// We first check if there are any tokens already being tracked as part of the paragraph.
// If not, then we do not start paragraphs with spaces, and the token is dropped.
//
// If a newline token is preceded by another token that ends with a newline, then we break
// the current paragraph and start a new one. Otherwise, we attach it to the current paragraph.
//
// We throw away all other space tokens.
func (ct *commentTracker) handleSpaceToken(t token.Token) {
	if strings.HasSuffix(t.Text(), "\n") && len(ct.current) > 0 {
		if strings.HasSuffix(ct.current[len(ct.current)-1].Text(), "\n") {
			ct.closeParagraph()
		} else {
			ct.current = append(ct.current, t)
		}
	}
}

// For non-skippable tokens, we first break off the current paragraph. We then determine
// where to donate currently tracked comments and reset currently tracked comments.
//
// Comments are either donated as leading or detached leading comments on the current token
// or as trailing comments on the last seen non-skippable token.
func (ct *commentTracker) handleNonSkippableToken(t token.Token) {
	ct.closeParagraph()
	prev := id.Wrap(ct.currentCursor.Context(), ct.prev)

	if len(ct.tracked) > 0 {
		var donate bool
		switch {
		case prev.IsZero():
			donate = false
		case ct.firstCommentOnSameLine:
			donate = true
		case ct.newLinesBetween(prev, ct.tracked[0][0], 2) < 2:
			// We check the remaining three criteria for donation if there are more than 2
			// newlines between the previous non-skippable token and the beginning of the first
			// currently tracked paragraph. These are:
			//
			// 1. Is there more than one comment? If not, donate.
			// 2. Is the current token one of the closers, ), ], or } (but not >). If yes, we
			//    donate the currently tracked paragraphs because a body is closed.
			// 3. Is there more than one newline between the current token and the end of the
			//    first tracked paragraph? If yes, donate.
			switch {
			case len(ct.tracked) > 1 && ct.tracked[1] != nil:
				donate = true
			case slices.Contains([]string{
				keyword.LParen.String(),
				keyword.LBracket.String(),
				keyword.LBrace.String(),
			}, t.Text()):
				donate = true
			case ct.newLinesBetween(ct.tracked[0][len(ct.tracked[0])-1], t, 2) > 1:
				donate = true
			}
		}

		if donate {
			ct.setTrailing(ct.tracked[0], prev)
			ct.tracked = ct.tracked[1:]
		}

		if len(ct.tracked) > 0 {
			// The leading comment must have precisely one new line between it and the current token.
			if last := ct.tracked[len(ct.tracked)-1]; ct.newLinesBetween(last[len(last)-1], t, 2) == 1 {
				ct.setLeading(last, t)
				ct.tracked = ct.tracked[:len(ct.tracked)-1]
			}
		}

		// Check the remaining tracked comments to see if they are detached comments.
		// Detached comments must be separated from other non-space tokens by at least 2
		// newlines (unless they are at the top of the file), e.g.
		//
		// // This is a detached comment at the top of the file.
		//
		//  edition = "2023";
		//
		// message Foo {}
		// // This is neither a detached nor trailing comment, since it is not separated from
		// // the closing brace above by an empty line.
		//
		// // This IS a detached comment for Bar.
		//
		// // A leading comment for Bar.
		// message Bar {}
		for i, remaining := range ct.tracked {
			prev := remaining[0].Prev()
			for prev.Kind() == token.Space {
				prev = prev.Prev()
			}
			next := remaining[len(remaining)-1].Next()
			for next.Kind() == token.Space {
				next = next.Next()
			}
			if prev.IsZero() || ct.newLinesBetween(prev, remaining[0], 2) == 2 {
				if !next.IsZero() && ct.newLinesBetween(remaining[len(remaining)-1], next, 2) == 2 {
					ct.setDetached(ct.tracked[i:], t)
					break
				}
			}
		}
		// Reset tracked comment information
		ct.firstCommentOnSameLine = false
		ct.tracked = nil
	}
	ct.prev = t.ID()
}

func (ct *commentTracker) closeParagraph() {
	// If the current paragraph only contains whitespace tokens, then we throw it away.
	var containsComment bool
	for _, t := range ct.current {
		if t.Kind() == token.Comment {
			containsComment = true
			break
		}
	}
	if containsComment {
		ct.tracked = append(ct.tracked, ct.current)
	}
	ct.current = nil
}

// newLinesBetween counts the number of \n characters between the end of [token.Token] a
// and the start of b, up to max.
//
// The final rune of a is included in this count, since comments may end in a \n rune.
func (ct *commentTracker) newLinesBetween(a, b token.Token, max int) int {
	end := a.LeafSpan().End
	if end != 0 {
		// Account for the final rune of a
		end--
	}

	start := b.LeafSpan().Start
	between := ct.currentCursor.Context().Text()[end:start]

	var total int
	for total < max {
		var found bool
		_, between, found = strings.Cut(between, "\n")
		if !found {
			break
		}

		total++
	}
	return total
}

func (ct *commentTracker) setLeading(leading paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.leading = leading
	})
}

func (ct *commentTracker) setTrailing(trailing paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.trailing = trailing
	})
}

func (ct *commentTracker) setDetached(detached []paragraph, t token.Token) {
	ct.mutateComment(t, func(raw *comments) {
		raw.detached = detached
	})

}

func (ct *commentTracker) mutateComment(t token.Token, cb func(*comments)) {
	if ct.donated == nil {
		ct.donated = make(map[token.ID]comments)
	}

	raw := ct.donated[t.ID()]
	cb(&raw)
	ct.donated[t.ID()] = raw
}
