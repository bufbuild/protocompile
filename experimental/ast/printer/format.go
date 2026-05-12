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

package printer

import (
	"cmp"
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/token"
)

// sourceWasFlat reports whether the source had open and close on the
// same line, i.e. the bracketed scope was written without any newline
// between its opening and closing tokens. Returns false if either token
// is zero or has a missing source position.
//
// Uses [token.Token.LeafSpan] (not [token.Token.Span]) because the open
// and close tokens of a fused bracket pair both report the full range
// of the bracketed scope from [token.Token.Span]; only LeafSpan gives
// the position of the punctuation token itself.
func sourceWasFlat(openTok, closeTok token.Token) bool {
	if openTok.IsZero() || closeTok.IsZero() {
		return false
	}
	openSpan, closeSpan := openTok.LeafSpan(), closeTok.LeafSpan()
	if openSpan.IsZero() || closeSpan.IsZero() {
		return false
	}
	return openSpan.StartLoc().Line == closeSpan.StartLoc().Line
}

// sourceBlankLineBetweenFields reports whether the source had a
// blank line (an empty or whitespace-only line) between two adjacent
// dict-literal fields. Used by `printDict` as a fallback when
// [detachedTrivia.hasBlankBefore] can't track per-element blank lines
// (e.g. when source elides commas, so the trivia walker doesn't see
// element boundaries).
func sourceBlankLineBetweenFields(prev, curr ast.ExprField) bool {
	prevSpan, currSpan := prev.Span(), curr.Span()
	if prevSpan.IsZero() || currSpan.IsZero() || prevSpan.File != currSpan.File {
		return false
	}
	between := prevSpan.File.Text()[prevSpan.End:currSpan.Start]
	// A blank line is two newlines separated only by horizontal
	// whitespace. Walk forward looking for that pattern.
	afterNewline := false
	for i := range len(between) {
		switch between[i] {
		case '\n':
			if afterNewline {
				return true
			}
			afterNewline = true
		case ' ', '\t':
			// preserve afterNewline across horizontal whitespace
		default:
			afterNewline = false
		}
	}
	return false
}

// literalShouldBreak applies the configured [LayoutStrategy] to a
// literal scope (compact options bracket, array literal, dict
// literal):
//
//   - [LayoutStrict]: broken if and only if the scope has 2 or more elements,
//     matching the legacy formatter's "expand if non-trivial" rule.
//
//   - [LayoutDynamic]: broken if and only if source had a newline between open
//     and close, deferring width-driven breaks to [dom.Group].
//
// Callers should OR the result with their own forceBroken signal
// (e.g. for scope-attached comments that require expansion).
func (p *printer) literalShouldBreak(openTok, closeTok token.Token, count int) bool {
	switch p.options.Formatting.LiteralLayout {
	case LayoutDynamic:
		return !sourceWasFlat(openTok, closeTok)
	default: // LayoutStrict
		return count >= 2
	}
}

// bodyShouldBreak applies the configured [LayoutStrategy] to a
// decl-bearing body scope (`{ ... }` on message, enum, service,
// oneof, extend, or RPC method):
//
//   - [LayoutStrict]: always broken. Callers handle the empty-body
//     case (rendered as `{}`) before consulting this helper.
//
//   - [LayoutDynamic]: broken if and only if source had a newline between open
//     and close brace.
//
// Callers should OR the result with their own forceBroken signal
// (e.g. for scope-attached comments that require expansion).
func (p *printer) bodyShouldBreak(openTok, closeTok token.Token) bool {
	switch p.options.Formatting.BodyLayout {
	case LayoutDynamic:
		return !sourceWasFlat(openTok, closeTok)
	default: // LayoutStrict
		return true
	}
}

// sortFileDeclsForFormat sorts file-level declarations in place into
// canonical order using a stable sort. The canonical order is:
//
//  1. syntax/edition
//  2. package
//  3. imports (sorted alphabetically, with edition "import option"
//     declarations after all other imports)
//  4. file-level options (plain before extension, alphabetically within
//     each group)
//  5. everything else (original order preserved)
func sortFileDeclsForFormat(decls []ast.DeclAny) {
	slices.SortStableFunc(decls, compareDecl)
}

// compareDecl compares two declarations for sorting. Declarations
// are first ordered by rank (syntax < package < import < option <
// body). Within the import and option ranks, ties are broken by
// sort name (see [importSortName] and [optionSortName]); decls at
// the body rank preserve their source order via the stable sort.
func compareDecl(a, b ast.DeclAny) int {
	aRank, bRank := rankDecl(a), rankDecl(b)
	if c := cmp.Compare(aRank, bRank); c != 0 {
		return c
	}
	switch a.Kind() {
	case ast.DeclKindImport:
		aImp := a.AsImport()
		bImp := b.AsImport()
		aImpOpt := 0
		if aImp.IsOption() {
			aImpOpt = 1
		}
		bImpOpt := 0
		if bImp.IsOption() {
			bImpOpt = 1
		}
		if c := cmp.Compare(aImpOpt, bImpOpt); c != 0 {
			return c
		}
		return cmp.Compare(importSortName(aImp), importSortName(bImp))
	case ast.DeclKindDef:
		if a.AsDef().Classify() == ast.DefKindOption {
			return cmp.Compare(optionSortName(a), optionSortName(b))
		}
		return 0
	default:
		return 0
	}
}

type declSortRank int

const (
	rankSyntax  declSortRank = iota // syntax/edition
	rankPackage                     // package
	rankImport                      // import
	rankOption                      // option
	rankBody                        // body
)

// rankDecl returns the sort rank for a declaration.
func rankDecl(decl ast.DeclAny) declSortRank {
	switch decl.Kind() {
	case ast.DeclKindSyntax:
		return rankSyntax
	case ast.DeclKindPackage:
		return rankPackage
	case ast.DeclKindImport:
		return rankImport
	case ast.DeclKindDef:
		if decl.AsDef().Classify() == ast.DefKindOption {
			return rankOption
		}
	}
	return rankBody
}

// importSortName returns the sort name for an import declaration.
// This is the raw token text of the import path (e.g. `"foo/bar.proto"`).
func importSortName(imp ast.DeclImport) string {
	lit := imp.ImportPath().AsLiteral()
	if lit.IsZero() {
		return ""
	}
	return lit.Token.Text()
}

// optionSortName returns the sort name for a file-level option declaration.
// Plain options sort before extension options by prefixing with "0" or "1".
func optionSortName(decl ast.DeclAny) string {
	opt := decl.AsDef().AsOption()
	canonical := opt.Path.Canonicalized()
	if isExtensionOption(opt) {
		return "1" + canonical
	}
	return "0" + canonical
}

// isExtensionOption returns true if the option's path starts with an
// extension component (parenthesized path like `(foo.bar)`).
func isExtensionOption(opt ast.DefOption) bool {
	for pc := range opt.Path.Components() {
		return !pc.AsExtension().IsZero()
	}
	return false
}
