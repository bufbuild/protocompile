package linker

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type File interface {
	protoreflect.FileDescriptor
	FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor
	FindImportByPath(string) File
}

func NewFile(f protoreflect.FileDescriptor, deps Files) (File, error) {
	for i := 0; i < f.Imports().Len(); i++ {
		imprt := f.Imports().Get(i)
		dep := deps.FindFileByPath(imprt.Path())
		if dep == nil {
			return nil, fmt.Errorf("cannot create File for %q: missing dependency for %q", f.Path(), imprt.Path())
		}
	}
	return newFile(f, deps)
}

func newFile(f protoreflect.FileDescriptor, deps Files) (File, error) {
	var reg protoregistry.Files
	if err := reg.RegisterFile(f); err != nil {
		return nil, err
	}
	return file{
		FileDescriptor: f,
		res:            &reg,
		deps:           deps,
	}, nil
}

func NewFileRecursive(f protoreflect.FileDescriptor) (File, error) {
	if file, ok := f.(File); ok {
		return file, nil
	}
	return newFileRecursive(f, map[protoreflect.FileDescriptor]File{})
}

func newFileRecursive(fd protoreflect.FileDescriptor, seen map[protoreflect.FileDescriptor]File) (File, error) {
	if res, ok := seen[fd]; ok {
		if res == nil {
			return nil, fmt.Errorf("import cycle encountered: file %s transitively imports itself", fd.Path())
		}
		return res, nil
	}

	if f, ok := fd.(File); ok {
		seen[fd] = f
		return f, nil
	}

	seen[fd] = nil
	deps := make([]File, fd.Imports().Len())
	for i := 0; i < fd.Imports().Len(); i++ {
		imprt := fd.Imports().Get(i)
		dep, err := newFileRecursive(imprt, seen)
		if err != nil {
			return nil, err
		}
		deps[i] = dep
	}

	f, err := newFile(fd, deps)
	if err != nil {
		return nil, err
	}
	seen[fd] = f
	return f, nil
}

type file struct {
	protoreflect.FileDescriptor
	res  protodesc.Resolver
	deps Files
}

func (f file) FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor {
	d, err := f.res.FindDescriptorByName(name)
	if err != nil {
		return nil
	}
	return d
}

func (f file) FindImportByPath(path string) File {
	return f.deps.FindFileByPath(path)
}

var _ File = file{}

type Files []File

func (f Files) FindFileByPath(path string) File {
	for _, file := range f {
		if file.Path() == path {
			return file
		}
	}
	return nil
}