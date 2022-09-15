// Copyright 2020-2022 Buf Technologies, Inc.
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

package linker

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/ast"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/protoutil"
)

// This file contains implementations of protoreflect.Descriptor. Note that
// this is a hack since those interfaces have a "doNotImplement" tag
// interface therein. We do just enough to make dynamicpb happy; constructing
// a regular descriptor would fail because we haven't yet interpreted options
// at the point we need these, and some validations will fail if the options
// aren't present.

type result struct {
	protoreflect.FileDescriptor
	parser.Result
	prefix         string
	descriptorPool map[string]proto.Message
	deps           Files
	descriptors    map[proto.Message]protoreflect.Descriptor
	usedImports    map[string]struct{}
	optionBytes    map[proto.Message][]byte
	srcLocs        []protoreflect.SourceLocation
	srcLocIndex    map[interface{}]protoreflect.SourceLocation
	// a map of AST nodes that represent identifiers in ast.FieldReferenceNodes
	// to their fully-qualified name. The identifiers are for field names in
	// message literals (in option values) that are extension fields. These names
	// are resolved during linking and stored here, to be used to interpret options.
	optionQualifiedNames map[ast.IdentValueNode]string
}

var _ protoreflect.FileDescriptor = (*result)(nil)
var _ Result = (*result)(nil)
var _ protoutil.DescriptorProtoWrapper = (*result)(nil)

func (r *result) AsProto() proto.Message {
	return r.FileDescriptorProto()
}

func (r *result) ParentFile() protoreflect.FileDescriptor {
	return r
}

func (r *result) Parent() protoreflect.Descriptor {
	return nil
}

func (r *result) Index() int {
	return 0
}

func (r *result) Syntax() protoreflect.Syntax {
	switch r.FileDescriptorProto().GetSyntax() {
	case "proto2", "":
		return protoreflect.Proto2
	case "proto3":
		return protoreflect.Proto3
	default:
		return 0 // ???
	}
}

func (r *result) Name() protoreflect.Name {
	return ""
}

func (r *result) FullName() protoreflect.FullName {
	return r.Package()
}

func (r *result) IsPlaceholder() bool {
	return false
}

func (r *result) Options() protoreflect.ProtoMessage {
	return r.FileDescriptorProto().Options
}

func (r *result) Path() string {
	return r.FileDescriptorProto().GetName()
}

func (r *result) Package() protoreflect.FullName {
	return protoreflect.FullName(r.FileDescriptorProto().GetPackage())
}

func (r *result) Imports() protoreflect.FileImports {
	return &fileImports{parent: r}
}

func (r *result) Enums() protoreflect.EnumDescriptors {
	return &enumDescriptors{file: r, parent: r, enums: r.FileDescriptorProto().GetEnumType(), prefix: r.prefix}
}

func (r *result) Messages() protoreflect.MessageDescriptors {
	return &msgDescriptors{file: r, parent: r, msgs: r.FileDescriptorProto().GetMessageType(), prefix: r.prefix}
}

func (r *result) Extensions() protoreflect.ExtensionDescriptors {
	return &extDescriptors{file: r, parent: r, exts: r.FileDescriptorProto().GetExtension(), prefix: r.prefix}
}

func (r *result) Services() protoreflect.ServiceDescriptors {
	return &svcDescriptors{file: r, svcs: r.FileDescriptorProto().GetService(), prefix: r.prefix}
}

func (r *result) SourceLocations() protoreflect.SourceLocations {
	srcInfoProtos := r.FileDescriptorProto().GetSourceCodeInfo().GetLocation()
	if r.srcLocs == nil && len(srcInfoProtos) > 0 {
		r.srcLocs = asSourceLocations(srcInfoProtos)
		r.srcLocIndex = computeSourceLocIndex(r.srcLocs)
	}
	return srcLocs{file: r, locs: r.srcLocs, index: r.srcLocIndex}
}

func computeSourceLocIndex(locs []protoreflect.SourceLocation) map[interface{}]protoreflect.SourceLocation {
	index := map[interface{}]protoreflect.SourceLocation{}
	for _, loc := range locs {
		if loc.Next == 0 {
			index[pathKey(loc.Path)] = loc
		}
	}
	return index
}

func asSourceLocations(srcInfoProtos []*descriptorpb.SourceCodeInfo_Location) []protoreflect.SourceLocation {
	locs := make([]protoreflect.SourceLocation, len(srcInfoProtos))
	prev := map[string]*protoreflect.SourceLocation{}
	for i, loc := range srcInfoProtos {
		var stLin, stCol, enLin, enCol int
		if len(loc.Span) == 3 {
			stLin, stCol, enCol = int(loc.Span[0]), int(loc.Span[1]), int(loc.Span[2])
			enLin = stLin
		} else {
			stLin, stCol, enLin, enCol = int(loc.Span[0]), int(loc.Span[1]), int(loc.Span[2]), int(loc.Span[3])
		}
		locs[i] = protoreflect.SourceLocation{
			Path:                    loc.Path,
			LeadingComments:         loc.GetLeadingComments(),
			LeadingDetachedComments: loc.GetLeadingDetachedComments(),
			TrailingComments:        loc.GetTrailingComments(),
			StartLine:               stLin,
			StartColumn:             stCol,
			EndLine:                 enLin,
			EndColumn:               enCol,
		}
		str := pathStr(loc.Path)
		pr := prev[str]
		if pr != nil {
			pr.Next = i
		}
		prev[str] = &locs[i]
	}
	return locs
}

func pathStr(p protoreflect.SourcePath) string {
	var buf bytes.Buffer
	for _, v := range p {
		fmt.Fprintf(&buf, "%x:", v)
	}
	return buf.String()
}

// AddOptionBytes associates the given opts (an options message encoded in the
// binary format) with the given options protobuf message. The protobuf message
// should exist in the hierarchy of this result's FileDescriptorProto. This
// allows the FileDescriptorProto to be marshaled to bytes in a way that
// preserves the way options are defined in source (just as is done by protoc,
// but not possible when only using the generated Go types and standard
// marshaling APIs in the protobuf runtime).
func (r *result) AddOptionBytes(pm proto.Message, opts []byte) {
	if r.optionBytes == nil {
		r.optionBytes = map[proto.Message][]byte{}
	}
	r.optionBytes[pm] = append(r.optionBytes[pm], opts...)
}

func (r *result) CanonicalProto() *descriptorpb.FileDescriptorProto {
	origFd := r.FileDescriptorProto()
	// make a copy that we can mutate
	fd := proto.Clone(origFd).(*descriptorpb.FileDescriptorProto)

	r.storeOptionBytesInFile(fd, origFd)

	return fd
}

