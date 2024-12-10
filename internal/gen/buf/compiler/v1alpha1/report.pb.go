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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.2
// 	protoc        (unknown)
// source: buf/compiler/v1alpha1/report.proto

package compilerv1alpha1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// A diagnostic level. This affects how (and whether) it is shown to users.
type Diagnostic_Level int32

const (
	Diagnostic_LEVEL_UNSPECIFIED Diagnostic_Level = 0
	Diagnostic_LEVEL_ERROR       Diagnostic_Level = 1
	Diagnostic_LEVEL_WARNING     Diagnostic_Level = 2
	Diagnostic_LEVEL_REMARK      Diagnostic_Level = 3
)

// Enum value maps for Diagnostic_Level.
var (
	Diagnostic_Level_name = map[int32]string{
		0: "LEVEL_UNSPECIFIED",
		1: "LEVEL_ERROR",
		2: "LEVEL_WARNING",
		3: "LEVEL_REMARK",
	}
	Diagnostic_Level_value = map[string]int32{
		"LEVEL_UNSPECIFIED": 0,
		"LEVEL_ERROR":       1,
		"LEVEL_WARNING":     2,
		"LEVEL_REMARK":      3,
	}
)

func (x Diagnostic_Level) Enum() *Diagnostic_Level {
	p := new(Diagnostic_Level)
	*p = x
	return p
}

func (x Diagnostic_Level) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Diagnostic_Level) Descriptor() protoreflect.EnumDescriptor {
	return file_buf_compiler_v1alpha1_report_proto_enumTypes[0].Descriptor()
}

func (Diagnostic_Level) Type() protoreflect.EnumType {
	return &file_buf_compiler_v1alpha1_report_proto_enumTypes[0]
}

func (x Diagnostic_Level) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Diagnostic_Level.Descriptor instead.
func (Diagnostic_Level) EnumDescriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_report_proto_rawDescGZIP(), []int{1, 0}
}

// A diagnostic report, consisting of `Diagnostics` and the `File`s they diagnose.
type Report struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Files       []*Report_File `protobuf:"bytes,1,rep,name=files,proto3" json:"files,omitempty"`
	Diagnostics []*Diagnostic  `protobuf:"bytes,2,rep,name=diagnostics,proto3" json:"diagnostics,omitempty"`
}

func (x *Report) Reset() {
	*x = Report{}
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Report) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Report) ProtoMessage() {}

func (x *Report) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Report.ProtoReflect.Descriptor instead.
func (*Report) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_report_proto_rawDescGZIP(), []int{0}
}

func (x *Report) GetFiles() []*Report_File {
	if x != nil {
		return x.Files
	}
	return nil
}

func (x *Report) GetDiagnostics() []*Diagnostic {
	if x != nil {
		return x.Diagnostics
	}
	return nil
}

// A diagnostic within a `Report`.
type Diagnostic struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The message to show for this diagnostic. This should fit on one line.
	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	// An optional machine-readable tag for the diagnostic.
	Tag string `protobuf:"bytes,8,opt,name=tag,proto3" json:"tag,omitempty"`
	// Required. The level for this diagnostic.
	Level Diagnostic_Level `protobuf:"varint,2,opt,name=level,proto3,enum=buf.compiler.v1alpha1.Diagnostic_Level" json:"level,omitempty"`
	// An optional path to show in the diagnostic, if it has no annotations.
	// This is useful for e.g. diagnostics that would have no spans.
	InFile string `protobuf:"bytes,3,opt,name=in_file,json=inFile,proto3" json:"in_file,omitempty"`
	// Annotations for source code relevant to this diagnostic.
	Annotations []*Diagnostic_Annotation `protobuf:"bytes,4,rep,name=annotations,proto3" json:"annotations,omitempty"`
	// Notes about the error to show to the user. May span multiple lines.
	Notes []string `protobuf:"bytes,5,rep,name=notes,proto3" json:"notes,omitempty"`
	// Helpful suggestions to the user.
	Help []string `protobuf:"bytes,6,rep,name=help,proto3" json:"help,omitempty"`
	// Debugging information related to the diagnostic. This should only be
	// used for information about debugging a tool or compiler that emits the
	// diagnostic, not the code being diagnosed.
	Debug []string `protobuf:"bytes,7,rep,name=debug,proto3" json:"debug,omitempty"`
}

