package ast

import (
	"github.com/bufbuild/protocompile/internal/arena"
)

// Decl is a Protobuf declaration.
//
// This is implemented by types in this package of the form Decl*.
type Decl interface {
	Spanner

	// declRaw splits up a Decl into its raw parts.
	declRaw() (declKind, arena.Untyped)
}

// DeclEmpty is an empty declaration, a lone ;.
type DeclEmpty struct {
	withContext
	ptr arena.Pointer[rawDeclEmpty]
	raw *rawDeclEmpty
}

type rawDeclEmpty struct {
	semi rawToken
}

// Semicolon returns this field's ending semicolon.
//
// May be nil, if not present.
func (e DeclEmpty) Semicolon() Token {
	return e.raw.semi.With(e)
}

// Span implements [Spanner].
func (e DeclEmpty) Span() Span {
	return e.Semicolon().Span()
}

func (e DeclEmpty) declRaw() (declKind, arena.Untyped) {
	return declEmpty, arena.Untyped(e.ptr)
}

func wrapDeclEmpty(c Contextual, ptr arena.Pointer[rawDeclEmpty]) DeclEmpty {
	ctx := c.Context()
	if ctx == nil || ptr.Nil() {
		return DeclEmpty{}
	}

	return DeclEmpty{
		withContext{ctx},
		ptr,
		ctx.decls.empties.Deref(ptr),
	}
}

const (
	declEmpty declKind = iota + 1
	declSyntax
	declPackage
	declImport
	declDef
	declScope
	declRange
)

type declKind int8

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
