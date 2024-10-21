package ast

import (
	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	DeclKindEmpty DeclKind = iota + 1
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
type DeclAny struct {
	// NOTE: These fields are sorted by alignment.
	withContext
	ptr  arena.Untyped
	kind DeclKind
}

// Kind returns the kind of declaration this is. This is suitable for use
// in a switch statement.
func (d DeclAny) Kind() DeclKind {
	return d.kind
}

// AsEmpty converts a DeclAny into a DeclEmpty, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsEmpty() DeclEmpty {
	if d.kind != DeclKindEmpty {
		return DeclEmpty{}
	}

	return wrapDeclEmpty(d, arena.Pointer[rawDeclEmpty](d.ptr))
}

// AsSyntax converts a DeclAny into a DeclSyntax, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsSyntax() DeclSyntax {
	if d.kind != DeclKindSyntax {
		return DeclSyntax{}
	}

	return wrapDeclSyntax(d, arena.Pointer[rawDeclSyntax](d.ptr))
}

// AsPackage converts a DeclAny into a DeclPackage, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsPackage() DeclPackage {
	if d.kind != DeclKindPackage {
		return DeclPackage{}
	}

	return wrapDeclPackage(d, arena.Pointer[rawDeclPackage](d.ptr))
}

// AsImport converts a DeclAny into a DeclImport, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsImport() DeclImport {
	if d.kind != DeclKindImport {
		return DeclImport{}
	}

	return wrapDeclImport(d, arena.Pointer[rawDeclImport](d.ptr))
}

// AsDef converts a DeclAny into a DeclDef, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsDef() DeclDef {
	if d.kind != DeclKindDef {
		return DeclDef{}
	}

	return wrapDeclDef(d, arena.Pointer[rawDeclDef](d.ptr))
}

// AsBody converts a DeclAny into a DeclBody, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsBody() DeclBody {
	if d.kind != DeclKindBody {
		return DeclBody{}
	}

	return wrapDeclBody(d, arena.Pointer[rawDeclBody](d.ptr))
}

// AsRange converts a DeclAny into a DeclRange, if that is the declaration
// it contains.
//
// Otherwise, returns nil.
func (d DeclAny) AsRange() DeclRange {
	if d.kind != DeclKindRange {
		return DeclRange{}
	}

	return wrapDeclRange(d, arena.Pointer[rawDeclRange](d.ptr))
}

// Span implements [Spanner].
func (d DeclAny) Span() Span {
	// At most one of the below will produce a non-nil decl, and that will be
	// the span selected by JoinSpans. If all of them are nil, this produces
	// the nil span.
	return JoinSpans(
		d.AsEmpty(),
		d.AsSyntax(),
		d.AsPackage(),
		d.AsImport(),
		d.AsDef(),
		d.AsBody(),
		d.AsRange(),
	)
}

// declImpl is the common implementation of pointer-like Decl* types.
type declImpl[Raw any] struct {
	// NOTE: These fields are sorted by alignment.
	withContext
	raw  *Raw
	ptr  arena.Pointer[Raw]
	kind DeclKind
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d declImpl[Raw]) AsAny() DeclAny {
	return DeclAny{
		withContext: d.withContext,
		ptr:         d.ptr.Untyped(),
		kind:        d.kind,
	}
}

func wrapDecl[Raw any](c Contextual, ptr arena.Pointer[Raw]) declImpl[Raw] {
	ctx := c.Context()
	if ctx == nil || ptr.Nil() {
		return declImpl[Raw]{}
	}

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
		arena_ = &ctx.decls.empties
	case rawDeclSyntax:
		kind = DeclKindSyntax
		arena_ = &ctx.decls.syntaxes
	case rawDeclPackage:
		kind = DeclKindPackage
		arena_ = &ctx.decls.packages
	case rawDeclImport:
		kind = DeclKindImport
		arena_ = &ctx.decls.imports
	case rawDeclDef:
		kind = DeclKindDef
		arena_ = &ctx.decls.defs
	case rawDeclBody:
		kind = DeclKindBody
		arena_ = &ctx.decls.bodies
	case rawDeclRange:
		kind = DeclKindRange
		arena_ = &ctx.decls.ranges
	default:
		return declImpl[Raw]{}
	}

	return declImpl[Raw]{
		withContext{ctx},
		arena_.(*arena.Arena[Raw]).Deref(ptr),
		ptr,
		kind,
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
