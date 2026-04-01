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
	"github.com/bufbuild/protocompile/experimental/fdp"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/ir"
	"google.golang.org/protobuf/types/descriptorpb"
)

// FDP is an [incremental.Query] that converts the lowered IR files [*ir.File]
// to [*descriptorpb.FileDescriptorProto]
//
// FDP queries with different File and Options are considered distinct.
// The File field must not be edited between different FDP queries!
type FDP struct {
	*ir.File // Must be comparable.
	*ir.Session
	Options *[]fdp.DescriptorOption // Must be comparable
}

var _ incremental.Query[*descriptorpb.FileDescriptorProto] = FDP{}

// Key implements [incremental.Query].
func (l FDP) Key() any {
	return l
}

// Execute implements [incremental.Query].
func (l FDP) Execute(t *incremental.Task) (*descriptorpb.FileDescriptorProto, error) {
	t.Report().Options.Stage += stageFDP

	return fdp.DescriptorProtoExclude(l.File, *l.Options...)
}