func (r *result) storeOptionBytesInFile(fd, origFd *descriptorpb.FileDescriptorProto) {
	if fd.Options != nil {
		fd.Options.Reset()
		fd.Options.ProtoReflect().SetUnknown(r.optionBytes[origFd.Options])
	}

	for i, md := range fd.MessageType {
		origMd := origFd.MessageType[i]
		r.storeOptionBytesInMessage(md, origMd)
	}

	for i, ed := range fd.EnumType {
		origEd := origFd.EnumType[i]
		r.storeOptionBytesInEnum(ed, origEd)
	}

	for i, exd := range fd.Extension {
		origExd := origFd.Extension[i]
		r.storeOptionBytesInField(exd, origExd)
	}

	for i, sd := range fd.Service {
		origSd := origFd.Service[i]
		if sd.Options != nil {
			sd.Options.Reset()
			sd.Options.ProtoReflect().SetUnknown(r.optionBytes[origSd.Options])
		}

		for j, mtd := range sd.Method {
			origMtd := origSd.Method[j]
			if mtd.Options != nil {
				mtd.Options.Reset()
				mtd.Options.ProtoReflect().SetUnknown(r.optionBytes[origMtd.Options])
			}
		}
	}
}

func (r *result) storeOptionBytesInMessage(md, origMd *descriptorpb.DescriptorProto) {
	if md.GetOptions().GetMapEntry() {
		// Map entry messages are synthesized. They won't have any option bytes
		// since they don't actually appear in the source and thus have any option
		// declarations in the source.
		return
	}

	if md.Options != nil {
		md.Options.Reset()
		md.Options.ProtoReflect().SetUnknown(r.optionBytes[origMd.Options])
	}

	for i, fld := range md.Field {
		origFld := origMd.Field[i]
		r.storeOptionBytesInField(fld, origFld)
	}

	for i, ood := range md.OneofDecl {
		origOod := origMd.OneofDecl[i]
		if ood.Options != nil {
			ood.Options.Reset()
			ood.Options.ProtoReflect().SetUnknown(r.optionBytes[origOod.Options])
		}
	}

	for i, exr := range md.ExtensionRange {
		origExr := origMd.ExtensionRange[i]
		if exr.Options != nil {
			exr.Options.Reset()
			exr.Options.ProtoReflect().SetUnknown(r.optionBytes[origExr.Options])
		}
	}

	for i, nmd := range md.NestedType {
		origNmd := origMd.NestedType[i]
		r.storeOptionBytesInMessage(nmd, origNmd)
	}

	for i, ed := range md.EnumType {
		origEd := origMd.EnumType[i]
		r.storeOptionBytesInEnum(ed, origEd)
	}

	for i, exd := range md.Extension {
		origExd := origMd.Extension[i]
		r.storeOptionBytesInField(exd, origExd)
	}
}

func (r *result) storeOptionBytesInEnum(ed, origEd *descriptorpb.EnumDescriptorProto) {
	if ed.Options != nil {
		ed.Options.Reset()
		ed.Options.ProtoReflect().SetUnknown(r.optionBytes[origEd.Options])
	}

	for i, evd := range ed.Value {
		origEvd := origEd.Value[i]
		if evd.Options != nil {
			evd.Options.Reset()
			evd.Options.ProtoReflect().SetUnknown(r.optionBytes[origEvd.Options])
		}
	}
}

func (r *result) storeOptionBytesInField(fld, origFld *descriptorpb.FieldDescriptorProto) {
	if fld.Options != nil {
		fld.Options.Reset()
		fld.Options.ProtoReflect().SetUnknown(r.optionBytes[origFld.Options])
	}
}

type fileImports struct {
	protoreflect.FileImports
	parent *result
}

func (f *fileImports) Len() int {
	return len(f.parent.FileDescriptorProto().Dependency)
}

func (f *fileImports) Get(i int) protoreflect.FileImport {
	dep := f.parent.FileDescriptorProto().Dependency[i]
	desc := f.parent.deps.FindFileByPath(dep)
	isPublic := false
	for _, d := range f.parent.FileDescriptorProto().PublicDependency {
		if d == int32(i) {
			isPublic = true
			break
		}
	}
	isWeak := false
	for _, d := range f.parent.FileDescriptorProto().WeakDependency {
		if d == int32(i) {
			isWeak = true
			break
		}
	}
	return protoreflect.FileImport{FileDescriptor: desc, IsPublic: isPublic, IsWeak: isWeak}
}

type srcLocs struct {
	protoreflect.SourceLocations
	file  *result
	locs  []protoreflect.SourceLocation
	index map[interface{}]protoreflect.SourceLocation
}

func (s srcLocs) Len() int {
	return len(s.locs)
}

func (s srcLocs) Get(i int) protoreflect.SourceLocation {
	return s.locs[i]
}

func (s srcLocs) ByPath(p protoreflect.SourcePath) protoreflect.SourceLocation {
	return s.index[pathKey(p)]
}

func (s srcLocs) ByDescriptor(d protoreflect.Descriptor) protoreflect.SourceLocation {
	if d.ParentFile() != s.file {
		return protoreflect.SourceLocation{}
	}
	path, ok := computePath(d)
	if !ok {
		return protoreflect.SourceLocation{}
	}
	return s.ByPath(path)
}

