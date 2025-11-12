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

package ast

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/arena"
)

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
type DeclAny id.DynNode[DeclAny, DeclKind, *File]

// AsEmpty converts a DeclAny into a DeclEmpty, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsEmpty() DeclEmpty {
	if d.Kind() != DeclKindEmpty {
		return DeclEmpty{}
	}

	return id.Wrap(d.Context(), id.ID[DeclEmpty](d.ID().Value()))
}

// AsSyntax converts a DeclAny into a DeclSyntax, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsSyntax() DeclSyntax {
	if d.Kind() != DeclKindSyntax {
		return DeclSyntax{}
	}

	return id.Wrap(d.Context(), id.ID[DeclSyntax](d.ID().Value()))
}

// AsPackage converts a DeclAny into a DeclPackage, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsPackage() DeclPackage {
	if d.Kind() != DeclKindPackage {
		return DeclPackage{}
	}

	return id.Wrap(d.Context(), id.ID[DeclPackage](d.ID().Value()))
}

// AsImport converts a DeclAny into a DeclImport, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsImport() DeclImport {
	if d.Kind() != DeclKindImport {
		return DeclImport{}
	}

	return id.Wrap(d.Context(), id.ID[DeclImport](d.ID().Value()))
}

// AsDef converts a DeclAny into a DeclDef, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsDef() DeclDef {
	if d.Kind() != DeclKindDef {
		return DeclDef{}
	}

	return id.Wrap(d.Context(), id.ID[DeclDef](d.ID().Value()))
}

// AsBody converts a DeclAny into a DeclBody, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsBody() DeclBody {
	if d.Kind() != DeclKindBody {
		return DeclBody{}
	}

	return id.Wrap(d.Context(), id.ID[DeclBody](d.ID().Value()))
}

// AsRange converts a DeclAny into a DeclRange, if that is the declaration
// it contains.
//
// Otherwise, returns zero.
func (d DeclAny) AsRange() DeclRange {
	if d.Kind() != DeclKindRange {
		return DeclRange{}
	}

	return id.Wrap(d.Context(), id.ID[DeclRange](d.ID().Value()))
}

// Span implements [source.Spanner].
func (d DeclAny) Span() source.Span {
	// At most one of the below will produce a non-zero decl, and that will be
	// the span selected by source.Join. If all of them are zero, this produces
	// the zero span.
	return source.Join(
		d.AsEmpty(),
		d.AsSyntax(),
		d.AsPackage(),
		d.AsImport(),
		d.AsDef(),
		d.AsBody(),
		d.AsRange(),
	)
}

func (DeclKind) DecodeDynID(lo, _ int32) DeclKind {
	return DeclKind(lo)
}

func (k DeclKind) EncodeDynID(value int32) (int32, int32, bool) {
	return int32(k), value, true
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
