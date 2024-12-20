// Copyright 2020-2024 Buf Technologies, Inc.
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

package ast

import (
	"reflect"

	"github.com/bufbuild/protocompile/experimental/internal"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	DeclKindInvalid DeclKind = iota
	DeclKindEmpty
	DeclKindSyntax
	DeclKindPackage
	DeclKindImport
	DeclKindDef
	DeclKindBody
	DeclKindRange
)

// DeclKind is a kind of declaration. There is one value of DeclKind for each
// Decl* type in this package.
type DeclKind int8

// DeclAny is any Decl* type in this package.
//
// Values of this type can be obtained by calling an AsAny method on a Decl*
// type, such as [DeclSyntax.AsAny]. It can be type-asserted back to any of
// the concrete Decl* types using its own As* methods.
//
// This type is used in lieu of a putative Decl interface type to avoid heap
// allocations in functions that would return one of many different Decl*
// types.
//
// # Grammar
//
//	DeclAny := DeclEmpty | DeclSyntax | DeclPackage | DeclImport | DeclDef | DeclBody | DeclRange
//
// Note that this grammar is highly ambiguous. TODO: document the rules under
// which parse DeclSyntax, DeclPackage, DeclImport, and DeclRange.
type DeclAny struct {
	// NOTE: These fields are sorted by alignment.
	withContext // Must be nil if raw is nil.

	raw rawDecl
}

// Kind returns the kind of declaration this is. This is suitable for use
// in a switch statement.
func (d DeclAny) Kind() DeclKind {
	return d.raw.kind
}

// AsEmpty converts a DeclAny into a DeclEmpty, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsEmpty() DeclEmpty {
	if d.Kind() != DeclKindEmpty {
		return DeclEmpty{}
	}

	return wrapDeclEmpty(d.Context(), arena.Pointer[rawDeclEmpty](d.raw.ptr))
}

// AsSyntax converts a DeclAny into a DeclSyntax, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsSyntax() DeclSyntax {
	if d.Kind() != DeclKindSyntax {
		return DeclSyntax{}
	}

	return wrapDeclSyntax(d.Context(), arena.Pointer[rawDeclSyntax](d.raw.ptr))
}

// AsPackage converts a DeclAny into a DeclPackage, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsPackage() DeclPackage {
	if d.Kind() != DeclKindPackage {
		return DeclPackage{}
	}

	return wrapDeclPackage(d.Context(), arena.Pointer[rawDeclPackage](d.raw.ptr))
}

// AsImport converts a DeclAny into a DeclImport, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsImport() DeclImport {
	if d.Kind() != DeclKindImport {
		return DeclImport{}
	}

	return wrapDeclImport(d.Context(), arena.Pointer[rawDeclImport](d.raw.ptr))
}

// AsDef converts a DeclAny into a DeclDef, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsDef() DeclDef {
	if d.Kind() != DeclKindDef {
		return DeclDef{}
	}

	return wrapDeclDef(d.Context(), arena.Pointer[rawDeclDef](d.raw.ptr))
}

// AsBody converts a DeclAny into a DeclBody, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsBody() DeclBody {
	if d.Kind() != DeclKindBody {
		return DeclBody{}
	}

	return wrapDeclBody(d.Context(), arena.Pointer[rawDeclBody](d.raw.ptr))
}

// AsRange converts a DeclAny into a DeclRange, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsRange() DeclRange {
	if d.Kind() != DeclKindRange {
		return DeclRange{}
	}

	return wrapDeclRange(d.Context(), arena.Pointer[rawDeclRange](d.raw.ptr))
}

// Span implements [report.Spanner].
func (d DeclAny) Span() report.Span {
	// At most one of the below will produce a non-zero decl, and that will be
	// the span selected by report.Join. If all of them are zero, this produces
	// the zero span.
	return report.Join(
		d.AsEmpty(),
		d.AsSyntax(),
		d.AsPackage(),
		d.AsImport(),
		d.AsDef(),
		d.AsBody(),
		d.AsRange(),
	)
}

// rawDecl is the actual data of a DeclAny.
type rawDecl struct {
	ptr  arena.Untyped
	kind DeclKind
}

func (d rawDecl) With(c Context) DeclAny {
	if c == nil || d.ptr.Nil() || d.kind == DeclKindInvalid {
		return DeclAny{}
	}

	return DeclAny{internal.NewWith(c), d}
}

// declImpl is the common implementation of pointer-like Decl* types.
type declImpl[Raw any] struct {
	withContext
	raw *Raw
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d declImpl[Raw]) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}

	kind, arena := declArena[Raw](&d.Context().Nodes().decls)
	return rawDecl{arena.Compress(d.raw).Untyped(), kind}.With(d.Context())
}

func wrapDecl[Raw any](ctx Context, ptr arena.Pointer[Raw]) declImpl[Raw] {
	if ctx == nil || ptr.Nil() {
		return declImpl[Raw]{}
	}

	_, arena := declArena[Raw](&ctx.Nodes().decls)
	return declImpl[Raw]{
		internal.NewWith(ctx),
		arena.Deref(ptr),
	}
}

// decls is storage for every kind of Decl in a Context.
type decls struct {
	empties  arena.Arena[rawDeclEmpty]
	syntaxes arena.Arena[rawDeclSyntax]
	packages arena.Arena[rawDeclPackage]
	imports  arena.Arena[rawDeclImport]
	defs     arena.Arena[rawDeclDef]
	bodies   arena.Arena[rawDeclBody]
	ranges   arena.Arena[rawDeclRange]
}

func declArena[Raw any](decls *decls) (DeclKind, *arena.Arena[Raw]) {
	var (
		kind DeclKind
		raw  Raw
		// Needs to be an any because Go doesn't know that only the case below
		// with the correct type for arena_ (if it were *arena.Arena[Raw]) will
		// be evaluated.
		arena_ any //nolint:revive // Named arena_ to avoid clashing with package arena.
	)

	switch any(raw).(type) {
	case rawDeclEmpty:
		kind = DeclKindEmpty
		arena_ = &decls.empties
	case rawDeclSyntax:
		kind = DeclKindSyntax
		arena_ = &decls.syntaxes
	case rawDeclPackage:
		kind = DeclKindPackage
		arena_ = &decls.packages
	case rawDeclImport:
		kind = DeclKindImport
		arena_ = &decls.imports
	case rawDeclDef:
		kind = DeclKindDef
		arena_ = &decls.defs
	case rawDeclBody:
		kind = DeclKindBody
		arena_ = &decls.bodies
	case rawDeclRange:
		kind = DeclKindRange
		arena_ = &decls.ranges
	default:
		panic("unknown decl type " + reflect.TypeOf(raw).Name())
	}

	return kind, arena_.(*arena.Arena[Raw]) //nolint:errcheck
}
