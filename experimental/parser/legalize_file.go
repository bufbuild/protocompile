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

package parser

import (
	"regexp"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/seq"
)

var isOrdinaryFilePath = regexp.MustCompile("[0-9a-zA-Z./_-]*")

// legalizeFile is the entry-point for legalizing a parsed Protobuf file
func legalizeFile(p *parser, file ast.File) {
	var (
		pkg     ast.DeclPackage
		imports = make(map[string][]ast.DeclImport)
	)
	seq.All(file.Decls())(func(i int, decl ast.DeclAny) bool {
		file := classified{file, taxa.TopLevel}
		switch decl.Kind() {
		case ast.DeclKindSyntax:
			legalizeSyntax(p, file, i, &p.syntax, decl.AsSyntax())
		case ast.DeclKindPackage:
			legalizePackage(p, file, i, &pkg, decl.AsPackage())
		case ast.DeclKindImport:
			legalizeImport(p, file, decl.AsImport(), imports)
		default:
			legalizeDecl(p, file, decl)
		}

		return true
	})
}

// legalizeSyntax legalizes a DeclSyntax.
//
// idx is the index of this declaration within its parent; first is a pointer to
// a slot where we can store the first DeclSyntax seen, so we can legalize
// against duplicates.
func legalizeSyntax(p *parser, parent classified, idx int, first *ast.DeclSyntax, decl ast.DeclSyntax) {
	if parent.what == taxa.TopLevel && first != nil {
		if !first.IsZero() {
			*first = decl
		}
	}
}

// legalizePackage legalizes a DeclPackage.
//
// idx is the index of this declaration within its parent; first is a pointer to
// a slot where we can store the first DeclPackage seen, so we can legalize
// against duplicates.
func legalizePackage(p *parser, parent classified, idx int, first *ast.DeclPackage, decl ast.DeclPackage) {
}

// legalizeImport legalizes a DeclImport.
//
// imports is a map that classifies DeclImports by the contents of their import string.
// This populates it and uses it to detect duplicates.
func legalizeImport(p *parser, parent classified, decl ast.DeclImport, imports map[string][]ast.DeclImport) {
}
