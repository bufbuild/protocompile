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

// Package dualcompiler provides a test abstraction layer for running tests
// with both the old protocompile.Compiler and the new experimental compiler.
package dualcompiler

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
)

// CompilerInterface abstracts the differences between the old protocompile.Compiler
// and the new experimental compiler.
type CompilerInterface interface {
	// Compile compiles the given proto files and returns the compilation result.
	Compile(ctx context.Context, files ...string) (CompilationResult, error)

	// Name returns a descriptive name for this compiler (used in test output).
	Name() string
}

// CompilationResult wraps the output of compilation from either compiler.
type CompilationResult interface {
	// Files returns all compiled files.
	Files() []CompiledFile
}

// CompiledFile represents a single compiled proto file from either compiler.
type CompiledFile interface {
	// Path returns the file path.
	Path() string

	// FileDescriptor returns the file as a protoreflect.FileDescriptor.
	FileDescriptor() (protoreflect.FileDescriptor, error)

	// FileDescriptorProto returns the file as a descriptorpb.FileDescriptorProto.
	// This is the primary method for comparing outputs between compilers.
	FileDescriptorProto() (*descriptorpb.FileDescriptorProto, error)
}

// CompilerOption is a function that configures a compiler adapter.
type CompilerOption func(config *compilerConfig)

// compilerConfig holds configuration for compiler adapters.
type compilerConfig struct {
	// Resolver is the protocompile.Resolver to use for finding proto files.
	// If nil, a default resolver will be used.
	resolver protocompile.Resolver
	// SourceInfoMode controls source code info generation (old compiler only).
	sourceInfoMode protocompile.SourceInfoMode
}

// WithResolver sets the resolver to use for finding proto files.
func WithResolver(resolver protocompile.Resolver) CompilerOption {
	return func(config *compilerConfig) {
		config.resolver = resolver
	}
}

// WithSourceInfoMode sets the source info mode for the old compiler.
func WithSourceInfoMode(mode protocompile.SourceInfoMode) CompilerOption {
	return func(config *compilerConfig) {
		config.sourceInfoMode = mode
	}
}
