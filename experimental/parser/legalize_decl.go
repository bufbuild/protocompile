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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
)

func legalizeDecl(p *parser, parent classified, decl ast.DeclAny) {
	switch decl.Kind() {
	case ast.DeclKindSyntax:
		legalizeSyntax(p, parent, -1, nil, decl.AsSyntax())
	case ast.DeclKindPackage:
		legalizePackage(p, parent, -1, nil, decl.AsPackage())
	case ast.DeclKindImport:
		legalizeImport(p, parent, decl.AsImport(), nil)

	case ast.DeclKindRange:
		legalizeRange(p, parent, decl.AsRange())

	case ast.DeclKindBody:
		body := decl.AsBody()
		braces := body.Braces().Span()
		p.Errorf("unexpected definition body in %v", parent.what).Apply(
			report.Snippet(decl),
			report.SuggestEdits(
				braces,
				"remove these braces",
				report.Edit{Start: 0, End: 1},
				report.Edit{Start: braces.Len() - 1, End: braces.Len()},
			),
		)

		seq.Values(body.Decls())(func(decl ast.DeclAny) bool {
			// Treat bodies as being immediately inlined, hence we pass
			// parent here and not body as the parent.
			legalizeDecl(p, parent, decl)
			return true
		})

	case ast.DeclKindDef:
		def := decl.AsDef()
		legalizeDef(p, parent, def)

		body := def.Body()
		what := classified{def, taxa.Classify(def)}
		seq.Values(body.Decls())(func(decl ast.DeclAny) bool {
			legalizeDecl(p, what, decl)
			return true
		})
	}
}

func legalizeRange(p *parser, parent classified, decl ast.DeclRange) {
}
