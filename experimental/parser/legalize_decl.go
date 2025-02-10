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
	"fmt"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// legalizeDecl legalizes a declaration.
//
// The parent definition is used for determining if a declaration nesting is
// permitted.
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
		body := def.Body()
		what := classified{def, taxa.Classify(def)}

		legalizeDef(p, parent, def)
		seq.Values(body.Decls())(func(decl ast.DeclAny) bool {
			legalizeDecl(p, what, decl)
			return true
		})
	}
}

// legalizeDecl legalizes an extension or reserved range.
func legalizeRange(p *parser, parent classified, decl ast.DeclRange) {
	in := taxa.Extensions
	if decl.IsReserved() {
		in = taxa.Reserved
	}

	var validParent bool
	switch parent.what {
	case taxa.Message:
		validParent = true
	case taxa.Enum:
		validParent = in == taxa.Reserved
	}
	if !validParent {
		p.Error(errBadNest{parent: parent, child: decl})
		return
	}

	if options := decl.Options(); !options.IsZero() {
		if in == taxa.Reserved {
			p.Error(errHasOptions{decl})
		} else {
			legalizeCompactOptions(p, options)
		}
	}

	// We only legalize reserved name productions here, because that depends on
	// the syntax/edition keyword. All other expressions are legalized when we
	// do constant evaluation.

	if in != taxa.Reserved {
		return
	}

	var names, tags []ast.ExprAny
	seq.Values(decl.Ranges())(func(expr ast.ExprAny) bool {
		var isName bool
		switch expr.Kind() {
		case ast.ExprKindPath:
			isName = true
			path := expr.AsPath()
			if !path.AsIdent().IsZero() {
				if m := p.Mode(); m == taxa.SyntaxMode {
					p.Errorf("cannot use %vs in %v in %v", taxa.Ident, in, m).Apply(
						report.Snippet(expr),
						report.Snippetf(p.syntax, "%v is specified here", m),
						report.SuggestEdits(
							expr,
							fmt.Sprintf("quote it to make it into a %v", taxa.String),
							report.Edit{
								Start: 0, End: 0, Replace: `"`,
							},
							report.Edit{
								Start: expr.Span().Len(), End: expr.Span().Len(),
								Replace: `"`,
							},
						),
					)
				}
			}

		case ast.ExprKindLiteral:
			lit := expr.AsLiteral()
			if name, ok := lit.AsString(); ok {
				isName = true

				if m := p.Mode(); m == taxa.EditionMode {
					err := p.Errorf("cannot use %vs in %v in %v", taxa.String, in, m).Apply(
						report.Snippet(expr),
						report.Snippetf(p.syntax, "%v is specified here", m),
					)

					// Only suggest unquoting if it's already an identifier.
					if isASCIIIdent(name) {
						err.Apply(report.SuggestEdits(
							lit, "replace this with an identifier",
							report.Edit{
								Start: 0, End: lit.Span().Len(),
								Replace: name,
							},
						))
					}

					return true
				}

				if !isASCIIIdent(name) {
					field := taxa.Field
					if parent.what == taxa.Enum {
						field = taxa.EnumValue
					}
					p.Errorf("reserved %v name is not a valid identifier", field).Apply(
						report.Snippet(expr),
					)
					return true
				}

				if !lit.IsPureString() {
					p.Warn(errImpureString{lit.Token, in.In()})
				}
			}
		}

		if isName {
			names = append(names, expr)
		} else {
			tags = append(tags, expr)
		}

		return true
	})

	if len(names) > 0 && len(tags) > 0 {
		parentWhat := "field"
		if parent.what == taxa.Enum {
			parentWhat = "value"
		}

		// We want to diagnose whichever element is least common in the range.
		least := names
		most := tags
		leastWhat := "name"
		mostWhat := "tag"
		if len(names) > len(tags) ||
			// When tied, use whichever comes last lexicographically.
			(len(names) == len(tags) && names[0].Span().Start < tags[0].Span().Start) {
			least, most = most, least
			leastWhat, mostWhat = mostWhat, leastWhat
		}

		err := p.Errorf("cannot mix tags and names in %s", taxa.Reserved).Apply(
			report.Snippetf(least[0], "this %s %s must go in its own %s", parentWhat, leastWhat, taxa.Reserved),
			report.Snippetf(most[0], "but expected a %s %s because of this", parentWhat, mostWhat),
		)

		span := decl.Span()
		var edits []report.Edit
		for _, expr := range least {
			// Delete leading whitespace and trailing whitespace (and a comma, too).
			toDelete := expr.Span().GrowLeft(unicode.IsSpace).GrowRight(unicode.IsSpace)
			if r, _ := stringsx.Rune(toDelete.After(), 0); r == ',' {
				toDelete.End++
			}

			edits = append(edits, report.Edit{
				Start: toDelete.Start - span.Start,
				End:   toDelete.End - span.Start,
			})
		}

		// If we're moving the last element out of the range, we need to obliterate
		// the trailing comma.
		comma := slicesx.LastPointer(most).Span()
		if comma.End < slicesx.LastPointer(least).Span().End {
			comma.Start = comma.End
			comma = comma.GrowRight(unicode.IsSpace)
			if r, _ := stringsx.Rune(comma.After(), 0); r == ',' {
				comma.End++
				edits = append(edits, report.Edit{
					Start: comma.Start - span.Start,
					End:   comma.End - span.Start,
				})
			}
		}

		edits = append(edits, report.Edit{
			Start: span.Len(), End: span.Len(),
			Replace: fmt.Sprintf("\n%sreserved %s;", span.Indentation(), iterx.Join(
				iterx.Map(slicesx.Values(least), func(e ast.ExprAny) string { return e.Span().Text() }),
				", ",
			)),
		})

		err.Apply(report.SuggestEdits(
			span,
			fmt.Sprintf("split the %s", taxa.Reserved),
			edits...,
		))
	}
}
