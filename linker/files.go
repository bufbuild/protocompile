package linker

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/jhump/protocompile/walk"
)

// File is a protoreflect.FileDescriptor that includes some helpful methods for
// looking up elements in the descriptor.
type File interface {
	protoreflect.FileDescriptor
	// FindDescriptorByName returns the given named element that is defined in
	// this file. If no such element exists, nil is returned.
	FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor
	// FindImportsByPath returns the File corresponding to the given import path.
	// If this file does not import the given path, nil is returned.
	FindImportByPath(path string) File
}

// NewFile converts a protoreflect.FileDescriptor to a File. The given deps must
// contain all dependencies/imports of f. Also see NewFileRecursive.
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
	descs := map[protoreflect.FullName]protoreflect.Descriptor{}
	err := walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		if _, ok := descs[d.FullName()]; ok {
			return fmt.Errorf("file %q contains multiple elements with the name %s", f.Path(), d.FullName())
		}
		descs[d.FullName()] = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return file{
		FileDescriptor: f,
		descs:          descs,
		deps:           deps,
	}, nil
}

// NewFileRecursive recursively converts a protoreflect.FileDescriptor to a File.
// If f has any dependencies/imports, they are converted, too, including any and
// all transitive dependencies.
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
	descs map[protoreflect.FullName]protoreflect.Descriptor
	deps Files
}

func (f file) FindDescriptorByName(name protoreflect.FullName) protoreflect.Descriptor {
	return f.descs[name]
}

func (f file) FindImportByPath(path string) File {
	return f.deps.FindFileByPath(path)
}

var _ File = file{}

// Files represents a set of protobuf files. It is a slice of File values, but
// also provides a method for easily looking up files by path and name.
type Files []File

// FindFileByPath finds a file in f that has the given path and name. If f
// contains no such file, nil is returned.
func (f Files) FindFileByPath(path string) File {
	for _, file := range f {
		if file.Path() == path {
			return file
		}
	}
	return nil
}
