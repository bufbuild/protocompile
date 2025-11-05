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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// DeclSyntax represents a language declaration, such as the syntax or edition
// keywords.
//
// # Grammar
//
//	DeclSyntax := (`syntax` | `edition`) (`=`? Expr)? CompactOptions? `;`?
//
// Note: options are not permitted on syntax declarations in Protobuf, but we
// parse them for diagnosis.
type DeclSyntax id.Node[DeclSyntax, *File, *rawDeclSyntax]

type rawDeclSyntax struct {
	value   id.Dyn[ExprAny, ExprKind]
	keyword token.ID
	equals  token.ID
	semi    token.ID
	options id.ID[CompactOptions]
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

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclSyntax) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindSyntax, id.ID[DeclAny](d.ID())))
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

	return id.Wrap(d.Context().Stream(), d.Raw().keyword)
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

	return id.Wrap(d.Context().Stream(), d.Raw().equals)
}

// Value returns the value expression of this declaration.
//
// May be zero, if the user wrote something like syntax;. It can also be
// a number or an identifier, for cases like edition = 2024; or syntax = proto2;.
func (d DeclSyntax) Value() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().value)
}

// SetValue sets the expression for this declaration's value.
//
// If passed zero, this clears the value (e.g., for syntax = ;).
func (d DeclSyntax) SetValue(expr ExprAny) {
	d.Raw().value = expr.ID()
}

// Options returns the compact options list for this declaration.
//
// Syntax declarations cannot have options, but we parse them anyways.
func (d DeclSyntax) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return id.Wrap(d.Context(), d.Raw().options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclSyntax) SetOptions(opts CompactOptions) {
	d.Raw().options = opts.ID()
}

// Semicolon returns this declaration's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclSyntax) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// source.Span implements [source.Spanner].
func (d DeclSyntax) Span() source.Span {
	if d.IsZero() {
		return source.Span{}
	}

	return source.Join(d.KeywordToken(), d.Equals(), d.Value(), d.Semicolon())
}

// DeclPackage is the package declaration for a file.
//
// # Grammar
//
//	DeclPackage := `package` Path? CompactOptions? `;`?
//
// Note: options are not permitted on package declarations in Protobuf, but we
// parse them for diagnosis.
type DeclPackage id.Node[DeclPackage, *File, *rawDeclPackage]

type rawDeclPackage struct {
	keyword token.ID
	path    PathID
	semi    token.ID
	options id.ID[CompactOptions]
}

// DeclPackageArgs is arguments for [Context.NewDeclPackage].
type DeclPackageArgs struct {
	Keyword   token.Token
	Path      Path
	Options   CompactOptions
	Semicolon token.Token
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclPackage) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindPackage, id.ID[DeclAny](d.ID())))
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

	return id.Wrap(d.Context().Stream(), d.Raw().keyword)
}

// Path returns this package's path.
//
// May be zero, if the user wrote something like package;.
func (d DeclPackage) Path() Path {
	if d.IsZero() {
		return Path{}
	}

	return d.Raw().path.In(d.Context())
}

// Options returns the compact options list for this declaration.
//
// Package declarations cannot have options, but we parse them anyways.
func (d DeclPackage) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return id.Wrap(d.Context(), d.Raw().options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclPackage) SetOptions(opts CompactOptions) {
	d.Raw().options = opts.ID()
}

// Semicolon returns this package's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclPackage) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// source.Span implements [source.Spanner].
func (d DeclPackage) Span() source.Span {
	if d.IsZero() {
		return source.Span{}
	}

	return source.Join(d.KeywordToken(), d.Path(), d.Semicolon())
}

// DeclImport is an import declaration within a file.
//
// # Grammar
//
//	DeclImport := `import` (`weak` | `public`)? Expr? CompactOptions? `;`?
//
// Note: options are not permitted on import declarations in Protobuf, but we
// parse them for diagnosis.
type DeclImport id.Node[DeclImport, *File, *rawDeclImport]

