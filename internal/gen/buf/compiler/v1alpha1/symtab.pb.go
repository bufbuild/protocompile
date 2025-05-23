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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        (unknown)
// source: buf/compiler/v1alpha1/symtab.proto

package compilerv1alpha1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Symbol_Kind int32

const (
	// Numbers synced with those in symbol_kind.go
	Symbol_KIND_UNSPECIFIED Symbol_Kind = 0
	Symbol_KIND_PACKAGE     Symbol_Kind = 1
	Symbol_KIND_SCALAR      Symbol_Kind = 2
	Symbol_KIND_MESSAGE     Symbol_Kind = 3
	Symbol_KIND_ENUM        Symbol_Kind = 4
	Symbol_KIND_FIELD       Symbol_Kind = 5
	Symbol_KIND_ENUM_VALUE  Symbol_Kind = 6
	Symbol_KIND_EXTENSION   Symbol_Kind = 7
	Symbol_KIND_ONEOF       Symbol_Kind = 8
)

// Enum value maps for Symbol_Kind.
var (
	Symbol_Kind_name = map[int32]string{
		0: "KIND_UNSPECIFIED",
		1: "KIND_PACKAGE",
		2: "KIND_SCALAR",
		3: "KIND_MESSAGE",
		4: "KIND_ENUM",
		5: "KIND_FIELD",
		6: "KIND_ENUM_VALUE",
		7: "KIND_EXTENSION",
		8: "KIND_ONEOF",
	}
	Symbol_Kind_value = map[string]int32{
		"KIND_UNSPECIFIED": 0,
		"KIND_PACKAGE":     1,
		"KIND_SCALAR":      2,
		"KIND_MESSAGE":     3,
		"KIND_ENUM":        4,
		"KIND_FIELD":       5,
		"KIND_ENUM_VALUE":  6,
		"KIND_EXTENSION":   7,
		"KIND_ONEOF":       8,
	}
)

func (x Symbol_Kind) Enum() *Symbol_Kind {
	p := new(Symbol_Kind)
	*p = x
	return p
}

func (x Symbol_Kind) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Symbol_Kind) Descriptor() protoreflect.EnumDescriptor {
	return file_buf_compiler_v1alpha1_symtab_proto_enumTypes[0].Descriptor()
}

func (Symbol_Kind) Type() protoreflect.EnumType {
	return &file_buf_compiler_v1alpha1_symtab_proto_enumTypes[0]
}

func (x Symbol_Kind) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Symbol_Kind.Descriptor instead.
func (Symbol_Kind) EnumDescriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{3, 0}
}

// A set of symbol tables.
type SymbolSet struct {
	state         protoimpl.MessageState  `protogen:"open.v1"`
	Tables        map[string]*SymbolTable `protobuf:"bytes,1,rep,name=tables,proto3" json:"tables,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SymbolSet) Reset() {
	*x = SymbolSet{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SymbolSet) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SymbolSet) ProtoMessage() {}

func (x *SymbolSet) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SymbolSet.ProtoReflect.Descriptor instead.
func (*SymbolSet) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{0}
}

func (x *SymbolSet) GetTables() map[string]*SymbolTable {
	if x != nil {
		return x.Tables
	}
	return nil
}

// Symbol information for a particular Protobuf file.
type SymbolTable struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Imports       []*Import              `protobuf:"bytes,1,rep,name=imports,proto3" json:"imports,omitempty"`
	Symbols       []*Symbol              `protobuf:"bytes,2,rep,name=symbols,proto3" json:"symbols,omitempty"`
	Options       *Value                 `protobuf:"bytes,3,opt,name=options,proto3" json:"options,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SymbolTable) Reset() {
	*x = SymbolTable{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SymbolTable) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SymbolTable) ProtoMessage() {}

func (x *SymbolTable) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SymbolTable.ProtoReflect.Descriptor instead.
func (*SymbolTable) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{1}
}

