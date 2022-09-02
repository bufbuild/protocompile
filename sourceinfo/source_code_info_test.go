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

package sourceinfo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/protoutil"
)

func TestSourceCodeInfo(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		}),
		IncludeSourceInfo: true,
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_comments.proto", "desc_test_complex.proto")
	if !assert.Nil(t, err) {
		return
	}
	// also test that imported files have source code info
	// (desc_test_comments.proto imports desc_test_options.proto)
	importedFd := fds[0].FindImportByPath("desc_test_options.proto")
	if !assert.NotNil(t, importedFd) {
		return
	}

	fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/source_info.protoset", linker.ResolverFromFile(fds[0]))
	actualFdset := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			protoutil.ProtoFromFileDescriptor(importedFd),
			protoutil.ProtoFromFileDescriptor(fds[0]),
			protoutil.ProtoFromFileDescriptor(fds[1]),
		},
	}

	for _, actualFd := range actualFdset.File {
		var expectedFd *descriptorpb.FileDescriptorProto
		for _, fd := range fdset.File {
			if fd.GetName() == actualFd.GetName() {
				expectedFd = fd
				break
			}
		}
		if !assert.NotNil(t, expectedFd, "file %q not found in source_info.protoset", actualFd.GetName()) {
			continue
		}
		prototest.CompareMessages(t, fmt.Sprintf("%q.source_code_info", actualFd.GetName()), expectedFd.SourceCodeInfo.ProtoReflect(), actualFd.SourceCodeInfo.ProtoReflect())
	}
}

