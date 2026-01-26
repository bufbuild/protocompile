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
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/protoutil"
)

// oldCompilerAdapter wraps the old protocompile.Compiler.
type oldCompilerAdapter struct {
	compiler *protocompile.Compiler
}

// NewOldCompiler creates a new CompilerInterface wrapping the old protocompile.Compiler.
// Use WithResolver option to specify a custom resolver.
func NewOldCompiler(opts ...CompilerOption) CompilerInterface {
	config := &compilerConfig{}
	for _, opt := range opts {
		opt(config)
	}

	compiler := &protocompile.Compiler{
		Resolver: config.resolver,
	}

	if config.sourceInfoMode != 0 {
		compiler.SourceInfoMode = config.sourceInfoMode
	}

	return &oldCompilerAdapter{
		compiler: compiler,
	}
}

// Name implements CompilerInterface.
func (a *oldCompilerAdapter) Name() string {
	return "old_compiler"
}

// Compile implements CompilerInterface.
func (a *oldCompilerAdapter) Compile(ctx context.Context, files ...string) (CompilationResult, error) {
	linkerFiles, err := a.compiler.Compile(ctx, files...)
	if err != nil {
		return nil, err
	}

	return &oldCompilationResult{
		files: linkerFiles,
	}, nil
}

// oldCompilationResult wraps linker.Files.
type oldCompilationResult struct {
	files linker.Files
}

// Files implements CompilationResult.
func (r *oldCompilationResult) Files() []CompiledFile {
	result := make([]CompiledFile, len(r.files))
	for i, file := range r.files {
		result[i] = &oldCompiledFile{file: file}
	}
	return result
}

// oldCompiledFile wraps a linker.File.
type oldCompiledFile struct {
	file linker.File
}

// Path implements CompiledFile.
func (f *oldCompiledFile) Path() string {
	return f.file.Path()
}

// FileDescriptor implements CompiledFile.
func (f *oldCompiledFile) FileDescriptor() (protoreflect.FileDescriptor, error) {
	// linker.File already implements protoreflect.FileDescriptor
	return f.file, nil
}

// FileDescriptorProto implements CompiledFile.
func (f *oldCompiledFile) FileDescriptorProto() (*descriptorpb.FileDescriptorProto, error) {
	return protoutil.ProtoFromFileDescriptor(f.file), nil
}