func (x *Diagnostic) Reset() {
	*x = Diagnostic{}
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Diagnostic) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Diagnostic) ProtoMessage() {}

func (x *Diagnostic) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Diagnostic.ProtoReflect.Descriptor instead.
func (*Diagnostic) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_report_proto_rawDescGZIP(), []int{1}
}

func (x *Diagnostic) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *Diagnostic) GetTag() string {
	if x != nil {
		return x.Tag
	}
	return ""
}

func (x *Diagnostic) GetLevel() Diagnostic_Level {
	if x != nil {
		return x.Level
	}
	return Diagnostic_LEVEL_UNSPECIFIED
}

func (x *Diagnostic) GetInFile() string {
	if x != nil {
		return x.InFile
	}
	return ""
}

func (x *Diagnostic) GetAnnotations() []*Diagnostic_Annotation {
	if x != nil {
		return x.Annotations
	}
	return nil
}

func (x *Diagnostic) GetNotes() []string {
	if x != nil {
		return x.Notes
	}
	return nil
}

func (x *Diagnostic) GetHelp() []string {
	if x != nil {
		return x.Help
	}
	return nil
}

func (x *Diagnostic) GetDebug() []string {
	if x != nil {
		return x.Debug
	}
	return nil
}

// A file involved in a diagnostic `Report`.
type Report_File struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The path to this file. Does not need to be meaningful as a file-system
	// path.
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	// The textual contents of this file. Presumed to be UTF-8, although it need
	// not be.
	Text []byte `protobuf:"bytes,2,opt,name=text,proto3" json:"text,omitempty"`
}

func (x *Report_File) Reset() {
	*x = Report_File{}
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Report_File) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Report_File) ProtoMessage() {}

func (x *Report_File) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Report_File.ProtoReflect.Descriptor instead.
func (*Report_File) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_report_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Report_File) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *Report_File) GetText() []byte {
	if x != nil {
		return x.Text
	}
	return nil
}

// A file annotation within a `Diagnostic`. This corresponds to a single
// span of source code in a `Report`'s file.
type Diagnostic_Annotation struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// A message to show under this snippet. May be empty.
	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	// Whether this is a "primary" snippet, which is used for deciding whether or not
	// to mark the snippet with the same color as the overall diagnostic.
	Primary bool `protobuf:"varint,2,opt,name=primary,proto3" json:"primary,omitempty"`
	// The index of `Report.files` of the file this annotation is for.
	//
	// This is not a whole `Report.File` to help keep serialized reports slim. This
	// avoids neeidng to duplicate the whole text of the file one for every annotation.
	File uint32 `protobuf:"varint,3,opt,name=file,proto3" json:"file,omitempty"`
	// The start offset of the annotated snippet, in bytes.
	Start uint32 `protobuf:"varint,4,opt,name=start,proto3" json:"start,omitempty"`
	// The end offset of the annotated snippet, in bytes.
	End uint32 `protobuf:"varint,5,opt,name=end,proto3" json:"end,omitempty"`
}

func (x *Diagnostic_Annotation) Reset() {
	*x = Diagnostic_Annotation{}
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Diagnostic_Annotation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Diagnostic_Annotation) ProtoMessage() {}

func (x *Diagnostic_Annotation) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_report_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Diagnostic_Annotation.ProtoReflect.Descriptor instead.
func (*Diagnostic_Annotation) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_report_proto_rawDescGZIP(), []int{1, 0}
}

func (x *Diagnostic_Annotation) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *Diagnostic_Annotation) GetPrimary() bool {
	if x != nil {
		return x.Primary
	}
	return false
}

func (x *Diagnostic_Annotation) GetFile() uint32 {
	if x != nil {
		return x.File
	}
	return 0
}

func (x *Diagnostic_Annotation) GetStart() uint32 {
	if x != nil {
		return x.Start
	}
	return 0
}

func (x *Diagnostic_Annotation) GetEnd() uint32 {
	if x != nil {
		return x.End
	}
	return 0
}

var File_buf_compiler_v1alpha1_report_proto protoreflect.FileDescriptor

