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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/iter"
)

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [DeclBody]). The File type provides
// convenience functions for extracting salient elements, such as the [DeclSyntax] and the [DeclPackage].
type File struct {
	DeclBody
}

// Syntax returns this file's pragma, if it has one.
func (f File) Syntax() (syntax DeclSyntax) {
	f.Iter(func(_ int, d DeclAny) bool {
		if s := d.AsSyntax(); !s.Nil() {
			syntax = s
			return false
		}
		return true
	})
	return
}

// Package returns this file's package declaration, if it has one.
func (f File) Package() (pkg DeclPackage) {
	f.Iter(func(_ int, d DeclAny) bool {
		if p := d.AsPackage(); !p.Nil() {
			pkg = p
			return false
		}
		return true
	})
	return
}

// Imports returns an iterator over this file's import declarations.
func (f File) Imports() iter.Seq2[int, DeclImport] {
	return func(yield func(int, DeclImport) bool) {
		var i int
		f.Iter(func(_ int, d DeclAny) bool {
			if imp := d.AsImport(); !imp.Nil() {
				if !yield(i, imp) {
					return false
				}
				i++
			}

			return true
		})
	}
}

// DeclSyntax represents a language pragma, such as the syntax or edition keywords.
type DeclSyntax struct{ declImpl[rawDeclSyntax] }

type rawDeclSyntax struct {
	keyword, equals, semi token.ID
	value                 rawExpr
	options               arena.Pointer[rawCompactOptions]
}

// DeclSyntaxArgs is arguments for [Context.NewDeclSyntax].
type DeclSyntaxArgs struct {
	// Must be "syntax" or "edition".
	Keyword   token.Token
	Equals    token.Token
	Value     ExprAny
	Options   CompactOptions
	Semicolon token.Token
}

// Keyword returns the keyword for this pragma.
func (d DeclSyntax) Keyword() token.Token {
	return d.raw.keyword.In(d.Context())
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
func (d DeclSyntax) Equals() token.Token {
	return d.raw.equals.In(d.Context())
}

// Value returns the value expression of this pragma.
//
// May be nil, if the user wrote something like syntax;. It can also be
// a number or an identifier, for cases like edition = 2024; or syntax = proto2;.
func (d DeclSyntax) Value() ExprAny {
	return d.raw.value.With(d.Context())
}

// SetValue sets the expression for this pragma's value.
//
// If passed nil, this clears the value (e.g., for syntax = ;).
func (d DeclSyntax) SetValue(expr ExprAny) {
	d.raw.value = expr.raw
}

// Options returns the compact options list for this declaration.
//
// Syntax declarations cannot have options, but we parse them anyways.
func (d DeclSyntax) Options() CompactOptions {
	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclSyntax) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this pragma's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclSyntax) Semicolon() token.Token {
	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclSyntax) Span() report.Span {
	return report.Join(d.Keyword(), d.Equals(), d.Value(), d.Semicolon())
}

func wrapDeclSyntax(c Context, ptr arena.Pointer[rawDeclSyntax]) DeclSyntax {
	return DeclSyntax{wrapDecl(c, ptr)}
}

// DeclPackage is the package declaration for a file.
type DeclPackage struct{ declImpl[rawDeclPackage] }

type rawDeclPackage struct {
	keyword token.ID
	path    rawPath
	semi    token.ID
	options arena.Pointer[rawCompactOptions]
}

// DeclPackageArgs is arguments for [Context.NewDeclPackage].
type DeclPackageArgs struct {
	Keyword   token.Token
	Path      Path
	Options   CompactOptions
	Semicolon token.Token
}

// Keyword returns the "package" keyword for this declaration.
func (d DeclPackage) Keyword() token.Token {
	return d.raw.keyword.In(d.Context())
}

// Path returns this package's path.
//
// May be nil, if the user wrote something like package;.
func (d DeclPackage) Path() Path {
	return d.raw.path.With(d.Context())
}

// Options returns the compact options list for this declaration.
//
// Package declarations cannot have options, but we parse them anyways.
func (d DeclPackage) Options() CompactOptions {
	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclPackage) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this package's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclPackage) Semicolon() token.Token {
	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclPackage) Span() report.Span {
	return report.Join(d.Keyword(), d.Path(), d.Semicolon())
}

func wrapDeclPackage(c Context, ptr arena.Pointer[rawDeclPackage]) DeclPackage {
	return DeclPackage{wrapDecl(c, ptr)}
}

// DeclImport is an import declaration within a file.
type DeclImport struct{ declImpl[rawDeclImport] }

type rawDeclImport struct {
	keyword, modifier, semi token.ID
	importPath              rawExpr
	options                 arena.Pointer[rawCompactOptions]
}

// DeclImportArgs is arguments for [Context.NewDeclImport].
type DeclImportArgs struct {
	Keyword    token.Token
	Modifier   token.Token
	ImportPath ExprAny
	Options    CompactOptions
	Semicolon  token.Token
}

// Keyword returns the "import" keyword for this pragma.
func (d DeclImport) Keyword() token.Token {
	return d.raw.keyword.In(d.Context())
}

// Keyword returns the modifier keyword for this pragma.
//
// May be nil if there is no modifier.
func (d DeclImport) Modifier() token.Token {
	return d.raw.modifier.In(d.Context())
}

// IsSyntax checks whether this is an "import public".
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
func (d DeclImport) ImportPath() ExprAny {
	return d.raw.importPath.With(d.Context())
}

// SetValue sets the expression for this import's file path.
//
// If passed nil, this clears the path expression.
func (d DeclImport) SetImportPath(expr ExprAny) {
	d.raw.importPath = expr.raw
}

// Options returns the compact options list for this declaration.
//
// Imports cannot have options, but we parse them anyways.
func (d DeclImport) Options() CompactOptions {
	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a nil Options clears it.
func (d DeclImport) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this import's ending semicolon.
//
// May be nil, if the user forgot it.
func (d DeclImport) Semicolon() token.Token {
	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclImport) Span() report.Span {
	return report.Join(d.Keyword(), d.Modifier(), d.ImportPath(), d.Semicolon())
}

func wrapDeclImport(c Context, ptr arena.Pointer[rawDeclImport]) DeclImport {
	return DeclImport{wrapDecl(c, ptr)}
}
