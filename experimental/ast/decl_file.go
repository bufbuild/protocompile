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
	"iter"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// File is the top-level AST node for a Protobuf file.
//
// A file is a list of declarations (in other words, it is a [DeclBody]). The
// File type provides convenience functions for extracting salient elements,
// such as the [DeclSyntax] and the [DeclPackage].
//
// # Grammar
//
//	File := DeclAny*
type File struct {
	DeclBody
}

// Syntax returns this file's declaration, if it has one.
func (f File) Syntax() DeclSyntax {
	for d := range seq.Values(f.Decls()) {
		if s := d.AsSyntax(); !s.IsZero() {
			return s
		}
	}
	return DeclSyntax{}
}

// Package returns this file's package declaration, if it has one.
func (f File) Package() DeclPackage {
	for d := range seq.Values(f.Decls()) {
		if p := d.AsPackage(); !p.IsZero() {
			return p
		}
	}
	return DeclPackage{}
}

// Imports returns an iterator over this file's import declarations.
func (f File) Imports() iter.Seq[DeclImport] {
	return iterx.FilterMap(seq.Values(f.Decls()), func(d DeclAny) (DeclImport, bool) {
		if imp := d.AsImport(); !imp.IsZero() {
			return imp, true
		}
		return DeclImport{}, false
	})
}

// DeclSyntax represents a language declaration, such as the syntax or edition
// keywords.
//
// # Grammar
//
//	DeclSyntax := (`syntax` | `edition`) (`=`? Expr)? CompactOptions? `;`?
//
// Note: options are not permitted on syntax declarations in Protobuf, but we
// parse them for diagnosis.
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

// Keyword returns the keyword for this declaration.
func (d DeclSyntax) Keyword() keyword.Keyword {
	return d.KeywordToken().Keyword()
}

// KeywordToken returns the keyword token for this declaration.
func (d DeclSyntax) KeywordToken() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.keyword.In(d.Context())
}

// IsSyntax checks whether this is an OG syntax declaration.
func (d DeclSyntax) IsSyntax() bool {
	return d.Keyword() == keyword.Syntax
}

// IsEdition checks whether this is a new-style edition declaration.
func (d DeclSyntax) IsEdition() bool {
	return d.Keyword() == keyword.Edition
}

// Equals returns the equals sign after the keyword.
//
// May be zero, if the user wrote something like syntax "proto2";.
func (d DeclSyntax) Equals() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.equals.In(d.Context())
}

// Value returns the value expression of this declaration.
//
// May be zero, if the user wrote something like syntax;. It can also be
// a number or an identifier, for cases like edition = 2024; or syntax = proto2;.
func (d DeclSyntax) Value() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return newExprAny(d.Context(), d.raw.value)
}

// SetValue sets the expression for this declaration's value.
//
// If passed zero, this clears the value (e.g., for syntax = ;).
func (d DeclSyntax) SetValue(expr ExprAny) {
	d.raw.value = expr.raw
}

// Options returns the compact options list for this declaration.
//
// Syntax declarations cannot have options, but we parse them anyways.
func (d DeclSyntax) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclSyntax) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this declaration's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclSyntax) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclSyntax) Span() report.Span {
	if d.IsZero() {
		return report.Span{}
	}

	return report.Join(d.KeywordToken(), d.Equals(), d.Value(), d.Semicolon())
}

func wrapDeclSyntax(c Context, ptr arena.Pointer[rawDeclSyntax]) DeclSyntax {
	return DeclSyntax{wrapDecl(c, ptr)}
}

// DeclPackage is the package declaration for a file.
//
// # Grammar
//
//	DeclPackage := `package` Path? CompactOptions? `;`?
//
// Note: options are not permitted on package declarations in Protobuf, but we
// parse them for diagnosis.
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

// Keyword returns the keyword for this declaration.
func (d DeclPackage) Keyword() keyword.Keyword {
	return d.KeywordToken().Keyword()
}

// KeywordToken returns the "package" token for this declaration.
func (d DeclPackage) KeywordToken() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.keyword.In(d.Context())
}

// Path returns this package's path.
//
// May be zero, if the user wrote something like package;.
func (d DeclPackage) Path() Path {
	if d.IsZero() {
		return Path{}
	}

	return d.raw.path.With(d.Context())
}

