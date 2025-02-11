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
	"regexp"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// isOrdinaryFilePath matches a "normal looking" file path, for the purposes
// of emitting warnings.
var isOrdinaryFilePath = regexp.MustCompile("[0-9a-zA-Z./_-]*")

// legalizeFile is the entry-point for legalizing a parsed Protobuf file.
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

	if pkg.IsZero() {
		p.Warnf("missing %s", taxa.Package).Apply(
			report.InFile(p.Stream().Path()),
			report.Notef("not explicitly specifying a package places the file"),
			report.Notef("in the unnamed package; using it strongly is discouraged"),
		)
	}
}

// legalizeSyntax legalizes a DeclSyntax.
//
// idx is the index of this declaration within its parent; first is a pointer to
// a slot where we can store the first DeclSyntax seen, so we can legalize
// against duplicates.
func legalizeSyntax(p *parser, parent classified, idx int, first *ast.DeclSyntax, decl ast.DeclSyntax) {
	in := taxa.Syntax
	if decl.IsEdition() {
		in = taxa.Edition
	}

	if parent.what != taxa.TopLevel || first == nil {
		p.Error(errBadNest{parent: parent, child: decl, validParents: taxa.TopLevel.AsSet()})
		return
	}

	file := parent.Spanner.(ast.File) //nolint:errcheck // Implied by == taxa.TopLevel.
	switch {
	case !first.IsZero():
		p.Errorf("unexpected %s", in).Apply(
			report.Snippet(decl),
			report.Snippetf(*first, "previous declaration is here"),
			report.SuggestEdits(
				decl,
				"remove this",
				report.Edit{Start: 0, End: decl.Span().Len()},
			),
			report.Notef("a file may contain at most one `syntax` or `edition` declaration"),
		)
		return
	case idx > 0:
		p.Errorf("unexpected %s", in).Apply(
			report.Snippet(decl),
			report.Snippetf(file.Decls().At(idx-1), "previous declaration is here"),
			// TODO: Add a suggestion to move this up.
			report.Notef("a %s must be the first declaration in a file", in),
		)
		*first = decl
		return
	default:
		*first = decl
	}

	if !decl.Options().IsZero() {
		p.Error(errHasOptions{decl})
	}

	expr := decl.Value()
	var name string
	switch expr.Kind() {
	case ast.ExprKindLiteral:
		if text, ok := expr.AsLiteral().AsString(); ok {
			name = text
			break
		}

		fallthrough
	case ast.ExprKindPath:
		name = expr.Span().Text()

	case ast.ExprKindInvalid:
		return
	default:
		p.Error(errUnexpected{
			what:  expr,
			where: in.In(),
			want:  taxa.String.AsSet(),
		})
		return
	}

	permitted := func() report.DiagnosticOption {
		values := iterx.Join(iterx.FilterMap(syntax.All(), func(s syntax.Syntax) (string, bool) {
			if s.IsEdition() != (in == taxa.Edition) {
				return "", false
			}

			return fmt.Sprintf("%q", s), true
		}), ", ")

		return report.Notef("permitted values: [%s]", values)
	}

	value := syntax.Lookup(name)
	lit := expr.AsLiteral()
	switch {
	case value == syntax.Unknown:
		p.Errorf("unrecognized %s value", in).Apply(
			report.Snippet(expr),
			permitted(),
		)
	case value.IsEdition() && in == taxa.Syntax:
		p.Errorf("unexpected edition in %s", in).Apply(
			report.Snippet(expr),
			permitted(),
		)
	case !value.IsEdition() && in == taxa.Edition:
		p.Errorf("unexpected syntax in %s", in).Apply(
			report.Snippet(expr),
			permitted(),
		)

	case lit.Kind() != token.String:
		span := expr.Span()
		p.Errorf("the value of a %s must be a string literal", in).Apply(
			report.Snippet(span),
			report.SuggestEdits(
				span,
				"add quotes to make this a string literal",
				report.Edit{Start: 0, End: 0, Replace: `"`},
				report.Edit{Start: span.Len(), End: span.Len(), Replace: `"`},
			),
		)

	case !lit.IsZero() && !lit.IsPureString():
		p.Warn(errImpureString{lit.Token, in.In()})
	}
}

