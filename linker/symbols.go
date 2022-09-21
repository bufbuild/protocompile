// Copyright 2020-2022 Buf Technologies, Inc.
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

package linker

import (
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/reporter"
	"github.com/bufbuild/protocompile/walk"
)

// Symbols is a symbol table that maps names for all program elements to their
// location in source. It also tracks extension tag numbers. This can be used
// to enforce uniqueness for symbol names and tag numbers across many files and
// many link operations.
//
// This type is thread-safe.
type Symbols struct {
	mu      sync.RWMutex
	files   map[protoreflect.FileDescriptor]struct{}
	symbols map[protoreflect.FullName]symbolEntry
	exts    map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos
}

type symbolEntry struct {
	pos         ast.SourcePos
	isEnumValue bool
	isPackage   bool
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

	s.mu.RLock()
	_, alreadyImported := s.files[fd]
	s.mu.RUnlock()

	if alreadyImported {
		return nil
	}

	for i := 0; i < fd.Imports().Len(); i++ {
		if err := s.Import(fd.Imports().Get(i).FileDescriptor, handler); err != nil {
			return err
		}
	}

	if res, ok := fd.(*result); ok {
		return s.importResultWithExtensions(res, handler)
	}

	return s.importFileWithExtensions(fd, handler)
}

func (s *Symbols) importFileWithExtensions(fd protoreflect.FileDescriptor, handler *reporter.Handler) error {
	imported, err := s.importFile(fd, handler)
	if err != nil {
		return err
	}
	if !imported {
		// nothing else to do
		return nil
	}

	return walk.Descriptors(fd, func(d protoreflect.Descriptor) error {
		fld, ok := d.(protoreflect.FieldDescriptor)
		if !ok || !fld.IsExtension() {
			return nil
		}
		pos := sourcePositionForNumber(fld)
		if err := s.addExtension(fld.ContainingMessage().FullName(), fld.Number(), pos, handler); err != nil {
			return err
		}
		return nil
	})
}

func (s *Symbols) importFile(fd protoreflect.FileDescriptor, handler *reporter.Handler) (bool, error) {
	if err := s.importPackages(sourcePositionForPackage(fd), fd.Package(), handler); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.files[fd]; ok {
		// have to double-check if it's already imported, in case
		// it was added after above read-locked check
		return false, nil
	}

	// first pass: check for conflicts
	if err := s.checkFileLocked(fd, handler); err != nil {
		return false, err
	}
	if err := handler.Error(); err != nil {
		return false, err
	}

	// second pass: commit all symbols
	s.commitFileLocked(fd)

	return true, nil
}

func (s *Symbols) importPackages(pos ast.SourcePos, pkg protoreflect.FullName, handler *reporter.Handler) error {
	parts := strings.Split(string(pkg), ".")
	for i := 1; i < len(parts); i++ {
		parts[i] = parts[i-1] + "." + parts[i]
	}

	for _, p := range parts {
		if err := s.importPackage(pos, protoreflect.FullName(p), handler); err != nil {
			return err
		}
	}

	return nil
}

func (s *Symbols) importPackage(pos ast.SourcePos, pkg protoreflect.FullName, handler *reporter.Handler) error {
	s.mu.RLock()
	existing, ok := s.symbols[pkg]
	s.mu.RUnlock()
	if ok && existing.isPackage {
		// package already exists
		return nil
	} else if ok {
		return reportSymbolCollision(pos, pkg, false, existing, handler)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// have to double-check in case it was added while upgrading to write lock
	existing, ok = s.symbols[pkg]
	if ok && existing.isPackage {
		// package already exists
		return nil
	} else if ok {
		return reportSymbolCollision(pos, pkg, false, existing, handler)
	}
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]symbolEntry{}
	}
	s.symbols[pkg] = symbolEntry{pos: pos, isPackage: true}
	return nil
}

func reportSymbolCollision(pos ast.SourcePos, fqn protoreflect.FullName, additionIsEnumVal bool, existing symbolEntry, handler *reporter.Handler) error {
	// because of weird scoping for enum values, provide more context in error message
	// if this conflict is with an enum value
	var isPkg, suffix string
	if additionIsEnumVal || existing.isEnumValue {
		suffix = "; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum"
	}
	if existing.isPackage {
		isPkg = " as a package"
	}
	orig := existing.pos
	conflict := pos
	if posLess(conflict, orig) {
		orig, conflict = conflict, orig
	}
	return handler.HandleErrorf(conflict, "symbol %q already defined%s at %v%s", fqn, isPkg, orig, suffix)
}

func posLess(a, b ast.SourcePos) bool {
	if a.Filename == b.Filename {
		if a.Line == b.Line {
			return a.Col < b.Col
		}
		return a.Line < b.Line
	}
	return false
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
		return nil
	})
}

func sourcePositionForPackage(fd protoreflect.FileDescriptor) ast.SourcePos {
	loc := fd.SourceLocations().ByPath([]int32{internal.FilePackageTag})
	if isZeroLoc(loc) {
		return ast.UnknownPos(fd.Path())
	}
	return ast.SourcePos{
		Filename: fd.Path(),
		Line:     loc.StartLine,
		Col:      loc.StartColumn,
	}
}