func (x *SymbolTable) GetImports() []*Import {
	if x != nil {
		return x.Imports
	}
	return nil
}

func (x *SymbolTable) GetSymbols() []*Symbol {
	if x != nil {
		return x.Symbols
	}
	return nil
}

func (x *SymbolTable) GetOptions() *Value {
	if x != nil {
		return x.Options
	}
	return nil
}

// Metadata associated with a transitive import.
type Import struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Path          string                 `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	Public        bool                   `protobuf:"varint,2,opt,name=public,proto3" json:"public,omitempty"`
	Weak          bool                   `protobuf:"varint,3,opt,name=weak,proto3" json:"weak,omitempty"`
	Transitive    bool                   `protobuf:"varint,4,opt,name=transitive,proto3" json:"transitive,omitempty"`
	Visible       bool                   `protobuf:"varint,5,opt,name=visible,proto3" json:"visible,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Import) Reset() {
	*x = Import{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Import) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Import) ProtoMessage() {}

func (x *Import) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Import.ProtoReflect.Descriptor instead.
func (*Import) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{2}
}

func (x *Import) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *Import) GetPublic() bool {
	if x != nil {
		return x.Public
	}
	return false
}

func (x *Import) GetWeak() bool {
	if x != nil {
		return x.Weak
	}
	return false
}

func (x *Import) GetTransitive() bool {
	if x != nil {
		return x.Transitive
	}
	return false
}

func (x *Import) GetVisible() bool {
	if x != nil {
		return x.Visible
	}
	return false
}

// A symbol in a file.
type Symbol struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	Fqn   string                 `protobuf:"bytes,1,opt,name=fqn,proto3" json:"fqn,omitempty"`
	Kind  Symbol_Kind            `protobuf:"varint,2,opt,name=kind,proto3,enum=buf.compiler.v1alpha1.Symbol_Kind" json:"kind,omitempty"`
	// The file this symbol came from.
	File string `protobuf:"bytes,3,opt,name=file,proto3" json:"file,omitempty"`
	// The index of this kind of entity in that file.
	Index uint32 `protobuf:"varint,4,opt,name=index,proto3" json:"index,omitempty"`
	// Whether this symbol can be validly referenced in the current file.
	Visible       bool   `protobuf:"varint,5,opt,name=visible,proto3" json:"visible,omitempty"`
	Options       *Value `protobuf:"bytes,6,opt,name=options,proto3" json:"options,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Symbol) Reset() {
	*x = Symbol{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Symbol) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Symbol) ProtoMessage() {}

func (x *Symbol) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Symbol.ProtoReflect.Descriptor instead.
func (*Symbol) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{3}
}

func (x *Symbol) GetFqn() string {
	if x != nil {
		return x.Fqn
	}
	return ""
}

func (x *Symbol) GetKind() Symbol_Kind {
	if x != nil {
		return x.Kind
	}
	return Symbol_KIND_UNSPECIFIED
}

func (x *Symbol) GetFile() string {
	if x != nil {
		return x.File
	}
	return ""
}

func (x *Symbol) GetIndex() uint32 {
	if x != nil {
		return x.Index
	}
	return 0
}

func (x *Symbol) GetVisible() bool {
	if x != nil {
		return x.Visible
	}
	return false
}

func (x *Symbol) GetOptions() *Value {
	if x != nil {
		return x.Options
	}
	return nil
}

// An option value attached to a symbol.
type Value struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Value:
	//
	//	*Value_Bool
	//	*Value_Int
	//	*Value_Uint
	//	*Value_Float
	//	*Value_String_
	//	*Value_Repeated_
	//	*Value_Message_
	Value         isValue_Value `protobuf_oneof:"value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Value) Reset() {
	*x = Value{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Value) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Value) ProtoMessage() {}

func (x *Value) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Value.ProtoReflect.Descriptor instead.
func (*Value) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{4}
}

