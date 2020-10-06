package protocompile

import (
	"io"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
)

type Resolver interface {
	FindFileByPath(string) (SearchResult, error)
}

type SearchResult struct {
	// only one of the following must be set, based on what
	// the resolver is able to find/produce; if multiple are set,
	// compiler prefers them in opposite order listed: so it uses
	// descriptor if present and only falls back to source if nothing
	// else if available
	Source io.Reader
	AST    *ast.FileNode
	Proto  *descriptorpb.FileDescriptorProto
	Desc   protoreflect.FileDescriptor
}

type ResolverFunc func(string) (SearchResult, error)

var _ Resolver = ResolverFunc(nil)

func (f ResolverFunc) FindFileByPath(path string) (SearchResult, error) {
	return f(path)
}

type CompositeResolver []Resolver

var _ Resolver = CompositeResolver(nil)

func (f CompositeResolver) FindFileByPath(path string) (SearchResult, error) {
	if len(f) == 0 {
		return SearchResult{}, protoregistry.NotFound
	}
	var firstErr error
	for _, res := range f {
		r, err := res.FindFileByPath(path)
		if err == nil {
			return r, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return SearchResult{}, firstErr
}

type SourceResolver struct {
	ImportPaths []string
	Accessor    func(string) (io.ReadCloser, error)
}

var _ Resolver = (*SourceResolver)(nil)

func (r *SourceResolver) FindFileByPath(path string) (SearchResult, error) {
	if len(r.ImportPaths) == 0 {
		reader, err := r.Accessor(path)
		if err != nil {
			return SearchResult{}, err
		}
		return SearchResult{Source: reader}, nil
	}

	var e error
	for _, importPath := range r.ImportPaths {
		reader, err := r.Accessor(filepath.Join(importPath, path))
		if err != nil {
			if os.IsNotExist(err) {
				e = err
				continue
			}
			return SearchResult{}, err
		}
		return SearchResult{Source: reader}, nil
	}
	return SearchResult{}, e
}