func computePath(d protoreflect.Descriptor) (protoreflect.SourcePath, bool) {
	_, ok := d.(protoreflect.FileDescriptor)
	if ok {
		return nil, true
	}
	var path protoreflect.SourcePath
	for {
		p := d.Parent()
		switch d := d.(type) {
		case protoreflect.FileDescriptor:
			return reverse(path), true
		case protoreflect.MessageDescriptor:
			path = append(path, int32(d.Index()))
			switch p.(type) {
			case protoreflect.FileDescriptor:
				path = append(path, internal.FileMessagesTag)
			case protoreflect.MessageDescriptor:
				path = append(path, internal.MessageNestedMessagesTag)
			default:
				return nil, false
			}
		case protoreflect.FieldDescriptor:
			path = append(path, int32(d.Index()))
			switch p.(type) {
			case protoreflect.FileDescriptor:
				if d.IsExtension() {
					path = append(path, internal.FileExtensionsTag)
				} else {
					return nil, false
				}
			case protoreflect.MessageDescriptor:
				if d.IsExtension() {
					path = append(path, internal.MessageExtensionsTag)
				} else {
					path = append(path, internal.MessageFieldsTag)
				}
			default:
				return nil, false
			}
		case protoreflect.OneofDescriptor:
			path = append(path, int32(d.Index()))
			if _, ok := p.(protoreflect.MessageDescriptor); ok {
				path = append(path, internal.MessageOneOfsTag)
			} else {
				return nil, false
			}
		case protoreflect.EnumDescriptor:
			path = append(path, int32(d.Index()))
			switch p.(type) {
			case protoreflect.FileDescriptor:
				path = append(path, internal.FileEnumsTag)
			case protoreflect.MessageDescriptor:
				path = append(path, internal.MessageEnumsTag)
			default:
				return nil, false
			}
		case protoreflect.EnumValueDescriptor:
			path = append(path, int32(d.Index()))
			if _, ok := p.(protoreflect.EnumDescriptor); ok {
				path = append(path, internal.EnumValuesTag)
			} else {
				return nil, false
			}
		case protoreflect.ServiceDescriptor:
			path = append(path, int32(d.Index()))
			if _, ok := p.(protoreflect.FileDescriptor); ok {
				path = append(path, internal.FileServicesTag)
			} else {
				return nil, false
			}
		case protoreflect.MethodDescriptor:
			path = append(path, int32(d.Index()))
			if _, ok := p.(protoreflect.ServiceDescriptor); ok {
				path = append(path, internal.ServiceMethodsTag)
			} else {
				return nil, false
			}
		}
		d = p
	}
}

func reverse(p protoreflect.SourcePath) protoreflect.SourcePath {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	return p
}

type msgDescriptors struct {
	protoreflect.MessageDescriptors
	file   *result
	parent protoreflect.Descriptor
	msgs   []*descriptorpb.DescriptorProto
	prefix string
}

func (m *msgDescriptors) Len() int {
	return len(m.msgs)
}

func (m *msgDescriptors) Get(i int) protoreflect.MessageDescriptor {
	msg := m.msgs[i]
	return m.file.asMessageDescriptor(msg, m.file, m.parent, i, m.prefix+msg.GetName())
}

func (m *msgDescriptors) ByName(s protoreflect.Name) protoreflect.MessageDescriptor {
	for i, msg := range m.msgs {
		if msg.GetName() == string(s) {
			return m.Get(i)
		}
	}
	return nil
}

type msgDescriptor struct {
	protoreflect.MessageDescriptor
	file   *result
	parent protoreflect.Descriptor
	index  int
	proto  *descriptorpb.DescriptorProto
	fqn    string
}

var _ protoreflect.MessageDescriptor = (*msgDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*msgDescriptor)(nil)

func (r *result) asMessageDescriptor(md *descriptorpb.DescriptorProto, file *result, parent protoreflect.Descriptor, index int, fqn string) *msgDescriptor {
	if ret := r.descriptors[md]; ret != nil {
		return ret.(*msgDescriptor)
	}
	ret := &msgDescriptor{file: file, parent: parent, index: index, proto: md, fqn: fqn}
	r.descriptors[md] = ret
	return ret
}

func (m *msgDescriptor) MessageDescriptorProto() *descriptorpb.DescriptorProto {
	return m.proto
}

func (m *msgDescriptor) AsProto() proto.Message {
	return m.proto
}

func (m *msgDescriptor) ParentFile() protoreflect.FileDescriptor {
	return m.file
}

func (m *msgDescriptor) Parent() protoreflect.Descriptor {
	return m.parent
}

func (m *msgDescriptor) Index() int {
	return m.index
}

func (m *msgDescriptor) Syntax() protoreflect.Syntax {
	return m.file.Syntax()
}

func (m *msgDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(m.proto.GetName())
}

func (m *msgDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(m.fqn)
}

func (m *msgDescriptor) IsPlaceholder() bool {
	return false
}

func (m *msgDescriptor) Options() protoreflect.ProtoMessage {
	return m.proto.Options
}

func (m *msgDescriptor) IsMapEntry() bool {
	return m.proto.Options.GetMapEntry()
}

func (m *msgDescriptor) Fields() protoreflect.FieldDescriptors {
	return &fldDescriptors{file: m.file, parent: m, fields: m.proto.GetField(), prefix: m.fqn + "."}
}

func (m *msgDescriptor) Oneofs() protoreflect.OneofDescriptors {
	return &oneofDescriptors{file: m.file, parent: m, oneofs: m.proto.GetOneofDecl(), prefix: m.fqn + "."}
}

func (m *msgDescriptor) ReservedNames() protoreflect.Names {
	return names{s: m.proto.ReservedName}
}

func (m *msgDescriptor) ReservedRanges() protoreflect.FieldRanges {
	return fieldRanges{s: m.proto.ReservedRange}
}

func (m *msgDescriptor) RequiredNumbers() protoreflect.FieldNumbers {
	var indexes fieldNums
	for _, fld := range m.proto.Field {
		if fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED {
			indexes.s = append(indexes.s, fld.GetNumber())
		}
	}
	return indexes
}

func (m *msgDescriptor) ExtensionRanges() protoreflect.FieldRanges {
	return extRanges{s: m.proto.ExtensionRange}
}

func (m *msgDescriptor) ExtensionRangeOptions(i int) protoreflect.ProtoMessage {
	return m.proto.ExtensionRange[i].Options
}

func (m *msgDescriptor) Enums() protoreflect.EnumDescriptors {
	return &enumDescriptors{file: m.file, parent: m, enums: m.proto.GetEnumType(), prefix: m.fqn + "."}
}

func (m *msgDescriptor) Messages() protoreflect.MessageDescriptors {
	return &msgDescriptors{file: m.file, parent: m, msgs: m.proto.GetNestedType(), prefix: m.fqn + "."}
}

func (m *msgDescriptor) Extensions() protoreflect.ExtensionDescriptors {
	return &extDescriptors{file: m.file, parent: m, exts: m.proto.GetExtension(), prefix: m.fqn + "."}
}

type names struct {
	protoreflect.Names
	s []string
}

func (n names) Len() int {
	return len(n.s)
}

func (n names) Get(i int) protoreflect.Name {
	return protoreflect.Name(n.s[i])
}

func (n names) Has(s protoreflect.Name) bool {
	for _, name := range n.s {
		if name == string(s) {
			return true
		}
	}
	return false
}

type fieldNums struct {
	protoreflect.FieldNumbers
	s []int32
}

func (n fieldNums) Len() int {
	return len(n.s)
}