var file_buf_compiler_v1alpha1_report_proto_rawDesc = []byte{
	0x0a, 0x22, 0x62, 0x75, 0x66, 0x2f, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x2f, 0x76,
	0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2f, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x15, 0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c,
	0x65, 0x72, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x22, 0xb7, 0x01, 0x0a, 0x06,
	0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x38, 0x0a, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x18,
	0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70,
	0x69, 0x6c, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x52, 0x65,
	0x70, 0x6f, 0x72, 0x74, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x52, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73,
	0x12, 0x43, 0x0a, 0x0b, 0x64, 0x69, 0x61, 0x67, 0x6e, 0x6f, 0x73, 0x74, 0x69, 0x63, 0x73, 0x18,
	0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x21, 0x2e, 0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70,
	0x69, 0x6c, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x44, 0x69,
	0x61, 0x67, 0x6e, 0x6f, 0x73, 0x74, 0x69, 0x63, 0x52, 0x0b, 0x64, 0x69, 0x61, 0x67, 0x6e, 0x6f,
	0x73, 0x74, 0x69, 0x63, 0x73, 0x1a, 0x2e, 0x0a, 0x04, 0x46, 0x69, 0x6c, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74,
	0x68, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x65, 0x78, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x04, 0x74, 0x65, 0x78, 0x74, 0x22, 0xf4, 0x03, 0x0a, 0x0a, 0x44, 0x69, 0x61, 0x67, 0x6e, 0x6f,
	0x73, 0x74, 0x69, 0x63, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x10,
	0x0a, 0x03, 0x74, 0x61, 0x67, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x74, 0x61, 0x67,
	0x12, 0x3d, 0x0a, 0x05, 0x6c, 0x65, 0x76, 0x65, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32,
	0x27, 0x2e, 0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x2e, 0x76,
	0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x44, 0x69, 0x61, 0x67, 0x6e, 0x6f, 0x73, 0x74,
	0x69, 0x63, 0x2e, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x52, 0x05, 0x6c, 0x65, 0x76, 0x65, 0x6c, 0x12,
	0x17, 0x0a, 0x07, 0x69, 0x6e, 0x5f, 0x66, 0x69, 0x6c, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x06, 0x69, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x12, 0x4e, 0x0a, 0x0b, 0x61, 0x6e, 0x6e, 0x6f,
	0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2c, 0x2e,
	0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x61,
	0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x44, 0x69, 0x61, 0x67, 0x6e, 0x6f, 0x73, 0x74, 0x69, 0x63,
	0x2e, 0x41, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0b, 0x61, 0x6e, 0x6e,
	0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x14, 0x0a, 0x05, 0x6e, 0x6f, 0x74, 0x65,
	0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x6f, 0x74, 0x65, 0x73, 0x12, 0x12,
	0x0a, 0x04, 0x68, 0x65, 0x6c, 0x70, 0x18, 0x06, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04, 0x68, 0x65,
	0x6c, 0x70, 0x12, 0x14, 0x0a, 0x05, 0x64, 0x65, 0x62, 0x75, 0x67, 0x18, 0x07, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x05, 0x64, 0x65, 0x62, 0x75, 0x67, 0x1a, 0x7c, 0x0a, 0x0a, 0x41, 0x6e, 0x6e, 0x6f,
	0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x69, 0x6d, 0x61, 0x72, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x07, 0x70, 0x72, 0x69, 0x6d, 0x61, 0x72, 0x79, 0x12, 0x12, 0x0a, 0x04, 0x66, 0x69,
	0x6c, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x66, 0x69, 0x6c, 0x65, 0x12, 0x14,
	0x0a, 0x05, 0x73, 0x74, 0x61, 0x72, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05, 0x73,
	0x74, 0x61, 0x72, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x65, 0x6e, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x03, 0x65, 0x6e, 0x64, 0x22, 0x54, 0x0a, 0x05, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x12,
	0x15, 0x0a, 0x11, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49,
	0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x0f, 0x0a, 0x0b, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f,
	0x45, 0x52, 0x52, 0x4f, 0x52, 0x10, 0x01, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x45, 0x56, 0x45, 0x4c,
	0x5f, 0x57, 0x41, 0x52, 0x4e, 0x49, 0x4e, 0x47, 0x10, 0x02, 0x12, 0x10, 0x0a, 0x0c, 0x4c, 0x45,
	0x56, 0x45, 0x4c, 0x5f, 0x52, 0x45, 0x4d, 0x41, 0x52, 0x4b, 0x10, 0x03, 0x42, 0xf4, 0x01, 0x0a,
	0x19, 0x63, 0x6f, 0x6d, 0x2e, 0x62, 0x75, 0x66, 0x2e, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65,
	0x72, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x42, 0x0b, 0x52, 0x65, 0x70, 0x6f,
	0x72, 0x74, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x54, 0x67, 0x69, 0x74, 0x68, 0x75,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x62, 0x75, 0x66, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x62, 0x75, 0x66, 0x2f, 0x63, 0x6f, 0x6d,
	0x70, 0x69, 0x6c, 0x65, 0x72, 0x2f, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x3b, 0x63,
	0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0xa2,
	0x02, 0x03, 0x42, 0x43, 0x58, 0xaa, 0x02, 0x15, 0x42, 0x75, 0x66, 0x2e, 0x43, 0x6f, 0x6d, 0x70,
	0x69, 0x6c, 0x65, 0x72, 0x2e, 0x56, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0xca, 0x02, 0x15,
	0x42, 0x75, 0x66, 0x5c, 0x43, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x5c, 0x56, 0x31, 0x61,
	0x6c, 0x70, 0x68, 0x61, 0x31, 0xe2, 0x02, 0x21, 0x42, 0x75, 0x66, 0x5c, 0x43, 0x6f, 0x6d, 0x70,
	0x69, 0x6c, 0x65, 0x72, 0x5c, 0x56, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x5c, 0x47, 0x50,
	0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x17, 0x42, 0x75, 0x66, 0x3a,
	0x3a, 0x43, 0x6f, 0x6d, 0x70, 0x69, 0x6c, 0x65, 0x72, 0x3a, 0x3a, 0x56, 0x31, 0x61, 0x6c, 0x70,
	0x68, 0x61, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_buf_compiler_v1alpha1_report_proto_rawDescOnce sync.Once
	file_buf_compiler_v1alpha1_report_proto_rawDescData = file_buf_compiler_v1alpha1_report_proto_rawDesc
)

