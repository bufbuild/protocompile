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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/protoutil"
)

func TestSourceCodeInfo(t *testing.T) {
	t.Parallel()
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		}),
		IncludeSourceInfo: true,
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_comments.proto", "desc_test_complex.proto")
	if pe, ok := err.(protocompile.PanicError); ok {
		t.Fatalf("panic! %v\n%v", pe, pe.Stack)
	}
	require.NoError(t, err)
	// also test that imported files have source code info
	// (desc_test_comments.proto imports desc_test_options.proto)
	importedFd := fds[0].FindImportByPath("desc_test_options.proto")
	require.NotNil(t, importedFd)

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
		fixupProtocSourceCodeInfo(expectedFd.SourceCodeInfo)
		prototest.AssertMessagesEqual(t, expectedFd.SourceCodeInfo, actualFd.SourceCodeInfo, expectedFd.GetName())
	}
}

var protocFixers = []struct {
	pathPatterns []*regexp.Regexp
	fixer        func(allLocs []*descriptorpb.SourceCodeInfo_Location, currentIndex int) *descriptorpb.SourceCodeInfo_Location
}{
	{
		// FieldDescriptorProto.default_value
		// https://github.com/protocolbuffers/protobuf/issues/10478
		pathPatterns: []*regexp.Regexp{
			regexp.MustCompile("^4,\\d+,(?:3,\\d+,)*2,\\d+,7$"), // normal fields
			regexp.MustCompile("^7,\\d+,7$"),                    // extension fields, top-level in file
			regexp.MustCompile("^4,\\d+,(?:3,\\d+,)*7,\\d+,7$"), // extension fields, nested in a message
		},
		fixer: func(allLocs []*descriptorpb.SourceCodeInfo_Location, currentIndex int) *descriptorpb.SourceCodeInfo_Location {
			// adjust span to include preceding "default = "
			allLocs[currentIndex].Span[1] -= 10
			return allLocs[currentIndex]
		},
	},
	{
		// FieldDescriptorProto.json_name
		// https://github.com/protocolbuffers/protobuf/issues/10478
		pathPatterns: []*regexp.Regexp{
			regexp.MustCompile("^4,\\d+,(?:3,\\d+,)*2,\\d+,10$"),
		},
		fixer: func(allLocs []*descriptorpb.SourceCodeInfo_Location, currentIndex int) *descriptorpb.SourceCodeInfo_Location {
			if currentIndex > 0 && pathsEqual(allLocs[currentIndex].Path, allLocs[currentIndex-1].Path) {
				// second span for json_name is not useful; remove it
				return nil
			}
			return allLocs[currentIndex]
		},
	},
}

func pathsEqual(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if b[i] != a[i] {
			return false
		}
	}
	return true
}

func fixupProtocSourceCodeInfo(info *descriptorpb.SourceCodeInfo) {
	for i := 0; i < len(info.Location); i++ {
		loc := info.Location[i]

		pathStrs := make([]string, len(loc.Path))
		for j, val := range loc.Path {
			pathStrs[j] = strconv.FormatInt(int64(val), 10)
		}
		pathStr := strings.Join(pathStrs, ",")

		for _, fixerEntry := range protocFixers {
			match := false
			for _, pattern := range fixerEntry.pathPatterns {
				if pattern.MatchString(pathStr) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
			newLoc := fixerEntry.fixer(info.Location, i)
			if newLoc == nil {
				// remove this entry
				info.Location = append(info.Location[:i], info.Location[i+1:]...)
				i--
			} else {
				info.Location[i] = newLoc
			}
			// only apply one fixer to each location
			break
		}
	}
}