func sourcePositionFor(d protoreflect.Descriptor) ast.SourcePos {
	path, ok := computePath(d)
	if !ok {
		return ast.UnknownPos(d.ParentFile().Path())
	}
	var namePath protoreflect.SourcePath
	switch d.(type) {
	case protoreflect.FieldDescriptor:
		namePath = append(path, internal.FieldNameTag)
	case protoreflect.MessageDescriptor:
		namePath = append(path, internal.MessageNameTag)
	case protoreflect.OneofDescriptor:
		namePath = append(path, internal.OneOfNameTag)
	case protoreflect.EnumDescriptor:
		namePath = append(path, internal.EnumNameTag)
	case protoreflect.EnumValueDescriptor:
		namePath = append(path, internal.EnumValNameTag)
	case protoreflect.ServiceDescriptor:
		namePath = append(path, internal.ServiceNameTag)
	case protoreflect.MethodDescriptor:
		namePath = append(path, internal.MethodNameTag)
	default:
		// NB: shouldn't really happen, but just in case fall back to path to
		// descriptor, sans name field
		namePath = path
	}
	loc := d.ParentFile().SourceLocations().ByPath(namePath)
	if isZeroLoc(loc) {
		loc = d.ParentFile().SourceLocations().ByPath(path)
		if isZeroLoc(loc) {
			return ast.UnknownPos(d.ParentFile().Path())
		}
	}
	return ast.SourcePos{
		Filename: d.ParentFile().Path(),
		Line:     loc.StartLine,
		Col:      loc.StartColumn,
	}
}

func sourcePositionForNumber(fd protoreflect.FieldDescriptor) ast.SourcePos {
	path, ok := computePath(fd)
	if !ok {
		return ast.UnknownPos(fd.ParentFile().Path())
	}
	numberPath := append(path, internal.FieldNumberTag)
	loc := fd.ParentFile().SourceLocations().ByPath(numberPath)
	if isZeroLoc(loc) {
		loc = fd.ParentFile().SourceLocations().ByPath(path)
		if isZeroLoc(loc) {
			return ast.UnknownPos(fd.ParentFile().Path())
		}
	}
	return ast.SourcePos{
		Filename: fd.ParentFile().Path(),
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
		return nil
	})

	if s.files == nil {
		s.files = map[protoreflect.FileDescriptor]struct{}{}
	}
	s.files[f] = struct{}{}
}

func (s *Symbols) importResultWithExtensions(r *result, handler *reporter.Handler) error {
	imported, err := s.importResult(r, handler)
	if err != nil {
		return err
	}
	if !imported {
		// nothing else to do
		return nil
	}

	return walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
		fd, ok := d.(*descriptorpb.FieldDescriptorProto)
		if !ok || fd.GetExtendee() == "" {
			return nil
		}
		file := r.FileNode()
		node := r.FieldNode(fd)
		pos := file.NodeInfo(node.FieldTag()).Start()

		extendeeFqn := protoreflect.FullName(strings.TrimPrefix(fd.GetExtendee(), "."))
		if err := s.addExtension(extendeeFqn, protoreflect.FieldNumber(fd.GetNumber()), pos, handler); err != nil {
			return err
		}

		return nil
	})
}

func (s *Symbols) importResult(r *result, handler *reporter.Handler) (bool, error) {
	if err := s.importPackages(packageNameStart(r), r.Package(), handler); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.files[r]; ok {
		// already imported
		return false, nil
	}

	// first pass: check for conflicts
	if err := s.checkResultLocked(r, handler); err != nil {
		return false, err
	}
	if err := handler.Error(); err != nil {
		return false, err
	}

	// second pass: commit all symbols
	s.commitResultLocked(r)

	return true, nil
}

func (s *Symbols) checkResultLocked(r *result, handler *reporter.Handler) error {
	resultSyms := map[protoreflect.FullName]symbolEntry{}
	return walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
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

		return nil
	})
}

func packageNameStart(r *result) ast.SourcePos {
	if node, ok := r.FileNode().(*ast.FileNode); ok {
		for _, decl := range node.Decls {
			if pkgNode, ok := decl.(*ast.PackageNode); ok {
				return r.FileNode().NodeInfo(pkgNode.Name).Start()
			}
		}
	}
	return ast.UnknownPos(r.Path())
}

func nameStart(file ast.FileDeclNode, n ast.Node) ast.SourcePos {
	// TODO: maybe ast package needs a NamedNode interface to simplify this?
	switch n := n.(type) {
	case ast.FieldDeclNode:
		return file.NodeInfo(n.FieldName()).Start()
	case ast.MessageDeclNode:
		return file.NodeInfo(n.MessageName()).Start()
	case ast.OneOfDeclNode:
		return file.NodeInfo(n.OneOfName()).Start()
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

func (s *Symbols) commitResultLocked(r *result) {
	if s.symbols == nil {
		s.symbols = map[protoreflect.FullName]symbolEntry{}
	}
	if s.exts == nil {
		s.exts = map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos{}
	}
	_ = walk.DescriptorProtos(r.FileDescriptorProto(), func(fqn protoreflect.FullName, d proto.Message) error {
		pos := nameStart(r.FileNode(), r.Node(d))
		_, isEnumValue := d.(protoreflect.EnumValueDescriptor)
		s.symbols[fqn] = symbolEntry{pos: pos, isEnumValue: isEnumValue}
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

	return s.addExtensionLocked(extendee, tag, pos, handler)
}

func (s *Symbols) addExtensionLocked(extendee protoreflect.FullName, tag protoreflect.FieldNumber, pos ast.SourcePos, handler *reporter.Handler) error {
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
