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

package ast2

import "iter"

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [Body]). The File type provides
// convenience functions for extracting salient elements, such as the [Pragma] and the [Package].
type File struct {
	Body
}

// Pragma returns this file's pragma, if it has one.
func (f File) Pragma() Pragma {
	for _, pragma := range Decls[Pragma](f.Body) {
		return pragma
	}

	return Pragma{}
}

// Package returns this file's package declaration, if it has one.
func (f File) Package() Package {
	for _, pkg := range Decls[Package](f.Body) {
		return pkg
	}

	return Package{}
}

// Imports returns an iterator over this file's import declarations.
func (f File) Imports() iter.Seq2[int, Import] {
	return Decls[Import](f.Body)
}

// Pragma represents a language pragma, such as the syntax or edition keywords.
type Pragma struct {
	withContext

	raw *rawPragma
}

type rawPragma struct {
	keyword, equals, value, semi rawToken
}

// Keyword returns the keyword for this pragma.
func (p Pragma) Keyword() Token {
	return p.raw.keyword.With(p)
}

// IsSyntax checks whether this is an OG syntax pragma.
func (p Pragma) IsSyntax() bool {
	return p.Keyword().Text() == "syntax"
}

// IsEdition checks whether this is a new-style edition pragma.
func (p Pragma) IsEdition() bool {
	return p.Keyword().Text() == "edition"
}

// Equals returns the equals sign after the keyword.
//
// May be nil, if the user wrote something like syntax "proto2";.
func (p Pragma) Equals() Token {
	return p.raw.equals.With(p)
}

// Value returns the value of this pragma, which may be any kind of token.
//
// May be nil, if the user wrote something like syntax;. It can also be
// a number or an identifier, for cases like edition = 2024; or syntax = proto2;.
func (p Pragma) Value() Token {
	return p.raw.value.With(p)
}

// Semicolon returns this pragma's ending semicolon.
//
// May be nil, if the user forgot it.
func (p Pragma) Semicolon() Token {
	return p.raw.semi.With(p)
}

// Span implements [Spanner] for Message.
func (p Pragma) Span() Span {
	return JoinSpans(p.Keyword(), p.Equals(), p.Value(), p.Semicolon())
}

func (Pragma) with(ctx *Context, idx int) Decl {
	return Pragma{withContext{ctx}, ctx.pragmas.At(idx)}
}

// Package is the package declaration for a file.
type Package struct {
	withContext

	raw *rawPackage
}

type rawPackage struct {
	keyword rawToken
	path    rawPath
	semi    rawToken
}

// Keyword returns the "package" keyword for this declaration.
func (p Package) Keyword() Token {
	return p.raw.keyword.With(p)
}

// Path returns this package's path.
//
// May be nil, if the user wrote something like package;.
func (p Package) Path() Path {
	return p.raw.path.With(p)
}

// Semicolon returns this package's ending semicolon.
//
// May be nil, if the user forgot it.
func (p Package) Semicolon() Token {
	return p.raw.semi.With(p)
}

// Span implements [Spanner] for Message.
func (p Package) Span() Span {
	return JoinSpans(p.Keyword(), p.Path(), p.Semicolon())
}

func (Package) with(ctx *Context, idx int) Decl {
	return Package{withContext{ctx}, ctx.packages.At(idx)}
}

// Import is an import declaration within a file.
type Import struct {
	withContext

	raw *rawImport
}

type rawImport struct {
	keyword, modifier, filePath, semi rawToken
}

// Keyword returns the "import" keyword for this pragma.
func (i Import) Keyword() Token {
	return i.raw.keyword.With(i)
}

// Keyword returns the modifier keyword for this pragma.
//
// May be nil if there is no modifier.
func (i Import) Modifier() Token {
	return i.raw.modifier.With(i)
}

// IsSyntax checks whether this is an "import public"
func (i Import) IsPublic() bool {
	return i.Keyword().Text() == "public"
}

// IsEdition checks whether this is an "import weak".
func (i Import) IsWeak() bool {
	return i.Keyword().Text() == "weak"
}

// ImportPath returns the file path for this import as a string.
//
// May be nil, if the user forgot it.
func (i Import) ImportPath() Token {
	return i.raw.filePath.With(i)
}

// Semicolon returns this import's ending semicolon.
//
// May be nil, if the user forgot it.
func (i Import) Semicolon() Token {
	return i.raw.semi.With(i)
}

// Span implements [Spanner] for Message.
func (i Import) Span() Span {
	return JoinSpans(i.Keyword(), i.Modifier(), i.ImportPath(), i.Semicolon())
}

func (Import) with(ctx *Context, idx int) Decl {
	return Import{withContext{ctx}, ctx.imports.At(idx)}
}
