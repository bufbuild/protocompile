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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

//go:generate go run github.com/bufbuild/protocompile/internal/enum symbol_kind.yaml

// Symbol is an entry in a [File]'s symbol table.
//
// [Symbol.Context] returns the context for the file in which the symbol was
// defined.
type Symbol struct {
	withContext
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

// Kind returns which kind of symbol this is.
func (s Symbol) Kind() SymbolKind {
	if s.IsZero() {
		return SymbolKindInvalid
	}
	return s.raw.kind
}

// AsType returns the type this symbol refers to, if it is one.
func (s Symbol) AsType() Type {
	if s.Kind() != SymbolKindType {
		return Type{}
	}
	return wrapType(s.Context(), ref[rawType]{
		file: 0, // Symbol context == context of declaring file.
		ptr:  arena.Pointer[rawType](s.raw.data),
	})
}

// AsField returns the field this symbol refers to, if it is one.
func (s Symbol) AsField() Field {
	if s.Kind() != SymbolKindField {
		return Field{}
	}
	return wrapField(s.Context(), ref[rawField]{
		file: 0, // Symbol context == context of declaring file.
		ptr:  arena.Pointer[rawField](s.raw.data),
	})
}

// AsOneof returns the oneof this symbol refers to, if it is one.
func (s Symbol) AsOneof() Oneof {
	if s.Kind() != SymbolKindOneof {
		return Oneof{}
	}
	return wrapOneof(s.Context(), arena.Pointer[rawOneof](s.raw.data))
}

// Definition returns a span for the definition site of this symbol;
// specifically, this is (typically) just an identifier.
func (s Symbol) Definition() report.Span {
	switch s.Kind() {
	case SymbolKindPackage:
		return s.Context().File().AST().Package().Span()
	case SymbolKindType:
		return s.AsType().AST().Name().Span()
	case SymbolKindField:
		return s.AsField().AST().Name().Span()
	case SymbolKindOneof:
		return s.AsOneof().AST().Name().Span()
	}

	return report.Span{}
}

func wrapSymbol(c *Context, r ref[rawSymbol]) Symbol {
	if r.ptr.Nil() || c == nil {
		return Symbol{}
	}

	c = r.context(c)
	return Symbol{
		withContext: internal.NewWith(c),
		raw:         c.arenas.symbols.Deref(r.ptr),
	}
}

// symtab is a symbol table: a mapping of the fully qualified names of symbols
// (stored as [intern.IDs]) to the entities they refer to.
type symtab []ref[rawSymbol]

// sort sorts this symbol table according according to the value of each intern
// ID.
func (s symtab) sort(c *Context) {
	slices.SortFunc(s, func(a, b ref[rawSymbol]) int {
		symA := wrapSymbol(c, a)
		symB := wrapSymbol(c, b)
		return cmp.Compare(symA.InternedFullName(), symB.InternedFullName())
	})
}
