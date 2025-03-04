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
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
	"github.com/bufbuild/protocompile/wellknownimports"
)

// FDS code-generates a FileDescriptorSet for the given files, and returns the
// result as an encoded byte slice.
//
// The resulting FDS is always fully linked: it contains all dependencies except
// the WKTs, and all names are fully-qualified.
func FDS(files []File, options ...CodegenOption) ([]byte, error) {
	var c codegen
	for _, opt := range options {
		if opt != nil {
			opt(&c)
		}
	}

	fds := new(descriptorpb.FileDescriptorSet)
	c.files(files, fds)
	return proto.Marshal(fds)
}

// FDP code-generates a single FileDescriptorProto for file, and returns the
// result as an encoded byte slice.
//
// The resulting FDP is fully linked: all names are fully-qualified.
func FDP(file File, options ...CodegenOption) ([]byte, error) {
	var c codegen
	for _, opt := range options {
		if opt != nil {
			opt(&c)
		}
	}

	fdp := new(descriptorpb.FileDescriptorProto)
	c.file(file, fdp)
	return proto.Marshal(fdp)
}

// CodegenOption is an option to pass to [DescriptorSet].
type CodegenOption func(*codegen)

// IncludeWKTs is an option for including FDPs for the Well-Known Types in
// codegen output.
func IncludeWKTs() CodegenOption {
	return func(c *codegen) { c.includeWKTs = true }
}

// IncludeSourceCodeInfo is an option that requires generating source code info
// for the descriptors.
func IncludeSourceCodeInfo() CodegenOption {
	return func(c *codegen) { c.includeSourceCodeInfo = true }
}

type codegen struct {
	includeWKTs           bool
	includeSourceCodeInfo bool

	currentFile File
}

func (c *codegen) files(files []File, fds *descriptorpb.FileDescriptorSet) {
	if c.includeSourceCodeInfo {
		panic("protocompile/ir: NYI: IncludeSourceCodeInfo()")
	}

	// Build up all of the imported files. We can't just pull out the transitive
	// imports for each file because we want the result to be sorted
	// topologically.
	for file := range TopologicalSort(files...) {
		if !c.includeWKTs && wellknownimports.StandardImport(file.Path()) != "" {
			continue
		}

		fdp := new(descriptorpb.FileDescriptorProto)
		fds.File = append(fds.File, fdp)
		c.file(file, fdp)
	}
}

func (c *codegen) file(file File, fdp *descriptorpb.FileDescriptorProto) {
	c.currentFile = file

	fdp.Name = addr(file.Path())
	fdp.Package = addr(file.Context().Package())

	if file.Syntax().IsEdition() {
		fdp.Syntax = addr("editions")
		//nolint:gocritic // Complains about single-case switch.
		switch file.Syntax() {
		case syntax.Edition2023:
			fdp.Edition = descriptorpb.Edition_EDITION_2023.Enum()
		}
	} else {
		fdp.Syntax = addr(file.Syntax().String())
	}

	for imp := range seq.Values(file.Imports()) {
		fdp.Dependency = append(fdp.Dependency, imp.Path())
		if imp.Public {
			fdp.PublicDependency = append(fdp.PublicDependency, int32(len(fdp.Dependency)-1))
		}
		if imp.Weak {
			fdp.WeakDependency = append(fdp.WeakDependency, int32(len(fdp.Dependency)-1))
		}
	}

	for ty := range seq.Values(file.Types()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			fdp.EnumType = append(fdp.EnumType, edp)
			c.enum(ty, edp)
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		fdp.MessageType = append(fdp.MessageType, mdp)
		c.message(ty, mdp)
	}

	// TODO: Services.

	for extn := range seq.Values(file.Extensions()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		fdp.Extension = append(fdp.Extension, fd)
		c.field(extn, fd)
	}

	for option := range seq.Values(file.Options()) {
		if fdp.Options == nil {
			fdp.Options = new(descriptorpb.FileOptions)
		}

		c.option(option, fdp.Options)
	}
}

