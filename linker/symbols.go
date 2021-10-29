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
	symbols map[protoreflect.FullName]symbolEntry
	exts    map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos
}

type symbolEntry struct {
	pos         ast.SourcePos
	isEnumValue bool
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

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.importLocked(fd, handler)
}

func (s *Symbols) importLocked(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	if _, ok := s.files[fd]; ok {
		// already imported
		return nil
	}

	// make sure deps are imported
	for i := 0; i < fd.Imports().Len(); i++ {
		imp := fd.Imports().Get(i)
		if err := s.importLocked(imp.FileDescriptor, handler); err != nil {
			return err
		}
	}

	if res, ok := fd.(*result); ok {
		return s.importResultLocked(res, false, true, handler)
	}

	// first pass: check for conflicts
	if err := s.checkFileLocked(fd, handler); err != nil {
		return err
	}
	if err := handler.Error(); err != nil {
		return err
	}

	// second pass: commit all symbols
	s.commitFileLocked(fd)

	return nil
}

func reportSymbolCollision(pos ast.SourcePos, fqn protoreflect.FullName, additionIsEnumVal bool, existing symbolEntry, handler *reporter.Handler) error {
	// because of weird scoping for enum values, provide more context in error message
	// if this conflict is with an enum value
	var suffix string
	if additionIsEnumVal || existing.isEnumValue {
		suffix = "; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum"
	}
	return handler.HandleErrorf(pos, "symbol %q already defined at %v%s", fqn, existing.pos, suffix)
}

func (s *Symbols) checkFileLocked(f protoreflect.FileDescriptor, handler *reporter.Handler) error {
	return walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		pos := sourcePositionFor(d)
		if existing, ok := s.symbols[d.FullName()]; ok {
			_, isEnumVal := d.(protoreflect.EnumValueDescriptor)
			if err := reportSymbolCollision(pos, d.FullName(), isEnumVal, existing, handler); err != nil {
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
	loc := d.ParentFile().SourceLocations().ByDescriptor(d)
	if isZeroLoc(loc) {
		return ast.UnknownPos(d.ParentFile().Path())
	}
	return ast.SourcePos{
		Filename: d.ParentFile().Path(),
		Line:     loc.StartLine,
		Col:      loc.StartColumn,
	}
}

func isZeroLoc(loc protoreflect.SourceLocation) bool {
	return loc.Path == nil &&
		loc.StartLine == 0 &&
		loc.StartColumn == 0 &&
		loc.EndLine == 0 &&
		loc.EndColumn == 0
}

func (s *Symbols) commitFileLocked(f protoreflect.FileDescriptor) {
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]symbolEntry{}
	}
	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}
	_ = walk.Descriptors(f, func(d protoreflect.Descriptor) error {
		pos := sourcePositionFor(d)
		name := d.FullName()
		_, isEnumValue := d.(protoreflect.EnumValueDescriptor)
		s.symbols[name] = symbolEntry{pos: pos, isEnumValue: isEnumValue}

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

	return s.importResultLocked(r, populatePool, checkExts, handler)
}

func (s *Symbols) importResultLocked(r *result, populatePool bool, checkExts bool, handler *reporter.Handler) error {
	// first pass: check for conflicts
	if err := s.checkResultLocked(r, checkExts, handler); err != nil {
		return err
	}
	if err := handler.Error(); err != nil {
		return err
	}

	// second pass: commit all symbols
	s.commitResultLocked(r, populatePool)

	return nil
}

func (s *Symbols) checkResultLocked(r *result, checkExts bool, handler *reporter.Handler) error {
	resultSyms := map[protoreflect.FullName]symbolEntry{}
	return walk.DescriptorProtos(r.Proto(), func(fqn protoreflect.FullName, d proto.Message) error {
		_, isEnumVal := d.(*descriptorpb.EnumValueDescriptorProto)
		file := r.FileNode()
		node := r.Node(d)
		pos := nameStart(file, node)
		// check symbols already in this symbol table
		if existing, ok := s.symbols[fqn]; ok {
			if err := reportSymbolCollision(pos, fqn, isEnumVal, existing, handler); err != nil {
				return err
			}
		}

		// also check symbols from this result (that are not yet in symbol table)
		if existing, ok := resultSyms[fqn]; ok {
			if err := reportSymbolCollision(pos, fqn, isEnumVal, existing, handler); err != nil {
				return err
			}
		}
		resultSyms[fqn] = symbolEntry{
			pos:         pos,
			isEnumValue: isEnumVal,
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
				pos := file.NodeInfo(node.(ast.FieldDeclNode).FieldTag()).Start()
				if err := handler.HandleErrorf(pos, "extension with tag %d for message %s already defined at %v", fld.GetNumber(), extendeeFqn, existing); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func nameStart(file ast.FileDeclNode, n ast.Node) ast.SourcePos {
	// TODO: maybe ast package needs a NamedNode interface to simplify this?
	switch n := n.(type) {
	case ast.FieldDeclNode:
		return file.NodeInfo(n.FieldName()).Start()
	case ast.MessageDeclNode:
		return file.NodeInfo(n.MessageName()).Start()
	case ast.EnumValueDeclNode:
		return file.NodeInfo(n.GetName()).Start()
	case *ast.EnumNode:
		return file.NodeInfo(n.Name).Start()
	case *ast.ServiceNode:
		return file.NodeInfo(n.Name).Start()
	case ast.RPCDeclNode:
		return file.NodeInfo(n.GetName()).Start()
	default:
		return file.NodeInfo(n).Start()
	}
}

func (s *Symbols) commitResultLocked(r *result, populatePool bool) {
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]symbolEntry{}
	}
	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}
	_ = walk.DescriptorProtos(r.Proto(), func(fqn protoreflect.FullName, d proto.Message) error {
		pos := nameStart(r.FileNode(), r.Node(d))
		_, isEnumValue := d.(protoreflect.EnumValueDescriptor)
		s.symbols[fqn] = symbolEntry{pos: pos, isEnumValue: isEnumValue}
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