func (n fieldNums) Get(i int) protoreflect.FieldNumber {
	return protoreflect.FieldNumber(n.s[i])
}

func (n fieldNums) Has(s protoreflect.FieldNumber) bool {
	for _, num := range n.s {
		if num == int32(s) {
			return true
		}
	}
	return false
}

type fieldRanges struct {
	protoreflect.FieldRanges
	s []*descriptorpb.DescriptorProto_ReservedRange
}

func (f fieldRanges) Len() int {
	return len(f.s)
}

func (f fieldRanges) Get(i int) [2]protoreflect.FieldNumber {
	r := f.s[i]
	return [2]protoreflect.FieldNumber{
		protoreflect.FieldNumber(r.GetStart()),
		protoreflect.FieldNumber(r.GetEnd()),
	}
}

func (f fieldRanges) Has(n protoreflect.FieldNumber) bool {
	for _, r := range f.s {
		if r.GetStart() <= int32(n) && r.GetEnd() > int32(n) {
			return true
		}
	}
	return false
}

type extRanges struct {
	protoreflect.FieldRanges
	s []*descriptorpb.DescriptorProto_ExtensionRange
}

func (e extRanges) Len() int {
	return len(e.s)
}

func (e extRanges) Get(i int) [2]protoreflect.FieldNumber {
	r := e.s[i]
	return [2]protoreflect.FieldNumber{
		protoreflect.FieldNumber(r.GetStart()),
		protoreflect.FieldNumber(r.GetEnd()),
	}
}

func (e extRanges) Has(n protoreflect.FieldNumber) bool {
	for _, r := range e.s {
		if r.GetStart() <= int32(n) && r.GetEnd() > int32(n) {
			return true
		}
	}
	return false
}

type enumDescriptors struct {
	protoreflect.EnumDescriptors
	file   *result
	parent protoreflect.Descriptor
	enums  []*descriptorpb.EnumDescriptorProto
	prefix string
}

func (e *enumDescriptors) Len() int {
	return len(e.enums)
}

func (e *enumDescriptors) Get(i int) protoreflect.EnumDescriptor {
	en := e.enums[i]
	return e.file.asEnumDescriptor(en, e.file, e.parent, i, e.prefix+en.GetName())
}

func (e *enumDescriptors) ByName(s protoreflect.Name) protoreflect.EnumDescriptor {
	for i, en := range e.enums {
		if en.GetName() == string(s) {
			return e.Get(i)
		}
	}
	return nil
}

type enumDescriptor struct {
	protoreflect.EnumDescriptor
	file   *result
	parent protoreflect.Descriptor
	index  int
	proto  *descriptorpb.EnumDescriptorProto
	fqn    string
}

var _ protoreflect.EnumDescriptor = (*enumDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*enumDescriptor)(nil)

func (r *result) asEnumDescriptor(ed *descriptorpb.EnumDescriptorProto, file *result, parent protoreflect.Descriptor, index int, fqn string) *enumDescriptor {
	if ret := r.descriptors[ed]; ret != nil {
		return ret.(*enumDescriptor)
	}
	ret := &enumDescriptor{file: file, parent: parent, index: index, proto: ed, fqn: fqn}
	r.descriptors[ed] = ret
	return ret
}

func (e *enumDescriptor) EnumDescriptorProto() *descriptorpb.EnumDescriptorProto {
	return e.proto
}

func (e *enumDescriptor) AsProto() proto.Message {
	return e.proto
}

func (e *enumDescriptor) ParentFile() protoreflect.FileDescriptor {
	return e.file
}

func (e *enumDescriptor) Parent() protoreflect.Descriptor {
	return e.parent
}

func (e *enumDescriptor) Index() int {
	return e.index
}

func (e *enumDescriptor) Syntax() protoreflect.Syntax {
	return e.file.Syntax()
}

func (e *enumDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(e.proto.GetName())
}

func (e *enumDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(e.fqn)
}

func (e *enumDescriptor) IsPlaceholder() bool {
	return false
}

func (e *enumDescriptor) Options() protoreflect.ProtoMessage {
	return e.proto.Options
}

func (e *enumDescriptor) Values() protoreflect.EnumValueDescriptors {
	// Unlike all other elements, the fully-qualified name of enum values
	// is NOT scoped to their parent element (the enum), but rather to
	// the enum's parent element. This follows C++ scoping rules for
	// enum values.
	prefix := strings.TrimSuffix(e.fqn, e.proto.GetName())
	return &enValDescriptors{file: e.file, parent: e, vals: e.proto.GetValue(), prefix: prefix}
}

func (e *enumDescriptor) ReservedNames() protoreflect.Names {
	return names{s: e.proto.ReservedName}
}

func (e *enumDescriptor) ReservedRanges() protoreflect.EnumRanges {
	return enumRanges{s: e.proto.ReservedRange}
}

type enumRanges struct {
	protoreflect.EnumRanges
	s []*descriptorpb.EnumDescriptorProto_EnumReservedRange
}

func (e enumRanges) Len() int {
	return len(e.s)
}

func (e enumRanges) Get(i int) [2]protoreflect.EnumNumber {
	r := e.s[i]
	return [2]protoreflect.EnumNumber{
		protoreflect.EnumNumber(r.GetStart()),
		protoreflect.EnumNumber(r.GetEnd()),
	}
}

func (e enumRanges) Has(n protoreflect.EnumNumber) bool {
	for _, r := range e.s {
		if r.GetStart() <= int32(n) && r.GetEnd() >= int32(n) {
			return true
		}
	}
	return false
}

type enValDescriptors struct {
	protoreflect.EnumValueDescriptors
	file   *result
	parent *enumDescriptor
	vals   []*descriptorpb.EnumValueDescriptorProto
	prefix string
}

func (e *enValDescriptors) Len() int {
	return len(e.vals)
}

func (e *enValDescriptors) Get(i int) protoreflect.EnumValueDescriptor {
	val := e.vals[i]
	return e.file.asEnumValueDescriptor(val, e.file, e.parent, i, e.prefix+val.GetName())
}

func (e *enValDescriptors) ByName(s protoreflect.Name) protoreflect.EnumValueDescriptor {
	for i, en := range e.vals {
		if en.GetName() == string(s) {
			return e.Get(i)
		}
	}
	return nil
}

func (e *enValDescriptors) ByNumber(n protoreflect.EnumNumber) protoreflect.EnumValueDescriptor {
	for i, en := range e.vals {
		if en.GetNumber() == int32(n) {
			return e.Get(i)
		}
	}
	return nil
}

