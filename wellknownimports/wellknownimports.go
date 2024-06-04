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

// Package wellknownimports provides source code for the well-known import
// files for use with a protocompile.Compiler.
package wellknownimports

import (
	"embed"
	"io"

	"github.com/bufbuild/protocompile"
)

//go:embed google/protobuf/*.proto google/protobuf/*/*.proto
var files embed.FS

// WithStandardImports returns a new resolver that can provide the source code for the
// standard imports that are included with protoc. This differs from
// protocompile.WithStandardImports, which uses descriptors embedded in generated
// code in the Protobuf Go module. That function is lighter weight, and does not need
// to bring in additional embedded data outside the Protobuf Go runtime. This version
// includes its own embedded versions of the source files.
//
// Unlike protocompile.WithStandardImports, this resolver does not provide results for
// "google/protobuf/go_features.proto" file. This resolver is backed by source files
// that are shipped with the Protobuf installation, which does not include that file.
//
// It is possible that the source code provided by this resolver differs from the
// source code used to create the descriptors provided by protocompile.WithStandardImports.
// That is because that other function depends on the Protobuf Go module, which could
// resolve in user programs to a different version than was used to build this package.
func WithStandardImports(resolver protocompile.Resolver) protocompile.Resolver {
	return protocompile.CompositeResolver{
		resolver,
		&protocompile.SourceResolver{
			Accessor: func(path string) (io.ReadCloser, error) {
				return files.Open(path)
			},
		},
	}
}
