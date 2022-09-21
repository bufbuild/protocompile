package linker

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/reporter"
)

// These symbols are only exported to linker_test package (hence in *_test.go file).

type SymbolEntry = symbolEntry
type PackageSymbols = packageSymbols

func (e SymbolEntry) Pos() ast.SourcePos {
	return e.pos
}
func (e SymbolEntry) IsEnumValue() bool {
	return e.isEnumValue
}
func (e SymbolEntry) IsPackage() bool {
	return e.isPackage
}

func (s *Symbols) ImportPackages(pos ast.SourcePos, pkg protoreflect.FullName, handler *reporter.Handler) (*PackageSymbols, error) {
	return s.importPackages(pos, pkg, handler)
}
func (s *Symbols) GetPackage(pkg protoreflect.FullName) *PackageSymbols {
	return s.getPackage(pkg)
}
func (s *Symbols) Packages() *PackageSymbols {
	return &s.pkgTrie
}

func (s *PackageSymbols) Children() map[protoreflect.FullName]*PackageSymbols {
	s.mu.Lock()
	defer s.mu.Unlock()
	ret := make(map[protoreflect.FullName]*PackageSymbols, len(s.children))
	for k, v := range s.children {
		ret[k] = v
	}
	return ret
}
func (s *PackageSymbols) Files() map[protoreflect.FileDescriptor]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ret := make(map[protoreflect.FileDescriptor]struct{}, len(s.files))
	for k, v := range s.files {
		ret[k] = v
	}
	return ret
}
func (s *PackageSymbols) Symbols() map[protoreflect.FullName]SymbolEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	ret := make(map[protoreflect.FullName]SymbolEntry, len(s.symbols))
	for k, v := range s.symbols {
		ret[k] = v
	}
	return ret
}
func (s *PackageSymbols) Extensions() map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos {
	s.mu.Lock()
	defer s.mu.Unlock()
	ret := make(map[protoreflect.FullName]map[protoreflect.FieldNumber]ast.SourcePos, len(s.exts))
	for k, v := range s.exts {
		extNums := make(map[protoreflect.FieldNumber]ast.SourcePos, len(v))
		for num, pos := range v {
			extNums[num] = pos
		}
		ret[k] = extNums
	}
	return ret
}
