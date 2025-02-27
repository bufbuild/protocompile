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

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// legalizeCompactOptions legalizes a [...] of options.
//
// All this really does is check that opt is non-empty and then forwards each
// entry to [legalizeOptionEntry].
func legalizeCompactOptions(p *parser, opts ast.CompactOptions) {
	entries := opts.Entries()
	if entries.Len() == 0 {
		p.Errorf("%s cannot be empty", taxa.CompactOptions).Apply(
			report.Snippetf(opts, "help: remove this"),
		)
		return
	}

	seq.Values(entries)(func(opt ast.Option) bool {
		legalizeOptionEntry(p, opt, opt.Span())
		return true
	})
}

// legalizeCompactOptions is the common path for legalizing options, either
// from an option def or from compact options.
//
// We can't perform type-checking yet, so all we can really do here
// is check that the path is ok for an option. Legalizing the value cannot
// happen until type-checking in IR construction.
func legalizeOptionEntry(p *parser, opt ast.Option, decl report.Span) {
	if opt.Path.IsZero() {
		p.Errorf("missing %v path", taxa.Option).Apply(
			report.Snippet(decl),
		)

		// Don't bother legalizing if the value is zero. That can only happen
		// when the user writes just option;, which will produce two very
		// similar diagnostics.
		return
	}

	legalizePath(p, taxa.Option.In(), opt.Path, pathOptions{
		AllowExts: true,
	})

	if opt.Value.IsZero() {
		p.Errorf("missing %v", taxa.OptionValue).Apply(
			report.Snippet(decl),
		)
	} else {
		legalizeOptionValue(p, decl, ast.ExprAny{}, opt.Value)
	}
}

