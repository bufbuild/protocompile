package linker

import (
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
	"github.com/jhump/protocompile/walk"
)

// Symbols is a symbol table that maps names for all program elements to their
// location in source. It also tracks extension tag numbers. This can be used
// to enforce uniqueness for symbol names and tag numbers across many files and
// many link operations.
//
// This type is thread-safe.
type Symbols struct {
	mu      sync.Mutex
	files   map[protoreflect.FileDescriptor]struct{}
	symbols map[protoreflect.FullName]ast.SourcePos
	exts    map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos
}

// Import populates the symbol table with all symbols/elements and extension
// tags present in the given file descriptor. If s is nil or if fd has already
// been imported into s, this returns immediately without doing anything. If any
// collisions in symbol names or extension tags are identified, an error will be
// returned and the symbol table will not be updated.
func (s *Symbols) Import(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	if s == nil {
		return nil
	}

	if f, ok := fd.(file); ok {
		// unwrap any file instance
		fd = f.FileDescriptor
	}

	if res, ok := fd.(*result); ok {
		return s.importResult(res, false, true, handler)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.files[fd]; ok {
		// already imported
		return nil
	}

	// first pass: check for conflicts
	if err := s.checkFileLocked(fd, handler); err != nil {
		return err
	}
	if err := handler.Error(); err != nil {
		return err
	}

	// second pass: commit all symbols
	s.importFileLocked(fd)

	return nil
}

func (s *Symbols) checkFileLocked(f protoreflect.FileDescriptor, handler *reporter.Handler) error {
	return walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		pos := sourcePositionFor(d)
		if existing, ok := s.symbols[d.FullName()]; ok {
			if err := handler.HandleErrorf(pos, "symbol %q already defined at %v", d.FullName(), existing); err != nil {
				return err
			}
		}

		fld, ok := d.(protoreflect.FieldDescriptor)
		if !ok || !fld.IsExtension() {
			return nil
		}

		extendee := fld.ContainingMessage().FullName()
		if tags, ok := s.exts[extendee]; ok {
			if existing, ok := tags[fld.Number()]; ok {
				if err := handler.HandleErrorf(pos, "extension with tag %d for message %s already defined at %v", fld.Number(), extendee, existing); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func sourcePositionFor(d protoreflect.Descriptor) ast.SourcePos {
	//TODO: d.ParentFile().SourceLocations().ByDescriptor(d)
	d.ParentFile()
	return ast.UnknownPos(d.ParentFile().Path())
}

func (s *Symbols) importFileLocked(f protoreflect.FileDescriptor) {
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]ast.SourcePos{}
	}
	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}
	_ = walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		pos := sourcePositionFor(d)
		name := d.FullName()
		s.symbols[name] = pos

		fld, ok := d.(protoreflect.FieldDescriptor)
		if !ok || !fld.IsExtension() {
			return nil
		}

		extendee := fld.ContainingMessage().FullName()
		tags := s.exts[extendee]
		if tags == nil {
			tags = map[protoreflect.FieldNumber]ast.SourcePos{}
			s.exts[extendee] = tags
		}
		tags[fld.Number()] = pos

		return nil
	})

	if s.files == nil {
		s.files = map[protoreflect.FileDescriptor]struct{}{}
	}
	s.files[f] = struct{}{}
}

func (s *Symbols) importResult(r *result, populatePool bool, checkExts bool, handler *reporter.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.files[r]; ok {
		// already imported
		return nil
	}

	// first pass: check for conflicts
	if err := s.checkResultLocked(r, checkExts, handler); err != nil {
		return err
	}
	if err := handler.Error(); err != nil {
		return err
	}

	// second pass: commit all symbols
	s.importResultLocked(r, populatePool)

	return nil
}

func (s *Symbols) checkResultLocked(r *result, checkExts bool, handler *reporter.Handler) error {
	return walk.DescriptorProtos(r.Proto(), func(fqn protoreflect.FullName, d proto.Message) error {
		pos := r.Node(d).Start()
		if existing, ok := s.symbols[fqn]; ok {
			if err := handler.HandleErrorf(pos, "symbol %q already defined at %v", fqn, existing); err != nil {
				return err
			}
		}
		if !checkExts {
			return nil
		}

		fld, ok := d.(*descriptorpb.FieldDescriptorProto)
		if !ok {
			return nil
		}
		extendee := fld.GetExtendee()
		if extendee == "" {
			return nil
		}

		extendeeFqn := protoreflect.FullName(strings.TrimPrefix(extendee, "."))
		if tags, ok := s.exts[extendeeFqn]; ok {
			if existing, ok := tags[protoreflect.FieldNumber(fld.GetNumber())]; ok {
				if err := handler.HandleErrorf(pos, "extension with tag %d for message %s already defined at %v", fld.GetNumber(), extendeeFqn, existing); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *Symbols) importResultLocked(r *result, populatePool bool) {
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]ast.SourcePos{}
	}
	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}
	_ = walk.DescriptorProtos(r.Proto(), func(fqn protoreflect.FullName, d proto.Message) error {
		pos := r.Node(d).Start()
		s.symbols[fqn] = pos
		if populatePool {
			r.descriptorPool[string(fqn)] = d
		}
		return nil
	})

	if s.files == nil {
		s.files = map[protoreflect.FileDescriptor]struct{}{}
	}
	s.files[r] = struct{}{}
}

func (s *Symbols) addExtension(extendee protoreflect.FullName, tag protoreflect.FieldNumber, pos ast.SourcePos, handler *reporter.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}

	usedExtTags := s.exts[extendee]
	if usedExtTags == nil {
		usedExtTags = map[protoreflect.FieldNumber]ast.SourcePos{}
		s.exts[extendee] = usedExtTags
	}
	if existing, ok := usedExtTags[tag]; ok {
		if err := handler.HandleErrorf(pos, "extension with tag %d for message %s already defined at %v", tag, extendee, existing); err != nil {
			return err
		}
	} else {
		usedExtTags[tag] = pos
	}
	return nil
}