type enValDescriptor struct {
	protoreflect.EnumValueDescriptor
	file   *result
	parent *enumDescriptor
	index  int
	proto  *descriptorpb.EnumValueDescriptorProto
	fqn    string
}

var _ protoreflect.EnumValueDescriptor = (*enValDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*enValDescriptor)(nil)

func (r *result) asEnumValueDescriptor(ed *descriptorpb.EnumValueDescriptorProto, file *result, parent *enumDescriptor, index int, fqn string) *enValDescriptor {
	if ret := r.descriptors[ed]; ret != nil {
		return ret.(*enValDescriptor)
	}
	ret := &enValDescriptor{file: file, parent: parent, index: index, proto: ed, fqn: fqn}
	r.descriptors[ed] = ret
	return ret
}

func (e *enValDescriptor) EnumValueDescriptorProto() *descriptorpb.EnumValueDescriptorProto {
	return e.proto
}

func (e *enValDescriptor) AsProto() proto.Message {
	return e.proto
}

func (e *enValDescriptor) ParentFile() protoreflect.FileDescriptor {
	return e.file
}

func (e *enValDescriptor) Parent() protoreflect.Descriptor {
	return e.parent
}

func (e *enValDescriptor) Index() int {
	return e.index
}

func (e *enValDescriptor) Syntax() protoreflect.Syntax {
	return e.file.Syntax()
}

func (e *enValDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(e.proto.GetName())
}

func (e *enValDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(e.fqn)
}

func (e *enValDescriptor) IsPlaceholder() bool {
	return false
}

func (e *enValDescriptor) Options() protoreflect.ProtoMessage {
	return e.proto.Options
}

func (e *enValDescriptor) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(e.proto.GetNumber())
}

type extDescriptors struct {
	protoreflect.ExtensionDescriptors
	file   *result
	parent protoreflect.Descriptor
	exts   []*descriptorpb.FieldDescriptorProto
	prefix string
}

func (e *extDescriptors) Len() int {
	return len(e.exts)
}

func (e *extDescriptors) Get(i int) protoreflect.ExtensionDescriptor {
	fld := e.exts[i]
	fd := e.file.asFieldDescriptor(fld, e.file, e.parent, i, e.prefix+fld.GetName())
	// extensions are expected to implement ExtensionTypeDescriptor, not just ExtensionDescriptor
	return extTypeDescriptor{ExtensionTypeDescriptor: dynamicpb.NewExtensionType(fd).TypeDescriptor(), fd: fd}
}

func (e *extDescriptors) ByName(s protoreflect.Name) protoreflect.ExtensionDescriptor {
	for i, ext := range e.exts {
		if ext.GetName() == string(s) {
			return e.Get(i)
		}
	}
	return nil
}

type extTypeDescriptor struct {
	protoreflect.ExtensionTypeDescriptor
	fd *fldDescriptor
}

var _ protoutil.DescriptorProtoWrapper = extTypeDescriptor{}

func (e extTypeDescriptor) FieldDescriptorProto() *descriptorpb.FieldDescriptorProto {
	return e.fd.proto
}

func (e extTypeDescriptor) AsProto() proto.Message {
	return e.fd.proto
}

type fldDescriptors struct {
	protoreflect.FieldDescriptors
	file   *result
	parent protoreflect.Descriptor
	fields []*descriptorpb.FieldDescriptorProto
	prefix string
}

func (f *fldDescriptors) Len() int {
	return len(f.fields)
}

func (f *fldDescriptors) Get(i int) protoreflect.FieldDescriptor {
	fld := f.fields[i]
	return f.file.asFieldDescriptor(fld, f.file, f.parent, i, f.prefix+fld.GetName())
}

func (f *fldDescriptors) ByName(s protoreflect.Name) protoreflect.FieldDescriptor {
	for i, fld := range f.fields {
		if fld.GetName() == string(s) {
			return f.Get(i)
		}
	}
	return nil
}

func (f *fldDescriptors) ByJSONName(s string) protoreflect.FieldDescriptor {
	for i, fld := range f.fields {
		if fld.GetJsonName() == s {
			return f.Get(i)
		}
	}
	return nil
}

func (f *fldDescriptors) ByTextName(s string) protoreflect.FieldDescriptor {
	return f.ByName(protoreflect.Name(s))
}

func (f *fldDescriptors) ByNumber(n protoreflect.FieldNumber) protoreflect.FieldDescriptor {
	for i, fld := range f.fields {
		if fld.GetNumber() == int32(n) {
			return f.Get(i)
		}
	}
	return nil
}

type fldDescriptor struct {
	protoreflect.FieldDescriptor
	file   *result
	parent protoreflect.Descriptor
	index  int
	proto  *descriptorpb.FieldDescriptorProto
	fqn    string
}

var _ protoreflect.FieldDescriptor = (*fldDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*fldDescriptor)(nil)

func (r *result) asFieldDescriptor(fd *descriptorpb.FieldDescriptorProto, file *result, parent protoreflect.Descriptor, index int, fqn string) *fldDescriptor {
	if ret := r.descriptors[fd]; ret != nil {
		return ret.(*fldDescriptor)
	}
	ret := &fldDescriptor{file: file, parent: parent, index: index, proto: fd, fqn: fqn}
	r.descriptors[fd] = ret
	return ret
}

func (f *fldDescriptor) FieldDescriptorProto() *descriptorpb.FieldDescriptorProto {
	return f.proto
}

func (f *fldDescriptor) AsProto() proto.Message {
	return f.proto
}

func (f *fldDescriptor) ParentFile() protoreflect.FileDescriptor {
	return f.file
}

func (f *fldDescriptor) Parent() protoreflect.Descriptor {
	return f.parent
}

func (f *fldDescriptor) Index() int {
	return f.index
}

func (f *fldDescriptor) Syntax() protoreflect.Syntax {
	return f.file.Syntax()
}

func (f *fldDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(f.proto.GetName())
}

func (f *fldDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(f.fqn)
}

func (f *fldDescriptor) IsPlaceholder() bool {
	return false
}

func (f *fldDescriptor) Options() protoreflect.ProtoMessage {
	return f.proto.Options
}

func (f *fldDescriptor) Number() protoreflect.FieldNumber {
	return protoreflect.FieldNumber(f.proto.GetNumber())
}

func (f *fldDescriptor) Cardinality() protoreflect.Cardinality {
	switch f.proto.GetLabel() {
	case descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
		return protoreflect.Repeated
	case descriptorpb.FieldDescriptorProto_LABEL_REQUIRED:
		return protoreflect.Required
	case descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL:
		return protoreflect.Optional
	default:
		return 0
	}
}

