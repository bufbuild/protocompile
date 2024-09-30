package ast

import "fmt"

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
	with(ctx *Context, idx int) Decl

	// kind returns what kind of decl this is.
	declKind() declKind
	// declIndex returns the index this declaration occupies in its owning
	// context. This is 0-indexed, and must be incremented
	declIndex() int
}

// decls is storage for every kind of Decl in a Context.
type decls struct {
	empties  pointers[rawDeclEmpty]
	syntaxes pointers[rawDeclSyntax]
	packages pointers[rawDeclPackage]
	imports  pointers[rawDeclImport]
	defs     pointers[rawDeclDef]
	bodies   pointers[rawDeclBody]
	ranges   pointers[rawDeclRange]
}

func (DeclEmpty) declKind() declKind   { return declEmpty }
func (DeclSyntax) declKind() declKind  { return declPragma }
func (DeclPackage) declKind() declKind { return declPackage }
func (DeclImport) declKind() declKind  { return declImport }
func (DeclDef) declKind() declKind     { return declDef }
func (DeclBody) declKind() declKind    { return declBody }
func (DeclRange) declKind() declKind   { return declRange }

// DeclEmpty is an empty declaration, a lone ;.
type DeclEmpty struct {
	withContext

	idx int
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

func (DeclEmpty) with(ctx *Context, idx int) Decl {
	return DeclEmpty{withContext{ctx}, idx, ctx.decls.empties.At(idx)}
}

func (e DeclEmpty) declIndex() int {
	return e.idx
}

// decl is a typed reference to a declaration inside some Context.
//
// Note: decl indices are one-indexed, to allow for the zero value
// to represent nil.
type decl[T Decl] uint32

func declFor[T Decl](d T) decl[T] {
	if d.Context() == nil {
		return decl[T](0)
	}
	return decl[T](d.declIndex() + 1)
}

// Wrap wraps this declID with a context to present to the user.
func (d decl[T]) With(c Contextual) T {
	ctx := c.Context()

	var decl T
	if d == 0 {
		return decl
	}

	return decl.with(ctx, int(uint32(d)-1)).(T)
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
		return DeclBody{}
	case declRange:
		return DeclRange{}
	default:
		panic(fmt.Sprintf("protocompile/ast: unknown declKind %d: this is a bug in protocompile", k))
	}
}
