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
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

// newCompilerAdapter wraps the experimental incremental compiler.
type newCompilerAdapter struct {
	executor *incremental.Executor
	opener   source.Opener
	session  *ir.Session
}

// NewNewCompiler creates a new CompilerInterface wrapping the experimental compiler.
// Use WithResolver option to specify a custom resolver.
// The resolver will be converted to an Opener and combined with WKTs.
func NewNewCompiler(opts ...CompilerOption) CompilerInterface {
	config := &compilerConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Create an opener that combines the resolver with WKTs.
	// WKTs are checked first so they're returned as source files, not descriptors.
	var opener source.Opener
	if config.resolver != nil {
		resolverOpener := ResolverToOpener(config.resolver)
		wkts := source.WKTs()
		opener = &source.Openers{wkts, resolverOpener}
	} else {
		opener = source.WKTs()
	}

	return &newCompilerAdapter{
		executor: incremental.New(),
		opener:   opener,
		session:  &ir.Session{},
	}
}

// Name implements CompilerInterface.
func (a *newCompilerAdapter) Name() string {
	return "new_compiler"
}

// Compile implements CompilerInterface.
func (a *newCompilerAdapter) Compile(ctx context.Context, files ...string) (CompilationResult, error) {
	// Create IR queries for each file
	qs := make([]incremental.Query[*ir.File], len(files))
	for i, file := range files {
		qs[i] = queries.IR{
			Opener:  a.opener,
			Session: a.session,
			Path:    file,
		}
	}

	// Run the queries
	results, rpt, err := incremental.Run(ctx, a.executor, qs...)
	if err != nil {
		return nil, err
	}

	// Check for fatal errors in individual results
	irFiles := make([]*ir.File, 0, len(results))
	for i, result := range results {
		if result.Fatal != nil {
			return nil, fmt.Errorf("compilation failed for %s: %w", files[i], result.Fatal)
		}
		irFiles = append(irFiles, result.Value)
	}

	// Check for errors in the report
	for _, diag := range rpt.Diagnostics {
		if diag.Level() == report.Error || diag.Level() == report.ICE {
			return nil, fmt.Errorf("%v", diag)
		}
	}

	return &newCompilationResult{
		files: irFiles,
	}, nil
}

// newCompilationResult wraps IR files.
type newCompilationResult struct {
	files []*ir.File
}

// Files implements CompilationResult.
func (r *newCompilationResult) Files() []CompiledFile {
	result := make([]CompiledFile, len(r.files))
	for i, file := range r.files {
		result[i] = &newCompiledFile{
			file: file,
		}
	}
	return result
}

// newCompiledFile wraps an ir.File.
type newCompiledFile struct {
	file *ir.File
}

// Path implements CompiledFile.
func (f *newCompiledFile) Path() string {
	return f.file.Path()
}

// FileDescriptor implements CompiledFile.
// Converts the FileDescriptorProto to a FileDescriptor using protodesc.
// Dependencies are resolved using the global registry (includes WKTs and other registered files).
func (f *newCompiledFile) FileDescriptor() (protoreflect.FileDescriptor, error) {
	fdp, err := f.FileDescriptorProto()
	if err != nil {
		return nil, err
	}

	return protodesc.NewFile(fdp, protoregistry.GlobalFiles)
}

// FileDescriptorProto implements CompiledFile.
func (f *newCompiledFile) FileDescriptorProto() (*descriptorpb.FileDescriptorProto, error) {
	data, err := ir.DescriptorProtoBytes(f.file)
	if err != nil {
		return nil, err
	}

	fdp := &descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(data, fdp); err != nil {
		return nil, err
	}

	return fdp, nil
}
