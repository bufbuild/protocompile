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

package dualcompiler

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
)

const (
	// defaultTestdataPath is the relative path from internal/testing/dualcompiler to testdata.
	defaultTestdataPath = "../../testdata"
)

// SkipConfig controls which compilers to skip for a particular test.
type SkipConfig struct {
	// SkipOld causes the old compiler to be skipped.
	SkipOld bool
	// SkipNew causes the new compiler to be skipped.
	SkipNew bool
	// SkipReason is the reason for skipping (used in t.Skip message).
	SkipReason string
}

// RunWithBothCompilers runs the given test function with both the old and new compilers.
// The test function receives a CompilerInterface that can be used to compile proto files.
// Uses default configuration (testdata ImportPaths with standard imports).
//
// The test will run in parallel subtests, one for each compiler.
//
// Example usage:
//
//	func TestMyFeature(t *testing.T) {
//	    t.Parallel()
//	    dualcompiler.RunWithBothCompilers(t, func(t *testing.T, compiler dualcompiler.CompilerInterface) {
//	        result, err := compiler.Compile(t.Context(), "test.proto")
//	        require.NoError(t, err)
//	        // ... test logic
//	    })
//	}
func RunWithBothCompilers(t *testing.T, testFunc func(t *testing.T, compiler CompilerInterface)) {
	t.Helper()
	RunWithBothCompilersIf(t, SkipConfig{}, nil, testFunc)
}

// RunWithBothCompilersIf is like RunWithBothCompilers but allows skipping specific compilers.
// It also accepts compiler options to customize the compiler configuration.
//
// Example usage:
//
//	func TestSourceCodeInfo(t *testing.T) {
//	    t.Parallel()
//	    skip := dualcompiler.SkipConfig{
//	        SkipNew:    true,
//	        SkipReason: "source code info not yet implemented in experimental compiler",
//	    }
//	    resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
//	        ImportPaths: []string{"internal/testdata"},
//	    })
//	    opts := []dualcompiler.CompilerOption{
//	        dualcompiler.WithResolver(resolver),
//	        dualcompiler.WithSourceInfoMode(protocompile.SourceInfoStandard),
//	    }
//	    dualcompiler.RunWithBothCompilersIf(t, skip, opts, func(t *testing.T, compiler dualcompiler.CompilerInterface) {
//	        // ... test logic
//	    })
//	}
func RunWithBothCompilersIf(
	t *testing.T,
	skip SkipConfig,
	opts []CompilerOption,
	testFunc func(t *testing.T, compiler CompilerInterface),
) {
	t.Helper()

	if !skip.SkipOld {
		t.Run("old_compiler", func(t *testing.T) {
			t.Helper()
			t.Parallel()
			compiler := SetupOldCompilerWithOptions(t, opts)
			testFunc(t, compiler)
		})
	}

	if !skip.SkipNew {
		t.Run("new_compiler", func(t *testing.T) {
			t.Helper()
			t.Parallel()
			if skip.SkipReason != "" {
				t.Skip(skip.SkipReason)
			}
			compiler := SetupNewCompilerWithOptions(t, opts)
			testFunc(t, compiler)
		})
	}
}

// SetupOldCompiler creates a CompilerInterface for the old compiler with standard test configuration.
// The returned compiler will:
//   - Use a SourceResolver with ImportPaths set to internal/testdata
//   - Include standard imports (WKTs).
func SetupOldCompiler(t *testing.T) CompilerInterface {
	t.Helper()
	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: []string{defaultTestdataPath},
	})
	return NewOldCompiler(WithResolver(resolver))
}

// SetupOldCompilerWithOptions creates a CompilerInterface for the old compiler with custom configuration.
// If no resolver is specified in opts, a default resolver with standard imports will be used.
func SetupOldCompilerWithOptions(t *testing.T, opts []CompilerOption) CompilerInterface {
	t.Helper()
	config := &compilerConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.resolver == nil {
		resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{defaultTestdataPath},
		})
		opts = append([]CompilerOption{WithResolver(resolver)}, opts...)
	}
	return NewOldCompiler(opts...)
}

// SetupNewCompiler creates a CompilerInterface for the new compiler with standard test configuration.
// The returned compiler will:
//   - Use a SourceResolver with ImportPaths set to internal/testdata
//   - Include WKTs via source.WKTs().
func SetupNewCompiler(t *testing.T) CompilerInterface {
	t.Helper()
	resolver := &protocompile.SourceResolver{
		ImportPaths: []string{defaultTestdataPath},
	}
	return NewNewCompiler(WithResolver(resolver))
}

