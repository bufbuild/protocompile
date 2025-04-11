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

package ir

import (
	"cmp"
	"slices"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum symbol_kind.yaml

// Symbol is an entry in a [File]'s symbol table.
//
// [Symbol.Context] returns the context for the file which imported this
// symbol. To map this to the context in which the symbol was defined, use
// [Symbol.InDefFile].
type Symbol struct {
	withContext
	ref ref[rawSymbol]
	raw *rawSymbol
}

type rawSymbol struct {
	kind SymbolKind
	fqn  intern.ID
	data arena.Untyped
}

// FullName returns this symbol's fully-qualified name.
func (s Symbol) FullName() FullName {
	if s.IsZero() {
		return ""
	}
	return FullName(s.Context().session.intern.Value(s.raw.fqn))
}

// InternedName returns the intern ID for [Symbol.FullName].
func (s Symbol) InternedFullName() intern.ID {
	if s.IsZero() {
		return 0
	}
	return s.raw.fqn
}

// InDefFile returns this symbol with its context set to that of its defining
// file.
func (s Symbol) InDefFile() Symbol {
	c := s.ref.context(s.Context())
	s.withContext = internal.NewWith(c)
	s.ref.file = 0 // Now points to the current file.
	return s
}

// File returns the file in which this symbol was defined.
func (s Symbol) File() File {
	return s.ref.context(s.Context()).File()
}

// Kind returns which kind of symbol this is.
func (s Symbol) Kind() SymbolKind {
	if s.IsZero() {
		return SymbolKindInvalid
	}
	return s.raw.kind
}

// AsType returns the type this symbol refers to, if it is one.
func (s Symbol) AsType() Type {
	if !s.Kind().IsType() {
		return Type{}
	}
	return wrapType(s.InDefFile().Context(), ref[rawType]{
		file: 0, // Symbol context == context of declaring file.
		ptr:  arena.Pointer[rawType](s.raw.data),
	})
}

// AsField returns the field this symbol refers to, if it is one.
func (s Symbol) AsField() Field {
	if !s.Kind().IsField() {
		return Field{}
	}
	return wrapField(s.InDefFile().Context(), ref[rawField]{
		file: 0, // Symbol context == context of declaring file.
		ptr:  arena.Pointer[rawField](s.raw.data),
	})
}

// AsOneof returns the oneof this symbol refers to, if it is one.
func (s Symbol) AsOneof() Oneof {
	if s.Kind() != SymbolKindOneof {
		return Oneof{}
	}
	return wrapOneof(s.InDefFile().Context(), arena.Pointer[rawOneof](s.raw.data))
}

// Visible returns whether or not this symbol is visible according to Protobuf's
// import semantics, within s.Context().File().
func (s Symbol) Visible() bool {
	return s.ref.file <= 0 ||
		s.Kind() == SymbolKindPackage || // Packages don't get visibility checks.
		s.Context().imports.visible.Test(uint(s.ref.file)-1)
}

// Definition returns a span for the definition site of this symbol;
// specifically, this is (typically) just an identifier.
func (s Symbol) Definition() report.Span {
	switch s.Kind() {
	case SymbolKindPackage:
		return s.File().AST().Package().Span()
	case SymbolKindMessage, SymbolKindEnum:
		return s.AsType().AST().Name().Span()
	case SymbolKindField, SymbolKindEnumValue, SymbolKindExtension:
		return s.AsField().AST().Name().Span()
	case SymbolKindOneof:
		return s.AsOneof().AST().Name().Span()
	}

	return report.Span{}
}

// noun returns a [taxa.Noun] for diagnostic use.
func (k SymbolKind) noun() taxa.Noun {
	return symbolNouns[k]
}

var symbolNouns = [...]taxa.Noun{
	SymbolKindPackage:   taxa.Package,
	SymbolKindScalar:    taxa.ScalarType,
	SymbolKindMessage:   taxa.MessageType,
	SymbolKindEnum:      taxa.EnumType,
	SymbolKindField:     taxa.Field,
	SymbolKindEnumValue: taxa.EnumValue,
	SymbolKindExtension: taxa.Extension,
	SymbolKindOneof:     taxa.Oneof,
}

// IsType returns whether this is a type's symbol kind.
func (k SymbolKind) IsType() bool {
	switch k {
	case SymbolKindMessage, SymbolKindEnum, SymbolKindScalar:
		return true
	default:
		return false
	}
}

// IsType returns whether this is a field's symbol kind. This includes
// enum values, which the ir package treats as fields of enum types.
func (k SymbolKind) IsField() bool {
	switch k {
	case SymbolKindField, SymbolKindExtension, SymbolKindEnumValue:
		return true
	default:
		return false
	}
}

// IsScope returns whether this is a symbol that defines a scope, for the
// purposes of name lookup.
func (k SymbolKind) IsScope() bool {
	switch k {
	case SymbolKindPackage, SymbolKindMessage:
		return true
	default:
		return false
	}
}

func wrapSymbol(c *Context, r ref[rawSymbol]) Symbol {
	if r.ptr.Nil() || c == nil {
		return Symbol{}
	}

	return Symbol{
		withContext: internal.NewWith(c),
		ref:         r,
		raw:         r.context(c).arenas.symbols.Deref(r.ptr),
	}
}

// symtab is a symbol table: a mapping of the fully qualified names of symbols
// to the entities they refer to.
//
// The elements of a symtab are sorted by the [intern.ID] of their FQN, allowing
// for O(n) merging of symbol tables.
type symtab []ref[rawSymbol]

// sort sorts this symbol table according to the value of each intern
// ID.
func (s symtab) sort(c *Context) {
	slices.SortFunc(s, func(a, b ref[rawSymbol]) int {
		symA := wrapSymbol(c, a)
		symB := wrapSymbol(c, b)
		return cmp.Compare(symA.InternedFullName(), symB.InternedFullName())
	})
}