// Options returns the compact options list for this declaration.
//
// Package declarations cannot have options, but we parse them anyways.
func (d DeclPackage) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclPackage) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this package's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclPackage) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclPackage) Span() report.Span {
	if d.IsZero() {
		return report.Span{}
	}

	return report.Join(d.KeywordToken(), d.Path(), d.Semicolon())
}

func wrapDeclPackage(c Context, ptr arena.Pointer[rawDeclPackage]) DeclPackage {
	return DeclPackage{wrapDecl(c, ptr)}
}

// DeclImport is an import declaration within a file.
//
// # Grammar
//
//	DeclImport := `import` (`weak` | `public`)? Expr? CompactOptions? `;`?
//
// Note: options are not permitted on import declarations in Protobuf, but we
// parse them for diagnosis.
type DeclImport struct{ declImpl[rawDeclImport] }

type rawDeclImport struct {
	keyword, semi token.ID
	modifiers     []token.ID
	importPath    rawExpr
	options       arena.Pointer[rawCompactOptions]
}

// DeclImportArgs is arguments for [Context.NewDeclImport].
type DeclImportArgs struct {
	Keyword    token.Token
	Modifiers  []token.Token
	ImportPath ExprAny
	Options    CompactOptions
	Semicolon  token.Token
}

// Keyword returns the keyword for this declaration.
func (d DeclImport) Keyword() keyword.Keyword {
	return d.KeywordToken().Keyword()
}

// KeywordToken returns the "import" keyword for this declaration.
func (d DeclImport) KeywordToken() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.keyword.In(d.Context())
}

// Modifier returns the modifier keyword for this declaration.
func (d DeclImport) Modifiers() seq.Indexer[keyword.Keyword] {
	var slice []token.ID
	if !d.IsZero() {
		slice = d.raw.modifiers
	}

	return seq.NewFixedSlice(slice, func(_ int, t token.ID) keyword.Keyword {
		return t.In(d.Context()).Keyword()
	})
}

// ModifierToken returns the modifier token for this declaration.
//
// May be zero if there is no modifier.
func (d DeclImport) ModifierTokens() seq.Inserter[token.Token] {
	if d.IsZero() {
		return seq.EmptySliceInserter[token.Token, token.ID]()
	}

	return seq.NewSliceInserter(&d.raw.modifiers,
		func(_ int, e token.ID) token.Token { return e.In(d.Context()) },
		func(_ int, t token.Token) token.ID {
			d.Context().Nodes().panicIfNotOurs(t)
			return t.ID()
		},
	)
}

// IsSyntax checks whether this is an "import public".
func (d DeclImport) IsPublic() bool {
	return iterx.Contains(seq.Values(d.Modifiers()), func(k keyword.Keyword) bool {
		return k == keyword.Public
	})
}

// IsEdition checks whether this is an "import weak".
func (d DeclImport) IsWeak() bool {
	return iterx.Contains(seq.Values(d.Modifiers()), func(k keyword.Keyword) bool {
		return k == keyword.Weak
	})
}

// ImportPath returns the file path for this import as a string.
//
// May be zero, if the user forgot it.
func (d DeclImport) ImportPath() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return newExprAny(d.Context(), d.raw.importPath)
}

// SetValue sets the expression for this import's file path.
//
// If passed zero, this clears the path expression.
func (d DeclImport) SetImportPath(expr ExprAny) {
	d.raw.importPath = expr.raw
}

// Options returns the compact options list for this declaration.
//
// Imports cannot have options, but we parse them anyways.
func (d DeclImport) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return wrapOptions(d.Context(), d.raw.options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclImport) SetOptions(opts CompactOptions) {
	d.raw.options = d.Context().Nodes().options.Compress(opts.raw)
}

// Semicolon returns this import's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclImport) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return d.raw.semi.In(d.Context())
}

// report.Span implements [report.Spanner].
func (d DeclImport) Span() report.Span {
	if d.IsZero() {
		return report.Span{}
	}

	return report.Join(d.KeywordToken(), d.ImportPath(), d.Semicolon())
}

func wrapDeclImport(c Context, ptr arena.Pointer[rawDeclImport]) DeclImport {
	return DeclImport{wrapDecl(c, ptr)}
}