// SetupNewCompilerWithOptions creates a CompilerInterface for the new compiler with custom configuration.
// If no resolver is specified in opts, a default resolver will be used.
func SetupNewCompilerWithOptions(t *testing.T, opts []CompilerOption) CompilerInterface {
	t.Helper()
	config := &compilerConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.resolver == nil {
		resolver := &protocompile.SourceResolver{
			ImportPaths: []string{defaultTestdataPath},
		}
		opts = append([]CompilerOption{WithResolver(resolver)}, opts...)
	}
	return NewNewCompiler(opts...)
}

// RunAndCompare runs a test with both compilers and compares their outputs.
// This is a convenience wrapper that combines RunWithBothCompilers with CompareCompilationResults.
// Uses default configuration (testdata ImportPaths with standard imports).
//
// The compile function should compile the same files with both compilers and return the results.
// Source code info is stripped by default before comparison.
//
// Example usage:
//
//	func TestMyFeature(t *testing.T) {
//	    t.Parallel()
//	    dualcompiler.RunAndCompare(t, func(t *testing.T, oldCompiler, newCompiler dualcompiler.CompilerInterface) (old, new dualcompiler.CompilationResult) {
//	        // Compile with both
//	        oldResult, err := oldCompiler.Compile(t.Context(), "test.proto")
//	        require.NoError(t, err)
//	        newResult, err := newCompiler.Compile(t.Context(), "test.proto")
//	        require.NoError(t, err)
//	        return oldResult, newResult
//	    })
//	}
func RunAndCompare(
	t *testing.T,
	compileFunc func(t *testing.T, oldCompiler, newCompiler CompilerInterface) (oldResult, newResult CompilationResult),
) {
	t.Helper()
	RunAndCompareIf(t, SkipConfig{}, nil, compileFunc)
}

// RunAndCompareIf is like RunAndCompare but allows skipping specific compilers
// and customizing the compiler configuration with options.
func RunAndCompareIf(
	t *testing.T,
	skip SkipConfig,
	opts []CompilerOption,
	compileFunc func(t *testing.T, oldCompiler, newCompiler CompilerInterface) (oldResult, newResult CompilationResult),
) {
	t.Helper()

	// If either compiler is skipped, we can't compare
	if skip.SkipOld || skip.SkipNew {
		t.Skip("Skipping comparison test because one compiler is skipped:", skip.SkipReason)
		return
	}

	// Set up both compilers
	oldCompiler := SetupOldCompilerWithOptions(t, opts)
	newCompiler := SetupNewCompilerWithOptions(t, opts)

	// Compile with both
	oldResult, newResult := compileFunc(t, oldCompiler, newCompiler)

	// Compare results (strip source info by default)
	compareCompilationResults(t, oldResult, newResult, true)
}

// compareCompilationResults compares two CompilationResults to ensure they produce
// equivalent FileDescriptorProtos. This is useful for verifying that both compilers
// produce the same output.
//
// By default, source code info is stripped before comparison since the new compiler
// doesn't yet support it. Set stripSourceInfo to false to include source info in the comparison.
func compareCompilationResults(t *testing.T, result1, result2 CompilationResult, stripSourceInfo bool) {
	t.Helper()

	files1 := result1.Files()
	files2 := result2.Files()

	require.Equal(t, len(files1), len(files2), "different number of files")

	for i := range files1 {
		fdp1, err := files1[i].FileDescriptorProto()
		require.NoError(t, err, "failed to get FileDescriptorProto from file %d", i)

		fdp2, err := files2[i].FileDescriptorProto()
		require.NoError(t, err, "failed to get FileDescriptorProto from file %d", i)

		if stripSourceInfo {
			fdp1 = stripSourceCodeInfo(fdp1)
			fdp2 = stripSourceCodeInfo(fdp2)
		}

		assert.Empty(t, cmp.Diff(fdp1, fdp2, protocmp.Transform()))
	}
}

// stripSourceCodeInfo returns a copy of the FileDescriptorProto with source_code_info removed.
func stripSourceCodeInfo(fdp *descriptorpb.FileDescriptorProto) *descriptorpb.FileDescriptorProto {
	clone := proto.CloneOf(fdp)
	clone.SourceCodeInfo = nil
	return clone
}
