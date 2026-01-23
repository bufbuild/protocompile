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

// Package descriptor provides functionality for lowering the IR to a FileDescriptorSet.
package descriptor

import (
	"github.com/bufbuild/protocompile/experimental/ir"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DescriptorSetBytes generates a FileDescriptorSet for the given files, and returns the
// result as an encoded byte slice.
//
// The resulting FileDescriptorSet is always fully linked: it contains all dependencies except
// the WKTs, and all names are fully-qualified.
func DescriptorSetBytes(files []*ir.File, options ...DescriptorOption) ([]byte, error) {
	var g generator
	for _, opt := range options {
		if opt != nil {
			opt(&g)
		}
	}

	fds := new(descriptorpb.FileDescriptorSet)
	g.files(files, fds)
	return proto.Marshal(fds)
}

// DescriptorProtoBytes generates a single FileDescriptorProto for file, and returns the
// result as an encoded byte slice.
//
// The resulting FileDescriptorProto is fully linked: all names are fully-qualified.
func DescriptorProtoBytes(file *ir.File, options ...DescriptorOption) ([]byte, error) {
	var g generator
	for _, opt := range options {
		if opt != nil {
			opt(&g)
		}
	}

	fdp := new(descriptorpb.FileDescriptorProto)
	g.file(file, fdp)
	return proto.Marshal(fdp)
}

// DescriptorOption is an option to pass to [DescriptorSetBytes] or [DescriptorProtoBytes].
type DescriptorOption func(*generator)

// IncludeDebugInfo sets whether or not to include google.protobuf.SourceCodeInfo in
// the output.
func IncludeSourceCodeInfo(flag bool) DescriptorOption {
	return func(g *generator) {
		g.includeDebugInfo = flag
	}
}

// ExcludeFiles excludes the given files from the output of [DescriptorSetBytes].
func ExcludeFiles(exclude func(*ir.File) bool) DescriptorOption {
	return func(g *generator) {
		g.exclude = exclude
	}
}
