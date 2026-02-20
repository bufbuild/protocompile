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
)

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
	slices.SortStableFunc(decls, func(a, b ast.DeclAny) int {
		aKey := declSortKey(a)
		bKey := declSortKey(b)

		if c := cmp.Compare(aKey.section, bKey.section); c != 0 {
			return c
		}
		return cmp.Compare(aKey.name, bKey.name)
	})
}

// section is the canonical ordering of file-level declaration sections.
type section int

const (
	sectionSyntax       section = iota // syntax/edition
	sectionPackage                     // package
	sectionImport                      // import, import public, import weak
	sectionImportOption                // import option (editions)
	sectionOption                      // file-level option
	sectionBody                        // everything else
)

type sortKey struct {
	section section
	name    string
}

// declSortKey returns the sort key for a file-level declaration.
func declSortKey(decl ast.DeclAny) sortKey {
	switch decl.Kind() {
	case ast.DeclKindSyntax:
		return sortKey{section: sectionSyntax}
	case ast.DeclKindPackage:
		return sortKey{section: sectionPackage}
	case ast.DeclKindImport:
		imp := decl.AsImport()
		s := sectionImport
		if imp.IsOption() {
			s = sectionImportOption
		}
		return sortKey{section: s, name: importSortName(imp)}
	case ast.DeclKindDef:
		if decl.AsDef().Classify() == ast.DefKindOption {
			return sortKey{section: sectionOption, name: optionSortName(decl)}
		}
	}
	return sortKey{section: sectionBody}
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
	for pc := range opt.Path.Components {
		return !pc.AsExtension().IsZero()
	}
	return false
}
