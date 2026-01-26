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

// This file demonstrates tests migrated to use the dual-compiler framework.

package protocompile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/testing/dualcompiler"
)

// TestDualCompiler_ParseFilesMessageComments tests message comment parsing.
// Migrated from: compiler_test.go::TestParseFilesMessageComments.
func TestDualCompiler_ParseFilesMessageComments(t *testing.T) {
	t.Parallel()

	skip := dualcompiler.SkipConfig{
		SkipNew:    true,
		SkipReason: "source code info not yet fully implemented in experimental compiler",
	}

	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: []string{"internal/testdata"},
	})
	opts := []dualcompiler.CompilerOption{
		dualcompiler.WithResolver(resolver),
		dualcompiler.WithSourceInfoMode(protocompile.SourceInfoStandard),
	}

	dualcompiler.RunWithBothCompilersIf(
		t,
		skip,
		opts,
		func(t *testing.T, compiler dualcompiler.CompilerInterface) {
			ctx := t.Context()
			result, err := compiler.Compile(ctx, "desc_test1.proto")
			require.NoError(t, err)

			comments := ""
			expected := " Comment for TestMessage\n"
			for _, compiledFile := range result.Files() {
				file, err := compiledFile.FileDescriptor()
				require.NoError(t, err)
				msg := file.Messages().ByName("TestMessage")
				if msg != nil {
					si := file.SourceLocations().ByDescriptor(msg)
					if si.Path != nil {
						comments = si.LeadingComments
					}
					break
				}
			}
			assert.Equal(t, expected, comments)
		},
	)
}

// TestDualCompiler_ParseFilesWithImportsNoImportPath tests parsing files with imports.
// Migrated from: compiler_test.go::TestParseFilesWithImportsNoImportPath.
func TestDualCompiler_ParseFilesWithImportsNoImportPath(t *testing.T) {
	t.Parallel()

	relFilePaths := []string{
		"a/b/b1.proto",
		"a/b/b2.proto",
		"c/c.proto",
	}

	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: []string{"internal/testdata/more"},
	})
	opts := []dualcompiler.CompilerOption{
		dualcompiler.WithResolver(resolver),
	}

	dualcompiler.RunAndCompareIf(
		t,
		dualcompiler.SkipConfig{},
		opts,
		func(t *testing.T, oldCompiler, newCompiler dualcompiler.CompilerInterface) (oldResult, newResult dualcompiler.CompilationResult) {
			var err error
			oldResult, err = oldCompiler.Compile(t.Context(), relFilePaths...)
			require.NoError(t, err)

			newResult, err = newCompiler.Compile(t.Context(), relFilePaths...)
			require.NoError(t, err)

			assert.Equal(t, len(relFilePaths), len(oldResult.Files()))
			assert.Equal(t, len(relFilePaths), len(newResult.Files()))

			return oldResult, newResult
		},
	)
}

// TestDualCompiler_ParseCommentsBeforeDot tests comment parsing before dots.
// Migrated from: compiler_test.go::TestParseCommentsBeforeDot.
func TestDualCompiler_ParseCommentsBeforeDot(t *testing.T) {
	t.Parallel()

	skip := dualcompiler.SkipConfig{
		SkipNew:    true,
		SkipReason: "source code info not yet fully implemented in experimental compiler",
	}

	accessor := protocompile.SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
message Foo {
  // leading comments
  .Foo foo = 1;
}
`,
	})
	resolver := &protocompile.SourceResolver{Accessor: accessor}
	opts := []dualcompiler.CompilerOption{
		dualcompiler.WithResolver(resolver),
		dualcompiler.WithSourceInfoMode(protocompile.SourceInfoStandard),
	}

	dualcompiler.RunWithBothCompilersIf(
		t,
		skip,
		opts,
		func(t *testing.T, compiler dualcompiler.CompilerInterface) {
			ctx := t.Context()
			result, err := compiler.Compile(ctx, "test.proto")
			require.NoError(t, err)
			require.Len(t, result.Files(), 1)

			file, err := result.Files()[0].FileDescriptor()
			require.NoError(t, err)
			field := file.Messages().Get(0).Fields().Get(0)
			comment := file.SourceLocations().ByDescriptor(field).LeadingComments
			assert.Equal(t, " leading comments\n", comment)
		},
	)
}

// TestDualCompiler_ParseCustomOptions tests parsing custom options.
// Migrated from: compiler_test.go::TestParseCustomOptions.
func TestDualCompiler_ParseCustomOptions(t *testing.T) {
	t.Parallel()

	skip := dualcompiler.SkipConfig{
		SkipNew:    true,
		SkipReason: "extension descriptors from protodesc don't support self-referential extensions",
	}

	accessor := protocompile.SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions {
    string foo = 30303;
    int64 bar = 30304;
}
message Foo {
  option (.foo) = "foo";
  option (bar) = 123;
}
`,
	})
	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: accessor})
	opts := []dualcompiler.CompilerOption{
		dualcompiler.WithResolver(resolver),
		dualcompiler.WithSourceInfoMode(protocompile.SourceInfoStandard),
	}

	dualcompiler.RunWithBothCompilersIf(
		t,
		skip,
		opts,
		func(t *testing.T, compiler dualcompiler.CompilerInterface) {
			ctx := t.Context()
			result, err := compiler.Compile(ctx, "test.proto")
			require.NoError(t, err)
			require.Len(t, result.Files(), 1)

			file, err := result.Files()[0].FileDescriptor()
			require.NoError(t, err)

			ext := file.Extensions().ByName("foo")
			md := file.Messages().Get(0)
			fooVal := md.Options().ProtoReflect().Get(ext)
			assert.Equal(t, "foo", fooVal.String())

			ext = file.Extensions().ByName("bar")
			barVal := md.Options().ProtoReflect().Get(ext)
			assert.Equal(t, int64(123), barVal.Int())
		},
	)
}
