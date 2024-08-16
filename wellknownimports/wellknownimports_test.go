// Copyright 2020-2024 Buf Technologies, Inc.
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

package wellknownimports

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
)

func TestWithStandardImports(t *testing.T) {
	t.Parallel()
	wellKnownImports := []string{
		"google/protobuf/any.proto",
		"google/protobuf/api.proto",
		"google/protobuf/compiler/plugin.proto",
		"google/protobuf/cpp_features.proto",
		"google/protobuf/descriptor.proto",
		"google/protobuf/duration.proto",
		"google/protobuf/empty.proto",
		"google/protobuf/field_mask.proto",
		"google/protobuf/java_features.proto",
		"google/protobuf/source_context.proto",
		"google/protobuf/struct.proto",
		"google/protobuf/timestamp.proto",
		"google/protobuf/type.proto",
		"google/protobuf/wrappers.proto",
	}
	// make sure we can successfully compile them all
	c := protocompile.Compiler{
		Resolver: WithStandardImports(&protocompile.SourceResolver{
			Accessor: func(_ string) (io.ReadCloser, error) {
				return nil, os.ErrNotExist
			},
		}),
		RetainASTs: true,
	}
	ctx := context.Background()
	for _, name := range wellKnownImports {
		t.Log(name)
		fds, err := c.Compile(ctx, name)
		if err != nil {
			t.Errorf("failed to compile %q: %v", name, err)
			continue
		}
		if len(fds) != 1 {
			t.Errorf("Compile returned wrong number of descriptors: expecting 1, got %d", len(fds))
			continue
		}
		// Make sure they were built from source
		result, ok := fds[0].(linker.Result)
		require.True(t, ok)
		require.NotNil(t, result.AST())

		if name == "google/protobuf/descriptor.proto" {
			// verify the extension declarations are present
			d := fds[0].FindDescriptorByName("google.protobuf.FeatureSet")
			require.NotNil(t, d)
			md, ok := d.(protoreflect.MessageDescriptor)
			require.True(t, ok)
			var extRangeCount int
			for i := 0; i < md.ExtensionRanges().Len(); i++ {
				opts, ok := md.ExtensionRangeOptions(i).(*descriptorpb.ExtensionRangeOptions)
				require.True(t, ok)
				extRangeCount += len(opts.GetDeclaration())
			}
			require.Positive(t, extRangeCount, "no declarations found for FeatureSet for %q", name)
		}
	}
}

func TestCantRedefineWellKnownCustomFeature(t *testing.T) {
	t.Parallel()
	c := protocompile.Compiler{
		Resolver: WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"features.proto": `
					edition = "2023";
					import "google/protobuf/descriptor.proto";
					message Custom {
						bool flag = 1;
					}
					extend google.protobuf.FeatureSet {
						// tag 1000 is declared by pb.cpp so shouldn't be allowed
						Custom custom = 1000;
					}
					`,
			}),
		}),
	}
	ctx := context.Background()
	_, err := c.Compile(ctx, "features.proto")
	require.ErrorContains(t, err, `features.proto:9:56: expected extension with number 1000 to be named pb.cpp, not custom, per declaration at google/protobuf/descriptor.proto`)
}
