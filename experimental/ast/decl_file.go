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

import "github.com/bufbuild/protocompile/internal/arena"

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [DeclScope]). The File type provides
// convenience functions for extracting salient elements, such as the [DeclSyntax] and the [DeclPackage].
type File struct {
	DeclScope
}

// Syntax returns this file's pragma, if it has one.
func (f File) Syntax() (syntax DeclSyntax) {
	Decls[DeclSyntax](f.DeclScope)(func(_ int, d DeclSyntax) bool {
		syntax = d
		return false
	})
	return
}

// Package returns this file's package declaration, if it has one.
func (f File) Package() (pkg DeclPackage) {
	Decls[DeclPackage](f.DeclScope)(func(_ int, d DeclPackage) bool {
		pkg = d
		return false
	})
	return
}

// Imports returns an iterator over this file's import declarations.
func (f File) Imports() func(func(int, DeclImport) bool) {
	return Decls[DeclImport](f.DeclScope)
}

// DeclSyntax represents a language pragma, such as the syntax or edition keywords.
type DeclSyntax struct {
	withContext

	ptr arena.Untyped
	raw *rawDeclSyntax
}

type rawDeclSyntax struct {
	keyword, equals, semi rawToken
	value                 rawExpr
	options               arena.Pointer[rawCompactOptions]
}

// DeclSyntaxArgs is arguments for [Context.NewDeclSyntax].
type DeclSyntaxArgs struct {
	// Must be "syntax" or "edition".
	Keyword   Token
	Equals    Token
	Value     Expr
	Options   CompactOptions
	Semicolon Token
}

var _ Decl = DeclSyntax{}

// Keyword returns the keyword for this pragma.
func (d DeclSyntax) Keyword() Token {
	return d.raw.keyword.With(d)
}

// IsSyntax checks whether this is an OG syntax pragma.
func (d DeclSyntax) IsSyntax() bool {
	return d.Keyword().Text() == "syntax"
}

// IsEdition checks whether this is a new-style edition pragma.
func (d DeclSyntax) IsEdition() bool {
	return d.Keyword().Text() == "edition"
}

// Equals returns the equals sign after the keyword.
//
// May be nil, if the user wrote something like syntax "proto2";.
func (d DeclSyntax) Equals() Token {
	return d.raw.equals.With(d)
}

// Value returns the value expression of this pragma.
//
// May be nil, if the user wrote something like syntax;. It can also be
// a number or an identifier, for cases like edition = 2024; or syntax = proto2;.
func (d DeclSyntax) Value() Expr {
	return d.raw.value.With(d)
}

// SetValue sets the expression for this pragma's value.
//
// If passed nil, this clears the value (e.g., for syntax = ;)
func (d DeclSyntax) SetValue(expr Expr) {
	d.raw.value = toRawExpr(expr)
}

// Options returns the compact options list for this declaration.
//
// Syntax declarations cannot have options, but we parse them anyways.
func (d DeclSyntax) Options() CompactOptions {
	return newOptions(d.raw.options, d)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclSyntax) SetOptions(opts CompactOptions) {
	d.raw.options = opts.ptr
}

// Semicolon returns this pragma's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclSyntax) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Span implements [Spanner] for Message.
func (d DeclSyntax) Span() Span {
	return JoinSpans(d.Keyword(), d.Equals(), d.Value(), d.Semicolon())
}

func (DeclSyntax) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclSyntax{withContext{ctx}, ptr, ctx.decls.syntaxes.At(ptr)}
}

func (d DeclSyntax) declIndex() arena.Untyped {
	return d.ptr
}

// DeclPackage is the package declaration for a file.
type DeclPackage struct {
	withContext

	ptr arena.Untyped
	raw *rawDeclPackage
}

type rawDeclPackage struct {
	keyword rawToken
	path    rawPath
	semi    rawToken
	options arena.Pointer[rawCompactOptions]
}

// DeclPackageArgs is arguments for [Context.NewDeclPackage].
type DeclPackageArgs struct {
	Keyword   Token
	Path      Path
	Options   CompactOptions
	Semicolon Token
}

var _ Decl = DeclPackage{}

// Keyword returns the "package" keyword for this declaration.
func (d DeclPackage) Keyword() Token {
	return d.raw.keyword.With(d)
}

// Path returns this package's path.
//
// May be nil, if the user wrote something like package;.
func (d DeclPackage) Path() Path {
	return d.raw.path.With(d)
}

// Options returns the compact options list for this declaration.
//
// Package declarations cannot have options, but we parse them anyways.
func (d DeclPackage) Options() CompactOptions {
	return newOptions(d.raw.options, d)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclPackage) SetOptions(opts CompactOptions) {
	d.raw.options = opts.ptr
}

// Semicolon returns this package's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclPackage) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Span implements [Spanner] for DeclPackage.
func (d DeclPackage) Span() Span {
	return JoinSpans(d.Keyword(), d.Path(), d.Semicolon())
}

func (DeclPackage) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclPackage{withContext{ctx}, ptr, ctx.decls.packages.At(ptr)}
}

func (d DeclPackage) declIndex() arena.Untyped {
	return d.ptr
}

// DeclImport is an import declaration within a file.
type DeclImport struct {
	withContext

	ptr arena.Untyped
	raw *rawDeclImport
}

type rawDeclImport struct {
	keyword, modifier, semi rawToken
	importPath              rawExpr
	options                 arena.Pointer[rawCompactOptions]
}

// DeclImportArgs is arguments for [Context.NewDeclImport].
type DeclImportArgs struct {
	Keyword    Token
	Modifier   Token
	ImportPath Expr
	Options    CompactOptions
	Semicolon  Token
}

var _ Decl = DeclImport{}

// Keyword returns the "import" keyword for this pragma.
func (d DeclImport) Keyword() Token {
	return d.raw.keyword.With(d)
}

// Keyword returns the modifier keyword for this pragma.
//
// May be nil if there is no modifier.
func (d DeclImport) Modifier() Token {
	return d.raw.modifier.With(d)
}

// IsSyntax checks whether this is an "import public"
func (d DeclImport) IsPublic() bool {
	return d.Modifier().Text() == "public"
}

// IsEdition checks whether this is an "import weak".
func (d DeclImport) IsWeak() bool {
	return d.Modifier().Text() == "weak"
}

// ImportPath returns the file path for this import as a string.
//
// May be nil, if the user forgot it.
func (d DeclImport) ImportPath() Expr {
	return d.raw.importPath.With(d)
}

// SetValue sets the expression for this import's file path.
//
// If passed nil, this clears the path expression.
func (d DeclImport) SetImportPath(expr Expr) {
	d.raw.importPath = toRawExpr(expr)
}

// Options returns the compact options list for this declaration.
//
// Imports cannot have options, but we parse them anyways.
func (d DeclImport) Options() CompactOptions {
	return newOptions(d.raw.options, d)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclImport) SetOptions(opts CompactOptions) {
	d.raw.options = opts.ptr
}

// Semicolon returns this import's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclImport) Semicolon() Token {
	return d.raw.semi.With(d)
}

// Span implements [Spanner] for DeclImport.
func (d DeclImport) Span() Span {
	return JoinSpans(d.Keyword(), d.Modifier(), d.ImportPath(), d.Semicolon())
}

func (DeclImport) with(ctx *Context, ptr arena.Untyped) Decl {
	return DeclImport{withContext{ctx}, ptr, ctx.decls.imports.At(ptr)}
}

func (d DeclImport) declIndex() arena.Untyped {
	return d.ptr
}
