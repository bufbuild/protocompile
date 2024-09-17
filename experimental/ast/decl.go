package ast

import (
	"fmt"

	"github.com/bufbuild/protocompile/internal/arena"
)

const (
	declEmpty declKind = iota + 1
	declPragma
	declPackage
	declImport
	declDef
	declBody
	declRange
)

// Decl is a Protobuf declaration.
//
// This is implemented by types in this package of the form Decl*.
type Decl interface {
	Spanner

	// with should be called on a nil value of this type (not
	// a nil interface) and return the corresponding value of this type
	// extracted from the given context and index.
	//
	// Not to be called directly; see rawDecl[T].With().
	with(ctx *Context, ptr arena.Untyped) Decl

	// kind returns what kind of decl this is.
	declKind() declKind
	// declIndex returns the untyped arena pointer for this declaration.
	declIndex() arena.Untyped
}

// decls is storage for every kind of Decl in a Context.
type decls struct {
	empties  arena.Arena[rawDeclEmpty]
	syntaxes arena.Arena[rawDeclSyntax]
	packages arena.Arena[rawDeclPackage]
	imports  arena.Arena[rawDeclImport]
	defs     arena.Arena[rawDeclDef]
	bodies   arena.Arena[rawDeclScope]
	ranges   arena.Arena[rawDeclRange]
}

func (DeclEmpty) declKind() declKind   { return declEmpty }
func (DeclSyntax) declKind() declKind  { return declPragma }
func (DeclPackage) declKind() declKind { return declPackage }
func (DeclImport) declKind() declKind  { return declImport }
func (DeclDef) declKind() declKind     { return declDef }
func (DeclScope) declKind() declKind   { return declBody }
func (DeclRange) declKind() declKind   { return declRange }

// DeclEmpty is an empty declaration, a lone ;.
type DeclEmpty struct {
	withContext
	ptr arena.Untyped
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

// Span implements [Spanner] for Service.
func (e DeclEmpty) Span() Span {
	return e.Semicolon().Span()
}

func (DeclEmpty) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclEmpty{withContext{ctx}, ptr, ctx.decls.empties.At(ptr)}
}

func (e DeclEmpty) declIndex() arena.Untyped {
	return e.ptr
}

// Wrap wraps this declID with a context to present to the user.
func wrapDecl[T Decl](p arena.Untyped, c Contextual) T {
	ctx := c.Context()
	var decl T
	if p.Nil() {
		return decl
	}
	return decl.with(ctx, p).(T)
}

type declKind int8

// reify returns the corresponding nil Decl for the given kind,
// such that k.reify().kind() == k.
func (k declKind) reify() Decl {
	switch k {
	case declEmpty:
		return DeclEmpty{}
	case declPragma:
		return DeclSyntax{}
	case declPackage:
		return DeclPackage{}
	case declImport:
		return DeclImport{}
	case declDef:
		return DeclDef{}
	case declBody:
		return DeclScope{}
	case declRange:
		return DeclRange{}
	default:
		panic(fmt.Sprintf("protocompile/ast: unknown declKind %d: this is a bug in protocompile", k))
	}
}
