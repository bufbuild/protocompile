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

	descriptorv1 "buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go/buf/descriptor/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
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

	sourceCodeInfo     *descriptorpb.SourceCodeInfo
	sourceCodeInfoExtn *descriptorv1.SourceCodeInfoExtension
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
	if dg.includeDebugInfo {
		dg.sourceCodeInfo = new(descriptorpb.SourceCodeInfo)
		fdp.SourceCodeInfo = dg.sourceCodeInfo

		dg.sourceCodeInfoExtn = new(descriptorv1.SourceCodeInfoExtension)
		proto.SetExtension(dg.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension, dg.sourceCodeInfoExtn)
	}

	fdp.Name = addr(file.Path())
	fdp.Package = addr(string(file.Package()))

	if file.Syntax().IsEdition() {
		fdp.Syntax = addr("editions")
		fdp.Edition = descriptorpb.Edition(file.Syntax()).Enum()
	} else {
		fdp.Syntax = addr(file.Syntax().String())
	}

	if dg.sourceCodeInfoExtn != nil {
		dg.sourceCodeInfoExtn.IsSyntaxUnspecified = file.AST().Syntax().IsZero()
	}

	// Canonicalize import order so that it does not change whenever we refactor
	// internal structures.
	imports := seq.ToSlice(file.Imports())
	slices.SortFunc(imports, cmpx.Key(func(imp Import) int {
		return imp.Decl.KeywordToken().Span().Start
	}))
	for i, imp := range imports {
		fdp.Dependency = append(fdp.Dependency, imp.Path())
		if imp.Public {
			fdp.PublicDependency = append(fdp.PublicDependency, int32(i))
		}
		if imp.Weak {
			fdp.WeakDependency = append(fdp.WeakDependency, int32(i))
		}

		if dg.sourceCodeInfoExtn != nil && !imp.Used {
			dg.sourceCodeInfoExtn.UnusedDependency = append(dg.sourceCodeInfoExtn.UnusedDependency, int32(i))
		}
	}

	for ty := range seq.Values(file.Types()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			fdp.EnumType = append(fdp.EnumType, edp)
			dg.enum(ty, edp)
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		fdp.MessageType = append(fdp.MessageType, mdp)
		dg.message(ty, mdp)
	}

	for service := range seq.Values(file.Services()) {
		sdp := new(descriptorpb.ServiceDescriptorProto)
		fdp.Service = append(fdp.Service, sdp)
		dg.service(service, sdp)
	}

	for extn := range seq.Values(file.Extensions()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		fdp.Extension = append(fdp.Extension, fd)
		dg.field(extn, fd)
	}

	if options := file.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FileOptions)
		dg.options(options, fdp.Options)
	}

	if dg.sourceCodeInfoExtn != nil && iterx.Empty2(dg.sourceCodeInfoExtn.ProtoReflect().Range) {
		proto.ClearExtension(dg.sourceCodeInfo, descriptorv1.E_BufSourceCodeInfoExtension)
	}
}

func (dg *descGenerator) message(ty Type, mdp *descriptorpb.DescriptorProto) {
	mdp.Name = addr(ty.Name())

	for field := range seq.Values(ty.Members()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Field = append(mdp.Field, fd)
		dg.field(field, fd)
	}

	for extn := range seq.Values(ty.Extensions()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Extension = append(mdp.Extension, fd)
		dg.field(extn, fd)
	}

	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			mdp.EnumType = append(mdp.EnumType, edp)
			dg.enum(ty, edp)
			continue
		}

		nested := new(descriptorpb.DescriptorProto)
		mdp.NestedType = append(mdp.NestedType, nested)
		dg.message(ty, nested)
	}

	for extensions := range seq.Values(ty.ExtensionRanges()) {
		er := new(descriptorpb.DescriptorProto_ExtensionRange)
		mdp.ExtensionRange = append(mdp.ExtensionRange, er)

		start, end := extensions.Range()
		er.Start = addr(start)
		er.End = addr(end + 1) // Exclusive.

		if options := extensions.Options(); !iterx.Empty(options.Fields()) {
			er.Options = new(descriptorpb.ExtensionRangeOptions)
			dg.options(options, er.Options)
		}
	}

	for reserved := range seq.Values(ty.ReservedRanges()) {
		rr := new(descriptorpb.DescriptorProto_ReservedRange)
		mdp.ReservedRange = append(mdp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end + 1) // Exclusive.
	}

	for name := range seq.Values(ty.ReservedNames()) {
		mdp.ReservedName = append(mdp.ReservedName, name.Name())
	}

	for oneof := range seq.Values(ty.Oneofs()) {
		odp := new(descriptorpb.OneofDescriptorProto)
		mdp.OneofDecl = append(mdp.OneofDecl, odp)
		dg.oneof(oneof, odp)
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
		dg.options(options, mdp.Options)
	}
}