func file_buf_compiler_v1alpha1_report_proto_rawDescGZIP() []byte {
	file_buf_compiler_v1alpha1_report_proto_rawDescOnce.Do(func() {
		file_buf_compiler_v1alpha1_report_proto_rawDescData = protoimpl.X.CompressGZIP(file_buf_compiler_v1alpha1_report_proto_rawDescData)
	})
	return file_buf_compiler_v1alpha1_report_proto_rawDescData
}

var file_buf_compiler_v1alpha1_report_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_buf_compiler_v1alpha1_report_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_buf_compiler_v1alpha1_report_proto_goTypes = []any{
	(Diagnostic_Level)(0),         // 0: buf.compiler.v1alpha1.Diagnostic.Level
	(*Report)(nil),                // 1: buf.compiler.v1alpha1.Report
	(*Diagnostic)(nil),            // 2: buf.compiler.v1alpha1.Diagnostic
	(*Report_File)(nil),           // 3: buf.compiler.v1alpha1.Report.File
	(*Diagnostic_Annotation)(nil), // 4: buf.compiler.v1alpha1.Diagnostic.Annotation
}
var file_buf_compiler_v1alpha1_report_proto_depIdxs = []int32{
	3, // 0: buf.compiler.v1alpha1.Report.files:type_name -> buf.compiler.v1alpha1.Report.File
	2, // 1: buf.compiler.v1alpha1.Report.diagnostics:type_name -> buf.compiler.v1alpha1.Diagnostic
	0, // 2: buf.compiler.v1alpha1.Diagnostic.level:type_name -> buf.compiler.v1alpha1.Diagnostic.Level
	4, // 3: buf.compiler.v1alpha1.Diagnostic.annotations:type_name -> buf.compiler.v1alpha1.Diagnostic.Annotation
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_buf_compiler_v1alpha1_report_proto_init() }
func file_buf_compiler_v1alpha1_report_proto_init() {
	if File_buf_compiler_v1alpha1_report_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_buf_compiler_v1alpha1_report_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_buf_compiler_v1alpha1_report_proto_goTypes,
		DependencyIndexes: file_buf_compiler_v1alpha1_report_proto_depIdxs,
		EnumInfos:         file_buf_compiler_v1alpha1_report_proto_enumTypes,
		MessageInfos:      file_buf_compiler_v1alpha1_report_proto_msgTypes,
	}.Build()
	File_buf_compiler_v1alpha1_report_proto = out.File
	file_buf_compiler_v1alpha1_report_proto_rawDesc = nil
	file_buf_compiler_v1alpha1_report_proto_goTypes = nil
	file_buf_compiler_v1alpha1_report_proto_depIdxs = nil
}