func (c *codegen) message(ty Type, mdp *descriptorpb.DescriptorProto) {
	mdp.Name = addr(ty.Name()) // Has a leading dot.

	for field := range seq.Values(ty.Fields()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Field = append(mdp.Field, fd)
		c.field(field, fd)
	}

	for extn := range seq.Values(ty.Extensions()) {
		fd := new(descriptorpb.FieldDescriptorProto)
		mdp.Extension = append(mdp.Extension, fd)
		c.field(extn, fd)
	}

	for ty := range seq.Values(ty.Nested()) {
		if ty.IsEnum() {
			edp := new(descriptorpb.EnumDescriptorProto)
			mdp.EnumType = append(mdp.EnumType, edp)
			c.enum(ty, edp)
			continue
		}

		mdp := new(descriptorpb.DescriptorProto)
		mdp.NestedType = append(mdp.NestedType, mdp)
		c.message(ty, mdp)
	}

	//nolint:revive // Complains about range_.
	for range_ := range seq.Values(ty.ExtensionRanges()) {
		er := new(descriptorpb.DescriptorProto_ExtensionRange)
		mdp.ExtensionRange = append(mdp.ExtensionRange, er)

		start, end := range_.Range()
		er.Start = addr(start)
		er.End = addr(end - 1)

		for option := range seq.Values(range_.Options()) {
			if er.Options == nil {
				er.Options = new(descriptorpb.ExtensionRangeOptions)
			}

			c.option(option, er.Options)
		}
	}

	//nolint:revive // Complains about range_.
	for range_ := range seq.Values(ty.ReservedRanges()) {
		rr := new(descriptorpb.DescriptorProto_ReservedRange)
		mdp.ReservedRange = append(mdp.ReservedRange, rr)

		start, end := range_.Range()
		rr.Start = addr(start)
		rr.End = addr(end - 1)
	}

	for name := range seq.Values(ty.ReservedNames()) {
		mdp.ReservedName = append(mdp.ReservedName, name.Name())
	}

	for oneof := range seq.Values(ty.Oneofs()) {
		odp := new(descriptorpb.OneofDescriptorProto)
		mdp.OneofDecl = append(mdp.OneofDecl, odp)
		c.oneof(oneof, odp)
	}

	if c.currentFile.Syntax() == syntax.Proto3 {
		var names syntheticNames

		// Only now that we have added all of the normal oneofs do we add the
		// synthetic oneofs.
		for i, field := range seq.All(ty.Fields()) {
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

	for option := range seq.Values(ty.Options()) {
		if mdp.Options == nil {
			mdp.Options = new(descriptorpb.MessageOptions)
		}

		c.option(option, mdp.Options)
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

func (c *codegen) field(f Field, fdp *descriptorpb.FieldDescriptorProto) {
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

	ty := f.Element()
	if kind, _ := slicesx.Get(predeclaredToFDPType, ty.Predeclared()); kind != 0 {
		fdp.Type = kind.Enum()
	} else if ty.IsEnum() {
		fdp.Type = descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum()
		fdp.TypeName = addr(ty.Name())
	} else {
		// TODO: Groups
		fdp.Type = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
		fdp.TypeName = addr(ty.Name())
	}

	if f.IsExtension() {
		fdp.Extendee = addr(f.Container().Name())
	}

	if oneof := f.Oneof(); !oneof.IsZero() {
		fdp.OneofIndex = addr(int32(oneof.Index()))
	}

	for option := range seq.Values(f.Options()) {
		if fdp.Options == nil {
			fdp.Options = new(descriptorpb.FieldOptions)
		}

		// We pass the FDP directly, because we want to keep it around for
		// dealing with the pseudo-options. codegen.option has a special case
		// for this.
		c.option(option, fdp)
	}
}

func (c *codegen) oneof(o Oneof, odp *descriptorpb.OneofDescriptorProto) {
	odp.Name = addr(o.Name())

	for option := range seq.Values(o.Options()) {
		if odp.Options == nil {
			odp.Options = new(descriptorpb.OneofOptions)
		}

		c.option(option, odp.Options)
	}
}

func (c *codegen) enum(ty Type, mdp *descriptorpb.EnumDescriptorProto) {
	mdp.Name = addr(ty.Name()) // Has a leading dot.

	for field := range seq.Values(ty.Fields()) {
		evd := new(descriptorpb.EnumValueDescriptorProto)
		mdp.Value = append(mdp.Value, evd)
		c.enumValue(field, evd)
	}

	//nolint:revive // Complains about range_.
	for range_ := range seq.Values(ty.ReservedRanges()) {
		rr := new(descriptorpb.EnumDescriptorProto_EnumReservedRange)
		mdp.ReservedRange = append(mdp.ReservedRange, rr)

		start, end := range_.Range()
		rr.Start = addr(start)
		rr.End = addr(end - 1)
	}

	for name := range seq.Values(ty.ReservedNames()) {
		mdp.ReservedName = append(mdp.ReservedName, name.Name())
	}

	for option := range seq.Values(ty.Options()) {
		if mdp.Options == nil {
			mdp.Options = new(descriptorpb.EnumOptions)
		}

		c.option(option, mdp.Options)
	}
}

func (c *codegen) enumValue(f Field, fdp *descriptorpb.EnumValueDescriptorProto) {
	fdp.Name = addr(f.Name())
	fdp.Number = addr(f.Number())

	for option := range seq.Values(f.Options()) {
		if fdp.Options == nil {
			fdp.Options = new(descriptorpb.EnumValueOptions)
		}

		c.option(option, fdp.Options)
	}
}

func (c *codegen) option(_ Option, target proto.Message) {
	var fdp *descriptorpb.FieldDescriptorProto
	if actual, ok := target.(*descriptorpb.FieldDescriptorProto); ok {
		fdp = actual
		target = fdp.Options
	}

	_ = target

	// There are two cases and both are painful.
	//
	// 1. For built-in options, we need to match up option.Field() to a
	//    protoreflect.Field in target, and then set it.
	//
	//    If we recognize this field as a pseudo-option, we need to forgo the
	//    above and set it directly on the non-nil fdp instead.
	//
	// 2. For custom options, we need to serialize option (perhaps with an
	//    Option.Marshal() function?) and append it to the unknown fields.

	// TODO: Implement the above (ow ow ow).
}

// addr is a helper for creating a pointer out of any type, because Go is
// missing the syntax &"foo", etc.
func addr[T any](v T) *T { return &v }

// Set of all names that are defined in scope of some message; used for
// generating synthetic names.
type syntheticNames map[intern.ID]struct{}

func (sn *syntheticNames) generate(candidate string, message Type) string {
	table := message.Context().intern
	var names = *sn
	if *sn == nil {
		names = make(map[intern.ID]struct{})
		*sn = names

		for field := range seq.Values(message.Fields()) {
			names[field.InternedName()] = struct{}{}
		}
		for extn := range seq.Values(message.Extensions()) {
			names[extn.InternedName()] = struct{}{}
		}
		for oneof := range seq.Values(message.Oneofs()) {
			names[oneof.InternedName()] = struct{}{}
		}
		for ty := range seq.Values(message.Nested()) {
			name := ty.Name()
			name = name[strings.LastIndex(name, ".")+1:]
			names[table.Intern(name)] = struct{}{}
		}
	}

	if !strings.HasPrefix(candidate, "_") {
		candidate = "_" + candidate
	}
	for intern.Contains(table, names, candidate) {
		candidate = "X" + candidate
	}

	names[table.Intern(candidate)] = struct{}{}
	return candidate
}