func (f *fldDescriptor) Kind() protoreflect.Kind {
	return protoreflect.Kind(f.proto.GetType())
}

func (f *fldDescriptor) HasJSONName() bool {
	return f.proto.JsonName != nil
}

func (f *fldDescriptor) JSONName() string {
	if f.IsExtension() {
		return f.TextName()
	} else {
		return f.proto.GetJsonName()
	}
}

func (f *fldDescriptor) TextName() string {
	if f.IsExtension() {
		return fmt.Sprintf("[%s]", f.FullName())
	} else {
		return string(f.Name())
	}
}

func (f *fldDescriptor) HasPresence() bool {
	if f.proto.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	return f.IsExtension() ||
		f.Syntax() == protoreflect.Proto2 ||
		f.Kind() == protoreflect.MessageKind || f.Kind() == protoreflect.GroupKind ||
		f.proto.OneofIndex != nil
}

func (f *fldDescriptor) IsExtension() bool {
	return f.proto.GetExtendee() != ""
}

func (f *fldDescriptor) HasOptionalKeyword() bool {
	if f.proto.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL {
		return false
	}
	if f.proto.GetProto3Optional() {
		// NB: This smells weird to return false here. If the proto3_optional field
		// is set, it's because the keyword WAS present. However, the Go runtime
		// returns false for this case, so we mirror that behavior.
		return !f.IsExtension()
	}
	// If it's optional, but not a proto3 optional, then the keyword is only
	// present for proto2 files, for fields that are not part of a oneof.
	return f.file.Syntax() == protoreflect.Proto2 && f.proto.OneofIndex == nil
}

func (f *fldDescriptor) IsWeak() bool {
	return f.proto.Options.GetWeak()
}

func (f *fldDescriptor) IsPacked() bool {
	return f.proto.Options.GetPacked()
}

func (f *fldDescriptor) IsList() bool {
	if f.proto.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	return !f.isMapEntry()
}

func (f *fldDescriptor) IsMap() bool {
	if f.proto.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	if f.IsExtension() {
		return false
	}
	return f.isMapEntry()
}

func (f *fldDescriptor) isMapEntry() bool {
	if f.proto.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return false
	}
	return f.Message().IsMapEntry()
}

func (f *fldDescriptor) MapKey() protoreflect.FieldDescriptor {
	if !f.IsMap() {
		return nil
	}
	return f.Message().Fields().ByNumber(1)
}

func (f *fldDescriptor) MapValue() protoreflect.FieldDescriptor {
	if !f.IsMap() {
		return nil
	}
	return f.Message().Fields().ByNumber(2)
}

func (f *fldDescriptor) HasDefault() bool {
	return f.proto.DefaultValue != nil
}

func (f *fldDescriptor) Default() protoreflect.Value {
	// We only return a valid value for scalar fields
	if f.proto.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED ||
		f.Kind() == protoreflect.GroupKind || f.Kind() == protoreflect.MessageKind {
		return protoreflect.Value{}
	}

	if f.proto.DefaultValue != nil {
		defVal := f.parseDefaultValue(f.proto.GetDefaultValue())
		if defVal.IsValid() {
			return defVal
		}
		// if we cannot parse a valid value, fall back to zero value below
	}

	// No custom default value, so return the zero value for the type
	switch f.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(0)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(0)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(0)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(0)
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(0)
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(0)
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(false)
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes(nil)
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("")
	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(f.Enum().Values().Get(0).Number())
	case protoreflect.GroupKind, protoreflect.MessageKind:
		return protoreflect.ValueOfMessage(dynamicpb.NewMessage(f.Message()))
	default:
		panic(fmt.Sprintf("unknown kind: %v", f.Kind()))
	}
}

func (f *fldDescriptor) parseDefaultValue(val string) protoreflect.Value {
	switch f.Kind() {
	case protoreflect.EnumKind:
		vd := f.Enum().Values().ByName(protoreflect.Name(val))
		if vd != nil {
			return protoreflect.ValueOfEnum(vd.Number())
		}
		return protoreflect.Value{}
	case protoreflect.BoolKind:
		switch val {
		case "true":
			return protoreflect.ValueOfBool(true)
		case "false":
			return protoreflect.ValueOfBool(false)
		default:
			return protoreflect.Value{}
		}
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte(unescape(val)))
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(val)
	case protoreflect.FloatKind:
		if f, err := strconv.ParseFloat(val, 32); err == nil {
			return protoreflect.ValueOfFloat32(float32(f))
		}
		return protoreflect.Value{}
	case protoreflect.DoubleKind:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return protoreflect.ValueOfFloat64(f)
		}
		return protoreflect.Value{}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if i, err := strconv.ParseInt(val, 10, 32); err == nil {
			return protoreflect.ValueOfInt32(int32(i))
		}
		return protoreflect.Value{}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if i, err := strconv.ParseUint(val, 10, 32); err == nil {
			return protoreflect.ValueOfUint32(uint32(i))
		}
		return protoreflect.Value{}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return protoreflect.ValueOfInt64(i)
		}
		return protoreflect.Value{}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if i, err := strconv.ParseUint(val, 10, 64); err == nil {
			return protoreflect.ValueOfUint64(i)
		}
		return protoreflect.Value{}
	default:
		return protoreflect.Value{}
	}
}

