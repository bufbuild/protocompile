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

package protoutil

import (
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ProtoFromFileDescriptor extracts a descriptor proto from the given "rich"
// descriptor. For file descriptors generated by the compiler, this is an
// inexpensive and non-lossy operation. File descriptors from other sources
// however may be expensive (to re-create a proto) and even lossy.
func ProtoFromFileDescriptor(f protoreflect.FileDescriptor) *descriptorpb.FileDescriptorProto {
	type canProto interface {
		Proto() *descriptorpb.FileDescriptorProto
	}
	if res, ok := f.(canProto); ok {
		return res.Proto()
	}
	return protodesc.ToFileDescriptorProto(f)
}