func (x *Value) GetValue() isValue_Value {
	if x != nil {
		return x.Value
	}
	return nil
}

func (x *Value) GetBool() bool {
	if x != nil {
		if x, ok := x.Value.(*Value_Bool); ok {
			return x.Bool
		}
	}
	return false
}

func (x *Value) GetInt() int64 {
	if x != nil {
		if x, ok := x.Value.(*Value_Int); ok {
			return x.Int
		}
	}
	return 0
}

func (x *Value) GetUint() uint64 {
	if x != nil {
		if x, ok := x.Value.(*Value_Uint); ok {
			return x.Uint
		}
	}
	return 0
}

func (x *Value) GetFloat() float64 {
	if x != nil {
		if x, ok := x.Value.(*Value_Float); ok {
			return x.Float
		}
	}
	return 0
}

func (x *Value) GetString_() []byte {
	if x != nil {
		if x, ok := x.Value.(*Value_String_); ok {
			return x.String_
		}
	}
	return nil
}

func (x *Value) GetRepeated() *Value_Repeated {
	if x != nil {
		if x, ok := x.Value.(*Value_Repeated_); ok {
			return x.Repeated
		}
	}
	return nil
}

func (x *Value) GetMessage() *Value_Message {
	if x != nil {
		if x, ok := x.Value.(*Value_Message_); ok {
			return x.Message
		}
	}
	return nil
}

type isValue_Value interface {
	isValue_Value()
}

type Value_Bool struct {
	Bool bool `protobuf:"varint,1,opt,name=bool,proto3,oneof"`
}

type Value_Int struct {
	Int int64 `protobuf:"varint,2,opt,name=int,proto3,oneof"`
}

type Value_Uint struct {
	Uint uint64 `protobuf:"varint,3,opt,name=uint,proto3,oneof"`
}

type Value_Float struct {
	Float float64 `protobuf:"fixed64,4,opt,name=float,proto3,oneof"`
}

type Value_String_ struct {
	String_ []byte `protobuf:"bytes,5,opt,name=string,proto3,oneof"`
}

type Value_Repeated_ struct {
	Repeated *Value_Repeated `protobuf:"bytes,6,opt,name=repeated,proto3,oneof"`
}

type Value_Message_ struct {
	Message *Value_Message `protobuf:"bytes,7,opt,name=message,proto3,oneof"`
}

func (*Value_Bool) isValue_Value() {}

func (*Value_Int) isValue_Value() {}

func (*Value_Uint) isValue_Value() {}

func (*Value_Float) isValue_Value() {}

func (*Value_String_) isValue_Value() {}

func (*Value_Repeated_) isValue_Value() {}

func (*Value_Message_) isValue_Value() {}

type Value_Message struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Fields        map[string]*Value      `protobuf:"bytes,1,rep,name=fields,proto3" json:"fields,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	Extns         map[string]*Value      `protobuf:"bytes,2,rep,name=extns,proto3" json:"extns,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Value_Message) Reset() {
	*x = Value_Message{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Value_Message) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Value_Message) ProtoMessage() {}

func (x *Value_Message) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Value_Message.ProtoReflect.Descriptor instead.
func (*Value_Message) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{4, 0}
}

func (x *Value_Message) GetFields() map[string]*Value {
	if x != nil {
		return x.Fields
	}
	return nil
}

func (x *Value_Message) GetExtns() map[string]*Value {
	if x != nil {
		return x.Extns
	}
	return nil
}