// legalizeOptionValue conservatively legalizes an option value.
func legalizeOptionValue(p *parser, decl report.Span, parent ast.ExprAny, value ast.ExprAny) {
	// TODO: Some diagnostics emitted by this function must be suppressed by type
	// checking, which generates more precise diagnostics.

	if slicesx.Among(value.Kind(), ast.ExprKindInvalid, ast.ExprKindError) {
		// Diagnosed elsewhere.
		return
	}

	switch value.Kind() {
	case ast.ExprKindLiteral:
		// All literals are allowed.
	case ast.ExprKindPath:
		if value.AsPath().AsIdent().IsZero() {
			p.Error(errUnexpected{
				what:  value,
				where: taxa.OptionValue.In(),
				want:  taxa.Ident.AsSet(),
			})
		}
	case ast.ExprKindPrefixed:
		value := value.AsPrefixed()
		if value.Expr().IsZero() {
			return
		}

		//nolint:gocritic // Intentional single-case switch.
		switch value.Prefix() {
		case keyword.Minus:
			ok := value.Expr().AsLiteral().Kind() == token.Number
			if path := value.Expr().AsPath(); !path.IsZero() {
				// A minus sign may precede inf or nan, but it may also precede
				// any identifier when inside of a message literal.
				ok = (parent.Kind() == ast.ExprKindField && !path.AsIdent().IsZero()) ||
					slicesx.Among(path.AsPredeclared(), predeclared.Inf, predeclared.NAN)
			}

			if !ok {
				p.Error(errUnexpected{
					what:  value.Expr(),
					where: taxa.Minus.After(),
					want:  taxa.NewSet(taxa.Int, taxa.Float),
				})
			}
		}
	case ast.ExprKindArray:
		array := value.AsArray().Elements()
		switch {
		case parent.IsZero():
			err := p.Error(errUnexpected{
				what:  value,
				where: taxa.OptionValue.In(),
			}).Apply(
				report.Notef("%ss can only appear inside of %ss", taxa.Array, taxa.Dict),
			)

			switch array.Len() {
			case 0:
				err.Apply(report.SuggestEdits(
					decl,
					fmt.Sprintf("delete this option; an empty %s has no effect", taxa.Array),
					report.Edit{Start: 0, End: decl.Len()},
				))
			case 1:
				elem := array.At(0)
				if !slicesx.Among(elem.Kind(),
					// This check avoids making nonsensical suggestions.
					ast.ExprKindInvalid, ast.ExprKindError,
					ast.ExprKindRange, ast.ExprKindField) {
					err.Apply(report.SuggestEdits(
						value,
						"delete the brackets; this is equivalent for repeated fields",
						report.Edit{Start: 0, End: 1},
						report.Edit{Start: value.Span().Len() - 1, End: value.Span().Len()},
					))
					break
				}
				fallthrough
			default:
				// TODO: generate a suggestion for this.
				err.Apply(report.Helpf("break this %s into one per element", taxa.Option))
			}

		case parent.Kind() == ast.ExprKindArray:
			p.Errorf("nested %ss are not allowed", taxa.Array).Apply(
				report.Snippetf(value, "cannot nest this %s...", taxa.Array),
				report.Snippetf(parent, "...within this %s", taxa.Array),
			)

		default:
			seq.Values(array)(func(e ast.ExprAny) bool {
				legalizeOptionValue(p, decl, value, e)
				return true
			})

			if parent.Kind() == ast.ExprKindField && array.Len() == 0 {
				p.Warnf("empty %s has no effect", taxa.Array).Apply(
					report.Snippet(value),
					report.SuggestEdits(
						parent,
						fmt.Sprintf("delete this %s", taxa.DictField),
						report.Edit{Start: 0, End: parent.Span().Len()},
					),
					report.Notef(`repeated fields do not distinguish "empty" and "missing" states`),
				)
			}
		}
	case ast.ExprKindDict:
		dict := value.AsDict()

		// Legalize against <...> in all cases, but only emit a warning when they
		// are not strictly illegal.
		if dict.Braces().Keyword() == keyword.Angles {
			var err *report.Diagnostic
			if parent.IsZero() {
				err = p.Errorf("cannot use %s for %s here", taxa.Angles, taxa.Dict)
			} else {
				err = p.Warnf("using %s for %s is not recommended", taxa.Angles, taxa.Dict)
			}

			err.Apply(
				report.Snippet(value),
				report.SuggestEdits(
					dict,
					fmt.Sprintf("use %s instead", taxa.Braces),
					report.Edit{Start: 0, End: 1, Replace: "{"},
					report.Edit{Start: dict.Span().Len() - 1, End: dict.Span().Len(), Replace: "}"},
				),
				report.Notef("%s are only permitted for sub-messages within a %s, but as top-level option values", taxa.Angles, taxa.Dict),
				report.Helpf("%s %ss are an obscure feature and not recommended", taxa.Angles, taxa.Dict),
			)
		}

		seq.Values(value.AsDict().Elements())(func(kv ast.ExprField) bool {
			want := taxa.NewSet(taxa.FieldName, taxa.ExtensionName, taxa.TypeURL)
			switch kv.Key().Kind() {
			case ast.ExprKindLiteral:
				lit := kv.Key().AsLiteral()
				err := p.Error(errUnexpected{
					what:  lit,
					where: taxa.DictField.In(),
					want:  want,
				})

				if name, _ := lit.AsString(); isASCIIIdent(name) {
					err.Apply(report.SuggestEdits(
						lit,
						"remove the quotes",
						report.Edit{
							Start: 0, End: lit.Span().Len(),
							Replace: name,
						},
					))
				}

			case ast.ExprKindPath:
				path := kv.Key().AsPath()
				first, ok := iterx.OnlyOne(path.Components)
				if !ok || !first.Separator().IsZero() {
					p.Error(errUnexpected{
						what:  path,
						where: taxa.DictField.In(),
						want:  want,
					})
					break
				}
				if !first.AsExtension().IsZero() {
					p.Errorf("cannot name extension field using %s in %s", taxa.Parens, taxa.Dict).Apply(
						report.Snippetf(path, "expected this to be wrapped in %s instead", taxa.Brackets),
						report.SuggestEdits(
							path,
							fmt.Sprintf("replace the %s with %s", taxa.Parens, taxa.Brackets),
							report.Edit{Start: 0, End: 1, Replace: "["},
							report.Edit{Start: path.Span().Len() - 1, End: path.Span().Len(), Replace: "]"},
						),
					)
				}

			case ast.ExprKindArray:
				elem, ok := iterx.OnlyOne(seq.Values(kv.Key().AsArray().Elements()))
				path := elem.AsPath().Path
				if !ok || path.IsZero() {
					p.Error(errUnexpected{
						what:  kv.Key(),
						where: taxa.DictField.In(),
						want:  want,
					})
					break
				}

				slashIdx, _ := iterx.Find(path.Components, func(pc ast.PathComponent) bool {
					return pc.Separator().Keyword() == keyword.Slash
				})
				if slashIdx != -1 {
					legalizePath(p, taxa.TypeURL.In(), path, pathOptions{AllowSlash: true})
				} else {
					legalizePath(p, taxa.ExtensionName.In(), path, pathOptions{
						// Surprisingly, this extension path cannot be an absolute
						// path!
						AllowAbsolute: false,
					})
				}
			default:
				if !kv.Key().IsZero() {
					p.Error(errUnexpected{
						what:  kv.Key(),
						where: taxa.DictField.In(),
						want:  want,
					})
				}
			}

			if kv.Colon().IsZero() && kv.Value().Kind() == ast.ExprKindArray {
				// When the user writes {a [ ... ]}, every element of the array
				// must be a dict.
				//
				// TODO: There is a version of this diagnostic that requires type
				// information. Namely, {a []} is not allowed if a is not of message
				// type. Arguably, because this syntax does nothing, it should
				// be disallowed...
				seq.Values(kv.Value().AsArray().Elements())(func(e ast.ExprAny) bool {
					if e.Kind() != ast.ExprKindDict {
						p.Error(errUnexpected{
							what:  e,
							where: taxa.Array.In(),
							want:  taxa.Dict.AsSet(),
						}).Apply(
							report.Snippetf(kv.Key(),
								"because this %s is missing a %s",
								taxa.DictField, taxa.Colon),
							report.Notef(
								"the %s can be omitted in a %s, but only if the value is a %s or a %s of them",
								taxa.Colon, taxa.DictField,
								taxa.Dict, taxa.Array),
						)

						return false // Only diagnose the first one.
					}

					return true
				})
			}

			legalizeOptionValue(p, decl, kv.AsAny(), kv.Value())
			return true
		})
	default:
		p.Error(errUnexpected{
			what:  value,
			where: taxa.OptionValue.In(),
		})
	}
}
