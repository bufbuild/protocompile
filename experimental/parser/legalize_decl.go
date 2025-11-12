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
	"slices"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/errtoken"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
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
		legalizeImport(p, parent, decl.AsImport())

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

		for decl := range seq.Values(body.Decls()) {
			// Treat bodies as being immediately inlined, hence we pass
			// parent here and not body as the parent.
			legalizeDecl(p, parent, decl)
		}

	case ast.DeclKindDef:
		def := decl.AsDef()
		body := def.Body()
		// legalizeDef also calls Classify(def).
		// TODO: try to pass around a classified when possible. Generalize
		// classified toe a generic type?
		what := classified{def, taxa.Classify(def)}

		legalizeDef(p, parent, def)
		for decl := range seq.Values(body.Decls()) {
			legalizeDecl(p, what, decl)
		}
	}
}

// legalizeDecl legalizes an extension or reserved range.
func legalizeRange(p *parser, parent classified, decl ast.DeclRange) {
	in := taxa.Extensions
	validParents := taxa.Message.AsSet()
	if decl.IsReserved() {
		in = taxa.Reserved
		validParents = validParents.With(taxa.Enum)
	}

	if !validParents.Has(parent.what) {
		p.Error(errBadNest{parent: parent, child: decl, validParents: validParents})
		return
	}

	if options := decl.Options(); !options.IsZero() {
		if in == taxa.Reserved {
			p.Error(errHasOptions{decl})
		} else {
			legalizeCompactOptions(p, options)
		}
	}

	want := taxa.NewSet(taxa.Int, taxa.Range)
	if in == taxa.Reserved {
		if p.syntax.IsEdition() {
			want = want.With(taxa.Ident)
		} else {
			want = want.With(taxa.String)
		}
	}

	var names, tags []ast.ExprAny
	for expr := range seq.Values(decl.Ranges()) {
		switch expr.Kind() {
		case ast.ExprKindPath:
			if in != taxa.Reserved {
				break
			}

			names = append(names, expr)

			if p.syntax.IsEdition() {
				break
			}
			p.Errorf("cannot use %vs in %v in %v", taxa.Ident, in, taxa.SyntaxMode).Apply(
				report.Snippet(expr),
				report.Snippetf(p.syntaxNode, "%v is specified here", taxa.SyntaxMode),
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

		case ast.ExprKindLiteral:
			lit := expr.AsLiteral()
			if str := lit.AsString(); !str.IsZero() {
				name := str.Text()
				if in == taxa.Extensions {
					p.Error(errUnexpected{
						what:  expr,
						where: in.In(),
						want:  want,
					})
					break
				}

				names = append(names, expr)
				if p.syntax.IsEdition() {
					err := p.Errorf("cannot use %vs in %v in %v", taxa.String, in, taxa.EditionMode).Apply(
						report.Snippet(expr),
						report.Snippetf(p.syntaxNode, "%v is specified here", taxa.EditionMode),
					)

					// Only suggest unquoting if it's already an identifier.
					if unicodex.IsASCIIIdent(name) {
						err.Apply(report.SuggestEdits(
							lit, "replace this with an identifier",
							report.Edit{
								Start: 0, End: lit.Span().Len(),
								Replace: name,
							},
						))
					}

					break
				}

				if !unicodex.IsASCIIIdent(name) {
					field := taxa.Field
					if parent.what == taxa.Enum {
						field = taxa.EnumValue
					}
					p.Errorf("reserved %v name is not a valid identifier", field).Apply(
						report.Snippet(expr),
					)
					break
				}

				if !str.IsPure() {
					p.Warn(errtoken.ImpureString{Token: lit.Token, Where: in.In()})
				}

				break
			}

			fallthrough

		case ast.ExprKindPrefixed, ast.ExprKindRange:
			tags = append(tags, expr)

		default:
			p.Error(errUnexpected{
				what:  expr,
				where: in.In(),
				want:  want,
			})
		}
	}

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
				iterx.Map(slices.Values(least), func(e ast.ExprAny) string { return e.Span().Text() }),
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