// legalizePackage legalizes a DeclPackage.
//
// idx is the index of this declaration within its parent; first is a pointer to
// a slot where we can store the first DeclPackage seen, so we can legalize
// against duplicates.
func legalizePackage(p *parser, parent classified, idx int, first *ast.DeclPackage, decl ast.DeclPackage) {
	if parent.what != taxa.TopLevel || first == nil {
		p.Error(errBadNest{parent: parent, child: decl, validParents: taxa.TopLevel.AsSet()})
		return
	}

	file := parent.Spanner.(ast.File) //nolint:errcheck // Implied by == taxa.TopLevel.
	switch {
	case !first.IsZero():
		p.Errorf("unexpected %s", taxa.Package).Apply(
			report.Snippet(decl),
			report.Snippetf(*first, "previous declaration is here"),
			report.SuggestEdits(
				decl,
				"remove this",
				report.Edit{Start: 0, End: decl.Span().Len()},
			),
			report.Notef("a file must contain exactly one %s", taxa.Package),
		)
		return
	case idx > 0:
		if idx > 1 || file.Decls().At(0).Kind() != ast.DeclKindSyntax {
			p.Warnf("the %s should be placed at the top of the file", taxa.Package).Apply(
				report.Snippet(decl),
				report.Snippetf(file.Decls().At(idx-1), "previous declaration is here"),
				// TODO: Add a suggestion to move this up.
				report.Helpf("a file's %s should immediately follow the `syntax` or `edition` declaration", taxa.Package),
			)
			return
		}
		fallthrough
	default:
		*first = decl
	}

	if !decl.Options().IsZero() {
		p.Error(errHasOptions{decl})
	}

	if decl.Path().IsZero() {
		p.Errorf("missing path in %s", taxa.Package).Apply(
			report.Snippet(decl),
			report.Helpf("to place a file in the unnamed package, remove the %s", taxa.Package),
			report.Helpf("however, using the unnamed package is discouraged"),
		)
	}

	legalizePath(p, taxa.Package.In(), decl.Path(), pathOptions{
		MaxBytes:      512,
		MaxComponents: 101,
	})
}

// legalizeImport legalizes a DeclImport.
//
// imports is a map that classifies DeclImports by the contents of their import string.
// This populates it and uses it to detect duplicates.
func legalizeImport(p *parser, parent classified, decl ast.DeclImport, imports map[string][]ast.DeclImport) {
	in := taxa.Import
	if decl.IsPublic() {
		in = taxa.PublicImport
	} else if decl.IsWeak() {
		in = taxa.WeakImport
	}

	if parent.what != taxa.TopLevel {
		p.Error(errBadNest{parent: parent, child: decl, validParents: taxa.TopLevel.AsSet()})
		return
	}

	if !decl.Options().IsZero() {
		p.Error(errHasOptions{decl})
	}

	expr := decl.ImportPath()
	switch expr.Kind() {
	case ast.ExprKindLiteral:
		lit := expr.AsLiteral()
		if file, ok := lit.AsString(); ok {
			if imports != nil {
				prev := imports[file]
				imports[file] = append(prev, decl)
				if len(prev) == 1 { // Do not bother diagnosing this more than once.
					p.Errorf("file %q imported multiple times", file).Apply(
						report.Snippet(decl),
						report.Snippetf(prev[0], "first imported here"),
					)
				}
				if prev != nil {
					return
				}
			}

			if !expr.AsLiteral().IsPureString() {
				// Only warn for cases where the import is alphanumeric.
				if isOrdinaryFilePath.MatchString(file) {
					p.Warn(errImpureString{lit.Token, in.In()})
				}
			}
			break
		}

		p.Error(errUnexpected{
			what:  expr,
			where: in.In(),
			want:  taxa.String.AsSet(),
		})
		return

	case ast.ExprKindPath:
		p.Error(errUnexpected{
			what:  expr,
			where: in.In(),
			want:  taxa.String.AsSet(),
		}).Apply(
			// TODO: potentially defer this diagnostic to later, when we can
			// perform symbol lookup and figure out what the correct file to
			// import is.
			report.Notef("Protobuf does not support importing symbols by name"),
			report.Notef("instead, try importing a file, e.g. `import \"google/protobuf/descriptor.proto\";`"),
		)
		return

	case ast.ExprKindInvalid:
		if decl.Semicolon().IsZero() {
			// If there is a missing semicolon, this is some other kind of syntax error
			// so we should avoid diagnosing it twice.
			return
		}

		p.Errorf("missing import path in %s", in).Apply(
			report.Snippet(decl),
		)
		return

	default:
		p.Error(errUnexpected{
			what:  expr,
			where: in.In(),
			want:  taxa.String.AsSet(),
		})
		return
	}

	if in == taxa.WeakImport {
		p.Warnf("use of `import weak`").Apply(
			report.Snippet(report.Join(decl.Keyword(), decl.Modifier())),
			report.Notef("`import weak` is deprecated and not supported correctly in most Protobuf implementations"),
		)
	}
}
