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

func (g *generator) apply(options ...DescriptorOption) *generator {
	for _, opt := range options {
		if opt != nil {
			opt.apply(&g.Options)
		}
	}
	return g
}

// DescriptorProtoExclude generates a single [*descriptorpb.FileDescriptorProto] for the given [*ir.File].
func DescriptorProto(file *ir.File, options ...DescriptorOption) (*descriptorpb.FileDescriptorProto, error) {
	var g generator
	g.apply(options...)

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
	g.apply(options...)

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
	g.apply(options...)

	fdp := new(descriptorpb.FileDescriptorProto)
	g.file(file, fdp)
	return proto.Marshal(fdp)
}

type DescriptorOption interface{ apply(*Options) }
type descriptorOption func(*Options)

// [DescriptorOption] instance for [descriptorOption]
func (dopt descriptorOption) apply(o *Options) {
	dopt(o)
}

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
func IncludeSourceCodeInfo(flag bool) descriptorOption {
	return func(o *Options) {
		if flag {
			o.debug = new(debug)
		} else {
			o.debug = nil
		}
	}
}

// GenerateExtraOptionLocations set whether or not to generate additional locations for
// elements inside of message literals in option values. This option is a no-op if
// [IncludeSourceCodeInfo] is not set.
func GenerateExtraOptionLocations(flag bool) descriptorOption {
	return func(o *Options) {
		o.generateExtraOptionLocations = flag
	}
}

// Excluder is used with [ExcludeFIles].
// This is an interface, rather than a function, so that implementations can be comparable for use in queries.
type Excluder interface {
	Exclude(*ir.File) bool
}

// ExcludeFiles excludes the given files from the output of [DescriptorSetBytes].
func ExcludeFiles(exclude Excluder) descriptorOption {
	return func(o *Options) {
		o.exclude = exclude
	}
}

// An implementation of [Excluder]. We exclude files for which IsDescriptorProto() returns true.
type IRExcluder struct{}

func (IRExcluder) Exclude(file *ir.File) bool {
	return file.IsDescriptorProto()
}
