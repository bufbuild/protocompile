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

package protocompile

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	_ "google.golang.org/protobuf/types/gofeaturespb" // link in packages that include the standard protos included with protoc.
	_ "google.golang.org/protobuf/types/known/anypb"
	_ "google.golang.org/protobuf/types/known/apipb"
	_ "google.golang.org/protobuf/types/known/durationpb"
	_ "google.golang.org/protobuf/types/known/emptypb"
	_ "google.golang.org/protobuf/types/known/fieldmaskpb"
	_ "google.golang.org/protobuf/types/known/sourcecontextpb"
	_ "google.golang.org/protobuf/types/known/structpb"
	_ "google.golang.org/protobuf/types/known/timestamppb"
	_ "google.golang.org/protobuf/types/known/typepb"
	_ "google.golang.org/protobuf/types/known/wrapperspb"
	_ "google.golang.org/protobuf/types/pluginpb"

	"github.com/bufbuild/protocompile/internal/featuresext"
)

// All files that are included with protoc are also included with this package
// so that clients do not need to explicitly supply a copy of these protos (just
// like callers of protoc do not need to supply them).
var standardImports map[string]protoreflect.FileDescriptor

func init() {
	standardFilenames := []string{
		"google/protobuf/any.proto",
		"google/protobuf/api.proto",
		"google/protobuf/compiler/plugin.proto",
		"google/protobuf/descriptor.proto",
		"google/protobuf/duration.proto",
		"google/protobuf/empty.proto",
		"google/protobuf/field_mask.proto",
		"google/protobuf/go_features.proto",
		"google/protobuf/source_context.proto",
		"google/protobuf/struct.proto",
		"google/protobuf/timestamp.proto",
		"google/protobuf/type.proto",
		"google/protobuf/wrappers.proto",
	}

	standardImports = map[string]protoreflect.FileDescriptor{}
	for _, fn := range standardFilenames {
		fd, err := protoregistry.GlobalFiles.FindFileByPath(fn)
		if err != nil {
			panic(err.Error())
		}
		standardImports[fn] = fd
	}

	otherFeatures := []struct {
		Name          string
		GetDescriptor func() (protoreflect.FileDescriptor, error)
	}{
		{
			Name:          "google/protobuf/cpp_features.proto",
			GetDescriptor: featuresext.CppFeaturesDescriptor,
		},
		{
			Name:          "google/protobuf/java_features.proto",
			GetDescriptor: featuresext.JavaFeaturesDescriptor,
		},
	}
	for _, feature := range otherFeatures {
		// First see if the program has generated Go code for this
		// file linked in:
		fd, err := protoregistry.GlobalFiles.FindFileByPath(feature.Name)
		if err == nil {
			standardImports[feature.Name] = fd
			continue
		}
		fd, err = feature.GetDescriptor()
		if err != nil {
			// For these extensions to FeatureSet, we are lenient. If
			// we can't load them, just ignore them.
			continue
		}
		standardImports[feature.Name] = fd
	}
}
