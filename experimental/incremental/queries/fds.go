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

package queries

import (
	"slices"

	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/fdp"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// FDS is an [incremental.Query] that produces a [*descriptorpb.FileDescriptorSet].
// It takes the IR files [ir.File] produced via [Link], topologically
// sorts the resulting IR files, and converts each to a [*descriptorpb.FileDescriptorProto]
// via [FDP]. It then bundles them all together into a [*descriptorpb.FileDescriptorSet].
//
// FDS queries with different Openers, options, and workspaces are considered distinct.
type FDS struct {
	source.Opener // Must be comparable.
	*ir.Session
	source.Workspace // Must be comparable
	fdp.Options      // Must be comparable
}

var _ incremental.Query[*descriptorpb.FileDescriptorSet] = FDS{}

// Key implements [incremental.Query].
func (l FDS) Key() any {
	return l
}

// Execute implements [incremental.Query].
func (l FDS) Execute(t *incremental.Task) (*descriptorpb.FileDescriptorSet, error) {
	t.Report().Options.Stage += stageFDS

	linkQuery := Link{
		Opener:    l.Opener,
		Session:   l.Session,
		Workspace: l.Workspace,
	}

	linkResult, err := incremental.Resolve(t, linkQuery)
	if err != nil {
		return nil, err
	}

	irs := linkResult[0].Value
	irs = slices.DeleteFunc(irs, func(f *ir.File) bool { return f == nil })

	fdpQueries := slicesx.Transform(
		slices.Collect(ir.TopoSort(irs)),
		func(f *ir.File) incremental.Query[*descriptorpb.FileDescriptorProto] {
			return FDP{
				File:    f,
				Options: l.Options,
			}
		},
	)

	fdpResults, err := incremental.Resolve(t, fdpQueries...)
	if err != nil {
		return nil, err
	}
	fdps, err := fdpResults.Slice()
	if err != nil {
		return nil, err
	}
	fdps = slices.DeleteFunc(
		fdps,
		func(fdp *descriptorpb.FileDescriptorProto) bool { return fdp == nil },
	)

	return &descriptorpb.FileDescriptorSet{File: fdps}, nil
}