type Value_Repeated struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Values        []*Value               `protobuf:"bytes,1,rep,name=values,proto3" json:"values,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Value_Repeated) Reset() {
	*x = Value_Repeated{}
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Value_Repeated) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Value_Repeated) ProtoMessage() {}

func (x *Value_Repeated) ProtoReflect() protoreflect.Message {
	mi := &file_buf_compiler_v1alpha1_symtab_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Value_Repeated.ProtoReflect.Descriptor instead.
func (*Value_Repeated) Descriptor() ([]byte, []int) {
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP(), []int{4, 1}
}

func (x *Value_Repeated) GetValues() []*Value {
	if x != nil {
		return x.Values
	}
	return nil
}

var File_buf_compiler_v1alpha1_symtab_proto protoreflect.FileDescriptor

const file_buf_compiler_v1alpha1_symtab_proto_rawDesc = "" +
	"\n" +
	"\"buf/compiler/v1alpha1/symtab.proto\x12\x15buf.compiler.v1alpha1\"\xb0\x01\n" +
	"\tSymbolSet\x12D\n" +
	"\x06tables\x18\x01 \x03(\v2,.buf.compiler.v1alpha1.SymbolSet.TablesEntryR\x06tables\x1a]\n" +
	"\vTablesEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x128\n" +
	"\x05value\x18\x02 \x01(\v2\".buf.compiler.v1alpha1.SymbolTableR\x05value:\x028\x01\"\xb7\x01\n" +
	"\vSymbolTable\x127\n" +
	"\aimports\x18\x01 \x03(\v2\x1d.buf.compiler.v1alpha1.ImportR\aimports\x127\n" +
	"\asymbols\x18\x02 \x03(\v2\x1d.buf.compiler.v1alpha1.SymbolR\asymbols\x126\n" +
	"\aoptions\x18\x03 \x01(\v2\x1c.buf.compiler.v1alpha1.ValueR\aoptions\"\x82\x01\n" +
	"\x06Import\x12\x12\n" +
	"\x04path\x18\x01 \x01(\tR\x04path\x12\x16\n" +
	"\x06public\x18\x02 \x01(\bR\x06public\x12\x12\n" +
	"\x04weak\x18\x03 \x01(\bR\x04weak\x12\x1e\n" +
	"\n" +
	"transitive\x18\x04 \x01(\bR\n" +
	"transitive\x12\x18\n" +
	"\avisible\x18\x05 \x01(\bR\avisible\"\xfa\x02\n" +
	"\x06Symbol\x12\x10\n" +
	"\x03fqn\x18\x01 \x01(\tR\x03fqn\x126\n" +
	"\x04kind\x18\x02 \x01(\x0e2\".buf.compiler.v1alpha1.Symbol.KindR\x04kind\x12\x12\n" +
	"\x04file\x18\x03 \x01(\tR\x04file\x12\x14\n" +
	"\x05index\x18\x04 \x01(\rR\x05index\x12\x18\n" +
	"\avisible\x18\x05 \x01(\bR\avisible\x126\n" +
	"\aoptions\x18\x06 \x01(\v2\x1c.buf.compiler.v1alpha1.ValueR\aoptions\"\xa9\x01\n" +
	"\x04Kind\x12\x14\n" +
	"\x10KIND_UNSPECIFIED\x10\x00\x12\x10\n" +
	"\fKIND_PACKAGE\x10\x01\x12\x0f\n" +
	"\vKIND_SCALAR\x10\x02\x12\x10\n" +
	"\fKIND_MESSAGE\x10\x03\x12\r\n" +
	"\tKIND_ENUM\x10\x04\x12\x0e\n" +
	"\n" +
	"KIND_FIELD\x10\x05\x12\x13\n" +
	"\x0fKIND_ENUM_VALUE\x10\x06\x12\x12\n" +
	"\x0eKIND_EXTENSION\x10\a\x12\x0e\n" +
	"\n" +
	"KIND_ONEOF\x10\b\"\x99\x05\n" +
	"\x05Value\x12\x14\n" +
	"\x04bool\x18\x01 \x01(\bH\x00R\x04bool\x12\x12\n" +
	"\x03int\x18\x02 \x01(\x03H\x00R\x03int\x12\x14\n" +
	"\x04uint\x18\x03 \x01(\x04H\x00R\x04uint\x12\x16\n" +
	"\x05float\x18\x04 \x01(\x01H\x00R\x05float\x12\x18\n" +
	"\x06string\x18\x05 \x01(\fH\x00R\x06string\x12C\n" +
	"\brepeated\x18\x06 \x01(\v2%.buf.compiler.v1alpha1.Value.RepeatedH\x00R\brepeated\x12@\n" +
	"\amessage\x18\a \x01(\v2$.buf.compiler.v1alpha1.Value.MessageH\x00R\amessage\x1a\xcb\x02\n" +
	"\aMessage\x12H\n" +
	"\x06fields\x18\x01 \x03(\v20.buf.compiler.v1alpha1.Value.Message.FieldsEntryR\x06fields\x12E\n" +
	"\x05extns\x18\x02 \x03(\v2/.buf.compiler.v1alpha1.Value.Message.ExtnsEntryR\x05extns\x1aW\n" +
	"\vFieldsEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x122\n" +
	"\x05value\x18\x02 \x01(\v2\x1c.buf.compiler.v1alpha1.ValueR\x05value:\x028\x01\x1aV\n" +
	"\n" +
	"ExtnsEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x122\n" +
	"\x05value\x18\x02 \x01(\v2\x1c.buf.compiler.v1alpha1.ValueR\x05value:\x028\x01\x1a@\n" +
	"\bRepeated\x124\n" +
	"\x06values\x18\x01 \x03(\v2\x1c.buf.compiler.v1alpha1.ValueR\x06valuesB\a\n" +
	"\x05valueB\xf4\x01\n" +
	"\x19com.buf.compiler.v1alpha1B\vSymtabProtoP\x01ZTgithub.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1;compilerv1alpha1\xa2\x02\x03BCX\xaa\x02\x15Buf.Compiler.V1alpha1\xca\x02\x15Buf\\Compiler\\V1alpha1\xe2\x02!Buf\\Compiler\\V1alpha1\\GPBMetadata\xea\x02\x17Buf::Compiler::V1alpha1b\x06proto3"

var (
	file_buf_compiler_v1alpha1_symtab_proto_rawDescOnce sync.Once
	file_buf_compiler_v1alpha1_symtab_proto_rawDescData []byte
)

func file_buf_compiler_v1alpha1_symtab_proto_rawDescGZIP() []byte {
	file_buf_compiler_v1alpha1_symtab_proto_rawDescOnce.Do(func() {
		file_buf_compiler_v1alpha1_symtab_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_buf_compiler_v1alpha1_symtab_proto_rawDesc), len(file_buf_compiler_v1alpha1_symtab_proto_rawDesc)))
	})
	return file_buf_compiler_v1alpha1_symtab_proto_rawDescData
}

var file_buf_compiler_v1alpha1_symtab_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_buf_compiler_v1alpha1_symtab_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_buf_compiler_v1alpha1_symtab_proto_goTypes = []any{
	(Symbol_Kind)(0),       // 0: buf.compiler.v1alpha1.Symbol.Kind
	(*SymbolSet)(nil),      // 1: buf.compiler.v1alpha1.SymbolSet
	(*SymbolTable)(nil),    // 2: buf.compiler.v1alpha1.SymbolTable
	(*Import)(nil),         // 3: buf.compiler.v1alpha1.Import
	(*Symbol)(nil),         // 4: buf.compiler.v1alpha1.Symbol
	(*Value)(nil),          // 5: buf.compiler.v1alpha1.Value
	nil,                    // 6: buf.compiler.v1alpha1.SymbolSet.TablesEntry
	(*Value_Message)(nil),  // 7: buf.compiler.v1alpha1.Value.Message
	(*Value_Repeated)(nil), // 8: buf.compiler.v1alpha1.Value.Repeated
	nil,                    // 9: buf.compiler.v1alpha1.Value.Message.FieldsEntry
	nil,                    // 10: buf.compiler.v1alpha1.Value.Message.ExtnsEntry
}
var file_buf_compiler_v1alpha1_symtab_proto_depIdxs = []int32{
	6,  // 0: buf.compiler.v1alpha1.SymbolSet.tables:type_name -> buf.compiler.v1alpha1.SymbolSet.TablesEntry
	3,  // 1: buf.compiler.v1alpha1.SymbolTable.imports:type_name -> buf.compiler.v1alpha1.Import
	4,  // 2: buf.compiler.v1alpha1.SymbolTable.symbols:type_name -> buf.compiler.v1alpha1.Symbol
	5,  // 3: buf.compiler.v1alpha1.SymbolTable.options:type_name -> buf.compiler.v1alpha1.Value
	0,  // 4: buf.compiler.v1alpha1.Symbol.kind:type_name -> buf.compiler.v1alpha1.Symbol.Kind
	5,  // 5: buf.compiler.v1alpha1.Symbol.options:type_name -> buf.compiler.v1alpha1.Value
	8,  // 6: buf.compiler.v1alpha1.Value.repeated:type_name -> buf.compiler.v1alpha1.Value.Repeated
	7,  // 7: buf.compiler.v1alpha1.Value.message:type_name -> buf.compiler.v1alpha1.Value.Message
	2,  // 8: buf.compiler.v1alpha1.SymbolSet.TablesEntry.value:type_name -> buf.compiler.v1alpha1.SymbolTable
	9,  // 9: buf.compiler.v1alpha1.Value.Message.fields:type_name -> buf.compiler.v1alpha1.Value.Message.FieldsEntry
	10, // 10: buf.compiler.v1alpha1.Value.Message.extns:type_name -> buf.compiler.v1alpha1.Value.Message.ExtnsEntry
	5,  // 11: buf.compiler.v1alpha1.Value.Repeated.values:type_name -> buf.compiler.v1alpha1.Value
	5,  // 12: buf.compiler.v1alpha1.Value.Message.FieldsEntry.value:type_name -> buf.compiler.v1alpha1.Value
	5,  // 13: buf.compiler.v1alpha1.Value.Message.ExtnsEntry.value:type_name -> buf.compiler.v1alpha1.Value
	14, // [14:14] is the sub-list for method output_type
	14, // [14:14] is the sub-list for method input_type
	14, // [14:14] is the sub-list for extension type_name
	14, // [14:14] is the sub-list for extension extendee
	0,  // [0:14] is the sub-list for field type_name
}

func init() { file_buf_compiler_v1alpha1_symtab_proto_init() }
func file_buf_compiler_v1alpha1_symtab_proto_init() {
	if File_buf_compiler_v1alpha1_symtab_proto != nil {
		return
	}
	file_buf_compiler_v1alpha1_symtab_proto_msgTypes[4].OneofWrappers = []any{
		(*Value_Bool)(nil),
		(*Value_Int)(nil),
		(*Value_Uint)(nil),
		(*Value_Float)(nil),
		(*Value_String_)(nil),
		(*Value_Repeated_)(nil),
		(*Value_Message_)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_buf_compiler_v1alpha1_symtab_proto_rawDesc), len(file_buf_compiler_v1alpha1_symtab_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_buf_compiler_v1alpha1_symtab_proto_goTypes,
		DependencyIndexes: file_buf_compiler_v1alpha1_symtab_proto_depIdxs,
		EnumInfos:         file_buf_compiler_v1alpha1_symtab_proto_enumTypes,
		MessageInfos:      file_buf_compiler_v1alpha1_symtab_proto_msgTypes,
	}.Build()
	File_buf_compiler_v1alpha1_symtab_proto = out.File
	file_buf_compiler_v1alpha1_symtab_proto_goTypes = nil
	file_buf_compiler_v1alpha1_symtab_proto_depIdxs = nil
}