func unescape(s string) string {
	// protoc encodes default values for 'bytes' fields using C escaping,
	// so this function reverses that escaping
	out := make([]byte, 0, len(s))
	var buf [4]byte
	for len(s) > 0 {
		if s[0] != '\\' || len(s) < 2 {
			// not escape sequence, or too short to be well-formed escape
			out = append(out, s[0])
			s = s[1:]
		} else if s[1] == 'x' || s[1] == 'X' {
			n := matchPrefix(s[2:], 2, isHex)
			if n == 0 {
				// bad escape
				out = append(out, s[:2]...)
				s = s[2:]
			} else {
				c, err := strconv.ParseUint(s[2:2+n], 16, 8)
				if err != nil {
					// shouldn't really happen...
					out = append(out, s[:2+n]...)
				} else {
					out = append(out, byte(c))
				}
				s = s[2+n:]
			}
		} else if s[1] >= '0' && s[1] <= '7' {
			n := 1 + matchPrefix(s[2:], 2, isOctal)
			c, err := strconv.ParseUint(s[1:1+n], 8, 8)
			if err != nil || c > 0xff {
				out = append(out, s[:1+n]...)
			} else {
				out = append(out, byte(c))
			}
			s = s[1+n:]
		} else if s[1] == 'u' {
			if len(s) < 6 {
				// bad escape
				out = append(out, s...)
				s = s[len(s):]
			} else {
				c, err := strconv.ParseUint(s[2:6], 16, 16)
				if err != nil {
					// bad escape
					out = append(out, s[:6]...)
				} else {
					w := utf8.EncodeRune(buf[:], rune(c))
					out = append(out, buf[:w]...)
				}
				s = s[6:]
			}
		} else if s[1] == 'U' {
			if len(s) < 10 {
				// bad escape
				out = append(out, s...)
				s = s[len(s):]
			} else {
				c, err := strconv.ParseUint(s[2:10], 16, 32)
				if err != nil || c > 0x10ffff {
					// bad escape
					out = append(out, s[:10]...)
				} else {
					w := utf8.EncodeRune(buf[:], rune(c))
					out = append(out, buf[:w]...)
				}
				s = s[10:]
			}
		} else {
			switch s[1] {
			case 'a':
				out = append(out, '\a')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'v':
				out = append(out, '\v')
			case '\\':
				out = append(out, '\\')
			case '\'':
				out = append(out, '\'')
			case '"':
				out = append(out, '"')
			case '?':
				out = append(out, '?')
			default:
				// invalid escape, just copy it as-is
				out = append(out, s[:2]...)
			}
			s = s[2:]
		}
	}
	return string(out)
}

func isOctal(b byte) bool { return b >= '0' && b <= '7' }
func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
func matchPrefix(s string, limit int, fn func(byte) bool) int {
	l := len(s)
	if l > limit {
		l = limit
	}
	i := 0
	for ; i < l; i++ {
		if !fn(s[i]) {
			return i
		}
	}
	return i
}

func (f *fldDescriptor) DefaultEnumValue() protoreflect.EnumValueDescriptor {
	ed := f.Enum()
	if ed == nil {
		return nil
	}
	if f.proto.DefaultValue != nil {
		if val := ed.Values().ByName(protoreflect.Name(f.proto.GetDefaultValue())); val != nil {
			return val
		}
	}
	// if no default specified in source, return nil
	return nil
}

func (f *fldDescriptor) ContainingOneof() protoreflect.OneofDescriptor {
	if f.IsExtension() {
		return nil
	}
	if f.proto.OneofIndex == nil {
		return nil
	}
	parent := f.parent.(*msgDescriptor)
	index := int(f.proto.GetOneofIndex())
	ood := parent.proto.OneofDecl[index]
	fqn := parent.fqn + "." + ood.GetName()
	return f.file.asOneOfDescriptor(ood, f.file, parent, index, fqn)
}

func (f *fldDescriptor) ContainingMessage() protoreflect.MessageDescriptor {
	if !f.IsExtension() {
		return f.parent.(*msgDescriptor)
	}
	return f.file.ResolveMessageType(protoreflect.FullName(f.proto.GetExtendee()))
}

func (f *fldDescriptor) Enum() protoreflect.EnumDescriptor {
	if f.proto.GetType() != descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		return nil
	}
	return f.file.ResolveEnumType(protoreflect.FullName(f.proto.GetTypeName()))
}

func (f *fldDescriptor) Message() protoreflect.MessageDescriptor {
	if f.proto.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE &&
		f.proto.GetType() != descriptorpb.FieldDescriptorProto_TYPE_GROUP {
		return nil
	}
	return f.file.ResolveMessageType(protoreflect.FullName(f.proto.GetTypeName()))
}

type oneofDescriptors struct {
	protoreflect.OneofDescriptors
	file   *result
	parent *msgDescriptor
	oneofs []*descriptorpb.OneofDescriptorProto
	prefix string
}

func (o *oneofDescriptors) Len() int {
	return len(o.oneofs)
}

func (o *oneofDescriptors) Get(i int) protoreflect.OneofDescriptor {
	oo := o.oneofs[i]
	return o.file.asOneOfDescriptor(oo, o.file, o.parent, i, o.prefix+oo.GetName())
}

func (o *oneofDescriptors) ByName(s protoreflect.Name) protoreflect.OneofDescriptor {
	for i, oo := range o.oneofs {
		if oo.GetName() == string(s) {
			return o.Get(i)
		}
	}
	return nil
}

type oneofDescriptor struct {
	protoreflect.OneofDescriptor
	file   *result
	parent *msgDescriptor
	index  int
	proto  *descriptorpb.OneofDescriptorProto
	fqn    string
}

var _ protoreflect.OneofDescriptor = (*oneofDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*oneofDescriptor)(nil)

func (r *result) asOneOfDescriptor(ood *descriptorpb.OneofDescriptorProto, file *result, parent *msgDescriptor, index int, fqn string) *oneofDescriptor {
	if ret := r.descriptors[ood]; ret != nil {
		return ret.(*oneofDescriptor)
	}
	ret := &oneofDescriptor{file: file, parent: parent, index: index, proto: ood, fqn: fqn}
	r.descriptors[ood] = ret
	return ret
}

func (o *oneofDescriptor) OneOfDescriptorProto() *descriptorpb.OneofDescriptorProto {
	return o.proto
}

func (o *oneofDescriptor) AsProto() proto.Message {
	return o.proto
}

func (o *oneofDescriptor) ParentFile() protoreflect.FileDescriptor {
	return o.file
}

func (o *oneofDescriptor) Parent() protoreflect.Descriptor {
	return o.parent
}

func (o *oneofDescriptor) Index() int {
	return o.index
}

func (o *oneofDescriptor) Syntax() protoreflect.Syntax {
	return o.file.Syntax()
}

func (o *oneofDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(o.proto.GetName())
}

func (o *oneofDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(o.fqn)
}

func (o *oneofDescriptor) IsPlaceholder() bool {
	return false
}

func (o *oneofDescriptor) Options() protoreflect.ProtoMessage {
	return o.proto.Options
}

func (o *oneofDescriptor) IsSynthetic() bool {
	for _, fld := range o.parent.proto.GetField() {
		if fld.OneofIndex != nil && int(fld.GetOneofIndex()) == o.index {
			return fld.GetProto3Optional()
		}
	}
	return false // NB: we should never get here
}

func (o *oneofDescriptor) Fields() protoreflect.FieldDescriptors {
	var fields []*descriptorpb.FieldDescriptorProto
	for _, fld := range o.parent.proto.GetField() {
		if fld.OneofIndex != nil && int(fld.GetOneofIndex()) == o.index {
			fields = append(fields, fld)
		}
	}
	return &fldDescriptors{file: o.file, parent: o.parent, fields: fields, prefix: o.parent.fqn + "."}
}