var predeclaredToFDPType = []descriptorpb.FieldDescriptorProto_Type{
	predeclared.Int32:  descriptorpb.FieldDescriptorProto_TYPE_INT32,
	predeclared.Int64:  descriptorpb.FieldDescriptorProto_TYPE_INT64,
	predeclared.UInt32: descriptorpb.FieldDescriptorProto_TYPE_INT32,
	predeclared.UInt64: descriptorpb.FieldDescriptorProto_TYPE_INT64,
	predeclared.SInt32: descriptorpb.FieldDescriptorProto_TYPE_INT32,
	predeclared.SInt64: descriptorpb.FieldDescriptorProto_TYPE_INT64,

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
	fdp.Name = addr(f.Name())
	fdp.Number = addr(f.Number())

	switch f.Presence() {
	case presence.Explicit, presence.Implicit, presence.Shared:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	case presence.Repeated:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	case presence.Required:
		fdp.Label = descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum()
	}

	if ty := f.Element(); !ty.IsZero() {
		if kind, _ := slicesx.Get(predeclaredToFDPType, ty.Predeclared()); kind != 0 {
			fdp.Type = kind.Enum()
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

	if f.IsExtension() && f.Container().FullName() != "" {
		fdp.Extendee = addr(string(f.Container().FullName().ToAbsolute()))
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		fdp.Options = new(descriptorpb.FieldOptions)
		dg.options(options, fdp.Options)
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
	odp.Name = addr(o.Name())

	if options := o.Options(); !iterx.Empty(options.Fields()) {
		odp.Options = new(descriptorpb.OneofOptions)
		dg.options(options, odp.Options)
	}
}

func (dg *descGenerator) enum(ty Type, edp *descriptorpb.EnumDescriptorProto) {
	edp.Name = addr(ty.Name())

	for field := range seq.Values(ty.Members()) {
		evd := new(descriptorpb.EnumValueDescriptorProto)
		edp.Value = append(edp.Value, evd)
		dg.enumValue(field, evd)
	}

	for reserved := range seq.Values(ty.ReservedRanges()) {
		rr := new(descriptorpb.EnumDescriptorProto_EnumReservedRange)
		edp.ReservedRange = append(edp.ReservedRange, rr)

		start, end := reserved.Range()
		rr.Start = addr(start)
		rr.End = addr(end) // Inclusive, not exclusive like the one for messages!
	}

	for name := range seq.Values(ty.ReservedNames()) {
		edp.ReservedName = append(edp.ReservedName, name.Name())
	}

	if options := ty.Options(); !iterx.Empty(options.Fields()) {
		edp.Options = new(descriptorpb.EnumOptions)
		dg.options(options, edp.Options)
	}
}

func (dg *descGenerator) enumValue(f Member, evdp *descriptorpb.EnumValueDescriptorProto) {
	evdp.Name = addr(f.Name())
	evdp.Number = addr(f.Number())

	if options := f.Options(); !iterx.Empty(options.Fields()) {
		evdp.Options = new(descriptorpb.EnumValueOptions)
		dg.options(options, evdp.Options)
	}
}

func (dg *descGenerator) service(s Service, sdp *descriptorpb.ServiceDescriptorProto) {
	sdp.Name = addr(s.Name())

	for method := range seq.Values(s.Methods()) {
		mdp := new(descriptorpb.MethodDescriptorProto)
		sdp.Method = append(sdp.Method, mdp)
		dg.method(method, mdp)
	}

	if options := s.Options(); !iterx.Empty(options.Fields()) {
		sdp.Options = new(descriptorpb.ServiceOptions)
		dg.options(options, sdp.Options)
	}
}

func (dg *descGenerator) method(m Method, mdp *descriptorpb.MethodDescriptorProto) {
	mdp.Name = addr(m.Name())

	in, inStream := m.Input()
	mdp.InputType = addr(string(in.FullName()))
	mdp.ClientStreaming = addr(inStream)

	out, outStream := m.Output()
	mdp.OutputType = addr(string(out.FullName()))
	mdp.ServerStreaming = addr(outStream)

	if options := m.Options(); !iterx.Empty(options.Fields()) {
		mdp.Options = new(descriptorpb.MethodOptions)
		dg.options(options, mdp.Options)
	}
}

func (dg *descGenerator) options(v MessageValue, target proto.Message) {
	target.ProtoReflect().SetUnknown(v.Marshal(nil, nil))
}

// addr is a helper for creating a pointer out of any type, because Go is
// missing the syntax &"foo", etc.
func addr[T any](v T) *T { return &v }