type rawDeclImport struct {
	keyword, semi token.ID
	modifiers     []token.ID
	importPath    id.Dyn[ExprAny, ExprKind]
	options       id.ID[CompactOptions]
}

// DeclImportArgs is arguments for [Context.NewDeclImport].
type DeclImportArgs struct {
	Keyword    token.Token
	Modifiers  []token.Token
	ImportPath ExprAny
	Options    CompactOptions
	Semicolon  token.Token
}

// AsAny type-erases this declaration value.
//
// See [DeclAny] for more information.
func (d DeclImport) AsAny() DeclAny {
	if d.IsZero() {
		return DeclAny{}
	}
	return id.WrapDyn(d.Context(), id.NewDyn(DeclKindImport, id.ID[DeclAny](d.ID())))
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

	return id.Wrap(d.Context().Stream(), d.Raw().keyword)
}

// Modifiers returns the modifiers for this declaration.
func (d DeclImport) Modifiers() seq.Indexer[keyword.Keyword] {
	var slice []token.ID
	if !d.IsZero() {
		slice = d.Raw().modifiers
	}

	return seq.NewFixedSlice(slice, func(_ int, t token.ID) keyword.Keyword {
		return id.Wrap(d.Context().Stream(), t).Keyword()
	})
}

// ModifierTokens returns the modifier tokens for this declaration.
func (d DeclImport) ModifierTokens() seq.Inserter[token.Token] {
	if d.IsZero() {
		return seq.EmptySliceInserter[token.Token, token.ID]()
	}

	return seq.NewSliceInserter(&d.Raw().modifiers,
		func(_ int, e token.ID) token.Token { return id.Wrap(d.Context().Stream(), e) },
		func(_ int, t token.Token) token.ID {
			d.Context().Nodes().panicIfNotOurs(t)
			return t.ID()
		},
	)
}

// IsPublic checks whether this is an "import public".
func (d DeclImport) IsPublic() bool {
	return iterx.Contains(seq.Values(d.Modifiers()), func(k keyword.Keyword) bool {
		return k == keyword.Public
	})
}

// IsWeak checks whether this is an "import weak".
func (d DeclImport) IsWeak() bool {
	return iterx.Contains(seq.Values(d.Modifiers()), func(k keyword.Keyword) bool {
		return k == keyword.Weak
	})
}

// IsOption checks whether this is an "import option".
func (d DeclImport) IsOption() bool {
	return iterx.Contains(seq.Values(d.Modifiers()), func(k keyword.Keyword) bool {
		return k == keyword.Option
	})
}

// ImportPath returns the file path for this import as a string.
//
// May be zero, if the user forgot it.
func (d DeclImport) ImportPath() ExprAny {
	if d.IsZero() {
		return ExprAny{}
	}

	return id.WrapDyn(d.Context(), d.Raw().importPath)
}

// SetValue sets the expression for this import's file path.
//
// If passed zero, this clears the path expression.
func (d DeclImport) SetImportPath(expr ExprAny) {
	d.Raw().importPath = expr.ID()
}

// Options returns the compact options list for this declaration.
//
// Imports cannot have options, but we parse them anyways.
func (d DeclImport) Options() CompactOptions {
	if d.IsZero() {
		return CompactOptions{}
	}

	return id.Wrap(d.Context(), d.Raw().options)
}

// SetOptions sets the compact options list for this declaration.
//
// Setting it to a zero Options clears it.
func (d DeclImport) SetOptions(opts CompactOptions) {
	d.Raw().options = opts.ID()
}

// Semicolon returns this import's ending semicolon.
//
// May be zero, if the user forgot it.
func (d DeclImport) Semicolon() token.Token {
	if d.IsZero() {
		return token.Zero
	}

	return id.Wrap(d.Context().Stream(), d.Raw().semi)
}

// source.Span implements [source.Spanner].
func (d DeclImport) Span() source.Span {
	if d.IsZero() {
		return source.Span{}
	}

	return source.Join(d.KeywordToken(), d.ImportPath(), d.Semicolon())
}