type svcDescriptors struct {
	protoreflect.ServiceDescriptors
	file   *result
	svcs   []*descriptorpb.ServiceDescriptorProto
	prefix string
}

func (s *svcDescriptors) Len() int {
	return len(s.svcs)
}

func (s *svcDescriptors) Get(i int) protoreflect.ServiceDescriptor {
	svc := s.svcs[i]
	return s.file.asServiceDescriptor(svc, s.file, i, s.prefix+svc.GetName())
}

func (s *svcDescriptors) ByName(n protoreflect.Name) protoreflect.ServiceDescriptor {
	for i, svc := range s.svcs {
		if svc.GetName() == string(n) {
			return s.Get(i)
		}
	}
	return nil
}

type svcDescriptor struct {
	protoreflect.ServiceDescriptor
	file  *result
	index int
	proto *descriptorpb.ServiceDescriptorProto
	fqn   string
}

var _ protoreflect.ServiceDescriptor = (*svcDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*svcDescriptor)(nil)

func (r *result) asServiceDescriptor(sd *descriptorpb.ServiceDescriptorProto, file *result, index int, fqn string) *svcDescriptor {
	if ret := r.descriptors[sd]; ret != nil {
		return ret.(*svcDescriptor)
	}
	ret := &svcDescriptor{file: file, index: index, proto: sd, fqn: fqn}
	r.descriptors[sd] = ret
	return ret
}

func (s *svcDescriptor) ServiceDescriptorProto() *descriptorpb.ServiceDescriptorProto {
	return s.proto
}

func (s *svcDescriptor) AsProto() proto.Message {
	return s.proto
}

func (s *svcDescriptor) ParentFile() protoreflect.FileDescriptor {
	return s.file
}

func (s *svcDescriptor) Parent() protoreflect.Descriptor {
	return s.file
}

func (s *svcDescriptor) Index() int {
	return s.index
}

func (s *svcDescriptor) Syntax() protoreflect.Syntax {
	return s.file.Syntax()
}

func (s *svcDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(s.proto.GetName())
}

func (s *svcDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(s.fqn)
}

func (s *svcDescriptor) IsPlaceholder() bool {
	return false
}

func (s *svcDescriptor) Options() protoreflect.ProtoMessage {
	return s.proto.Options
}

func (s *svcDescriptor) Methods() protoreflect.MethodDescriptors {
	return &mtdDescriptors{file: s.file, parent: s, mtds: s.proto.GetMethod(), prefix: s.fqn + "."}
}

type mtdDescriptors struct {
	protoreflect.MethodDescriptors
	file   *result
	parent *svcDescriptor
	mtds   []*descriptorpb.MethodDescriptorProto
	prefix string
}

func (m *mtdDescriptors) Len() int {
	return len(m.mtds)
}

func (m *mtdDescriptors) Get(i int) protoreflect.MethodDescriptor {
	mtd := m.mtds[i]
	return m.file.asMethodDescriptor(mtd, m.file, m.parent, i, m.prefix+mtd.GetName())
}

func (m *mtdDescriptors) ByName(n protoreflect.Name) protoreflect.MethodDescriptor {
	for i, svc := range m.mtds {
		if svc.GetName() == string(n) {
			return m.Get(i)
		}
	}
	return nil
}

type mtdDescriptor struct {
	protoreflect.MethodDescriptor
	file   *result
	parent *svcDescriptor
	index  int
	proto  *descriptorpb.MethodDescriptorProto
	fqn    string
}

var _ protoreflect.MethodDescriptor = (*mtdDescriptor)(nil)
var _ protoutil.DescriptorProtoWrapper = (*mtdDescriptor)(nil)

func (r *result) asMethodDescriptor(mtd *descriptorpb.MethodDescriptorProto, file *result, parent *svcDescriptor, index int, fqn string) *mtdDescriptor {
	if ret := r.descriptors[mtd]; ret != nil {
		return ret.(*mtdDescriptor)
	}
	ret := &mtdDescriptor{file: file, parent: parent, index: index, proto: mtd, fqn: fqn}
	r.descriptors[mtd] = ret
	return ret
}

func (m *mtdDescriptor) MethodDescriptorProto() *descriptorpb.MethodDescriptorProto {
	return m.proto
}

func (m *mtdDescriptor) AsProto() proto.Message {
	return m.proto
}

func (m *mtdDescriptor) ParentFile() protoreflect.FileDescriptor {
	return m.file
}

func (m *mtdDescriptor) Parent() protoreflect.Descriptor {
	return m.parent
}

func (m *mtdDescriptor) Index() int {
	return m.index
}

func (m *mtdDescriptor) Syntax() protoreflect.Syntax {
	return m.file.Syntax()
}

func (m *mtdDescriptor) Name() protoreflect.Name {
	return protoreflect.Name(m.proto.GetName())
}

func (m *mtdDescriptor) FullName() protoreflect.FullName {
	return protoreflect.FullName(m.fqn)
}

func (m *mtdDescriptor) IsPlaceholder() bool {
	return false
}

func (m *mtdDescriptor) Options() protoreflect.ProtoMessage {
	return m.proto.Options
}

func (m *mtdDescriptor) Input() protoreflect.MessageDescriptor {
	return m.file.ResolveMessageType(protoreflect.FullName(m.proto.GetInputType()))
}

func (m *mtdDescriptor) Output() protoreflect.MessageDescriptor {
	return m.file.ResolveMessageType(protoreflect.FullName(m.proto.GetOutputType()))
}

func (m *mtdDescriptor) IsStreamingClient() bool {
	return m.proto.GetClientStreaming()
}

func (m *mtdDescriptor) IsStreamingServer() bool {
	return m.proto.GetServerStreaming()
}

func (r *result) FindImportByPath(path string) File {
	return r.deps.FindFileByPath(path)
}

func (r *result) FindExtensionByNumber(msg protoreflect.FullName, tag protoreflect.FieldNumber) protoreflect.ExtensionTypeDescriptor {
	return findExtension(r, msg, tag)
}

func (r *result) FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor {
	fqn := strings.TrimPrefix(string(name), ".")
	d := r.descriptorPool[string(name)]
	if d == nil {
		return nil
	}
	return r.toDescriptor(fqn, d)
}

func (r *result) importsAsFiles() Files {
	return r.deps
}
