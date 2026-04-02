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

// Package fdp provides functionality for lowering the IR to a FileDescriptorSet.
package fdp

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ir"
)

// DescriptorProtoExclude generates a single [*descriptorpb.FileDescriptorProto] for the given [*ir.File].
func DescriptorProtoExclude(file *ir.File, options ...DescriptorOption) (*descriptorpb.FileDescriptorProto, error) {
	var g generator
	for _, opt := range options {
		if opt != nil {
			opt(&g)
		}
	}

	if g.exclude != nil && g.exclude(file) {
		return nil, nil
	}

	fdp := new(descriptorpb.FileDescriptorProto)
	g.file(file, fdp)
	return fdp, nil
}

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

// IncludeSourceCodeInfo sets whether or not to include google.protobuf.SourceCodeInfo in
// the output.
func IncludeSourceCodeInfo(flag bool) DescriptorOption {
	return func(g *generator) {
		if flag {
			g.debug = new(debug)
		} else {
			g.debug = nil
		}
	}
}

// ExcludeFiles excludes the given files from the output of [DescriptorSetBytes].
func ExcludeFiles(exclude func(*ir.File) bool) DescriptorOption {
	return func(g *generator) {
		g.exclude = exclude
	}
}

// GenerateExtraOptionLocations set whether or not to generate additional locations for
// elements inside of message literals in option values. This option is a no-op if
// [IncludeSourceCodeInfo] is not set.
func GenerateExtraOptionLocations(flag bool) DescriptorOption {
	return func(g *generator) {
		g.generateExtraOptionLocations = flag
	}
}

// Options is a comparable way to set certain options in the generator.
type Options struct {
	IncludeSourceCodeInfo        bool
	GenerateExtraOptionLocations bool
}

// Excluder is used because functions are not comparable.
// Instead we create types such as [IRExcluder] which implement this, and are comparable.
type Excluder interface {
	Exclude(*ir.File) bool
}

// OptionsToDescriptorOptions turns [Options] to an array of [DescriptorOption].
func OptionsToDescriptorOptions(opt Options) []DescriptorOption {
	return []DescriptorOption{
		IncludeSourceCodeInfo(opt.IncludeSourceCodeInfo),
		GenerateExtraOptionLocations(opt.GenerateExtraOptionLocations),
	}
}

// ExcluderToOption turns an implementation of Excluder into a DescriptorOption.
func ExcluderToOption(excl Excluder) DescriptorOption {
	return ExcludeFiles(func(f *ir.File) bool { return excl.Exclude(f) })
}

// An implementation of [Excluder]. We exclude files for which IsDescriptorProto() returns true.
type IRExcluder struct{}

func (IRExcluder) Exclude(file *ir.File) bool {
	return file.IsDescriptorProto()
}
