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

// DescriptorProto generates a single [*descriptorpb.FileDescriptorProto] for the given
// [*ir.File].
func DescriptorProto(file *ir.File, options ...DescriptorOption) (*descriptorpb.FileDescriptorProto, error) {
	var g generator
	g.Apply(options...)

	if g.exclude != nil && g.exclude.Exclude(file) {
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
	g.Apply(options...)

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
	g.Apply(options...)

	fdp := new(descriptorpb.FileDescriptorProto)
	g.file(file, fdp)
	return proto.Marshal(fdp)
}

// DescriptorOption is an option to pass to [DescriptorSetBytes], [DescriptorProtoBytes],
// or DescriptorProto.
type DescriptorOption interface {
	apply(*Options)
}

type descriptorOption func(*Options)

// [DescriptorOption] instance for [descriptorOption].
//
// This lets us use arbitrary closures as a DescriptorOption. We use this in
// [IncludeSourceCodeInfo], [GenerateExtraOptionLocations], and [ExcludeFiles].
func (dopt descriptorOption) apply(o *Options) {
	dopt(o)
}

// [DescriptorOption] instance for [Options].
//
// This allows us for example to pass a value of [Options] to [DescriptorProto].
func (o *Options) apply(that *Options) {
	*that = *o
}

// Apply applies the given [DescriptorOption] to this [Options].
//
// Nil values are ignored; does nothing if opt is nil.
func (o *Options) Apply(options ...DescriptorOption) *Options {
	if o != nil {
		for _, option := range options {
			if option != nil {
				option.apply(o)
			}
		}
	}
	return o
}

// IncludeSourceCodeInfo sets whether or not to include google.protobuf.SourceCodeInfo in
// the output.
func IncludeSourceCodeInfo(flag bool) DescriptorOption {
	return descriptorOption(func(o *Options) {
		if flag {
			o.debug = new(debug)
		} else {
			o.debug = nil
		}
	})
}

// GenerateExtraOptionLocations set whether or not to generate additional locations for
// elements inside of message literals in option values. This option is a no-op if
// [IncludeSourceCodeInfo] is not set.
func GenerateExtraOptionLocations(flag bool) DescriptorOption {
	return descriptorOption(func(o *Options) {
		o.generateExtraOptionLocations = flag
	})
}

// Excluder is used with [ExcludeFiles].
//
// This is an interface, rather than a function, so that implementations can be comparable for
// use in queries.
type Excluder interface {
	Exclude(*ir.File) bool
}

// ExcludeFiles excludes the given files from the output of [DescriptorSetBytes] and
// [DescriptorProto].
func ExcludeFiles(exclude Excluder) DescriptorOption {
	return descriptorOption(func(o *Options) {
		o.exclude = exclude
	})
}
