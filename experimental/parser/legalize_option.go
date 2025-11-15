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
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
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

	for opt := range seq.Values(entries) {
		legalizeOptionEntry(p, opt, opt.Span())
	}
}

// legalizeCompactOptions is the common path for legalizing options, either
// from an option def or from compact options.
//
// We can't perform type-checking yet, so all we can really do here
// is check that the path is ok for an option. Legalizing the value cannot
// happen until type-checking in IR construction.
func legalizeOptionEntry(p *parser, opt ast.Option, decl source.Span) {
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
		legalizeValue(p, decl, ast.ExprAny{}, opt.Value, taxa.OptionValue.In())
	}
}

// legalizeValue conservatively legalizes a def's value.
func legalizeValue(p *parser, decl source.Span, parent ast.ExprAny, value ast.ExprAny, where taxa.Place) {
	// TODO: Some diagnostics emitted by this function must be suppressed by type
	// checking, which generates more precise diagnostics.

	if slicesx.Among(value.Kind(), ast.ExprKindInvalid, ast.ExprKindError) {
		// Diagnosed elsewhere.
		return
	}

	switch value.Kind() {
	case ast.ExprKindLiteral:
		legalizeLiteral(p, value.AsLiteral())
	case ast.ExprKindPath:
		// Qualified paths are allowed, since we want to diagnose them once we
		// have symbol lookup information so that we can suggest a proper
		// reference.
	case ast.ExprKindPrefixed:
		// - is only allowed before certain identifiers, but which ones is
		// quite tricky to determine. This needs to happen during constant
		// evaluation, so repeating that logic here is somewhat redundant.
	case ast.ExprKindArray:
		array := value.AsArray().Elements()
		switch {
		case parent.IsZero() && where.Subject() == taxa.OptionValue:
			err := p.Error(errUnexpected{
				what:  value,
				where: where,
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
				// err.Apply(report.Helpf("break this %s into one per element", taxa.Option))
			}

		case parent.Kind() == ast.ExprKindArray:
			p.Errorf("nested %ss are not allowed", taxa.Array).Apply(
				report.Snippetf(value, "cannot nest this %s...", taxa.Array),
				report.Snippetf(parent, "...within this %s", taxa.Array),
			)

		default:
			for e := range seq.Values(array) {
				legalizeValue(p, decl, value, e, where)
			}

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

		for kv := range seq.Values(dict.Elements()) {
			want := taxa.NewSet(taxa.FieldName, taxa.ExtensionName, taxa.TypeURL)
			switch kv.Key().Kind() {
			case ast.ExprKindLiteral:
				legalizeLiteral(p, kv.Key().AsLiteral())

			case ast.ExprKindPath:
				path := kv.Key().AsPath()
				first, _ := iterx.First(path.Components)
				if !first.AsExtension().IsZero() {
					// TODO: move this into ir/lower_eval.go
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
					if !elem.AsLiteral().IsZero() {
						// Allow literals in this position, since we can diagnose
						// them better later.
						break
					}

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
				for e := range seq.Values(kv.Value().AsArray().Elements()) {
					if e.Kind() == ast.ExprKindDict {
						continue
					}
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

					break // Only diagnose the first one.
				}
			}

			legalizeValue(p, decl, kv.AsAny(), kv.Value(), where)
		}
	default:
		p.Error(errUnexpected{what: value, where: where})
	}
}

// legalizeLiteral conservatively legalizes a literal.
func legalizeLiteral(p *parser, value ast.ExprLiteral) {
	switch value.Kind() {
	case token.Number:
		n := value.AsNumber()
		if !n.IsValid() {
			return
		}

		what := taxa.Int
		if n.IsFloat() {
			what = taxa.Float
		}

		base := n.Base()
		var validBase bool
		switch base {
		case 2:
			validBase = false
		case 8:
			validBase = n.Prefix().Text() == "0"
		case 10:
			validBase = true
		case 16:
			validBase = what == taxa.Int
		}

		// Diagnose against number literals we currently accept but which are not
		// part of Protobuf.
		if !validBase {
			d := p.Errorf("unsupported base for %s", what)
			if what == taxa.Int {
				switch base {
				case 2:
					v, _ := n.Value().Int(nil)
					d.Apply(
						report.SuggestEdits(value, "use a hexadecimal literal instead", report.Edit{
							Start:   0,
							End:     len(value.Text()),
							Replace: fmt.Sprintf("%#.0x%s", v, n.Suffix().Text()),
						}),
						report.Notef("Protobuf does not support binary literals"),
					)
					return

				case 8:
					d.Apply(
						report.SuggestEdits(value, "remove the `o`", report.Edit{Start: 1, End: 2}),
						report.Notef("octal literals are prefixed with `0`, not `0o`"),
					)
					return
				}
			}

			var name string
			switch base {
			case 2:
				name = "binary"
			case 8:
				name = "octal"
			case 16:
				name = "hexadecimal"
			}

			d.Apply(
				report.Snippet(value),
				report.Notef("Protobuf does not support %s %ss", name, what),
			)
			return
		}

		if suffix := n.Suffix(); suffix.Text() != "" {
			p.Errorf("unrecognized suffix for %s", what).Apply(
				report.SuggestEdits(suffix, "delete it", report.Edit{
					Start: 0,
					End:   len(suffix.Text()),
				}),
			)
			return
		}

		if n.HasSeparators() {
			p.Errorf("%s contains underscores", what).Apply(
				report.SuggestEdits(value, "remove these underscores", report.Edit{
					Start:   0,
					End:     len(value.Text()),
					Replace: strings.ReplaceAll(value.Text(), "_", ""),
				}),
				report.Notef("Protobuf does not support Go/Java/Rust-style thousands separators"),
			)
			return
		}

	case token.String:
		s := value.AsString()
		if sigil := s.Prefix(); sigil.Text() != "" {
			p.Errorf("unrecognized prefix for %s", taxa.String).Apply(
				report.SuggestEdits(sigil, "delete it", report.Edit{
					Start: 0,
					End:   len(sigil.Text()),
				}),
			)
		}
		// NOTE: we do not need to legalize triple-quoted strings:
		// """a""" is just "" "a" "" without whitespace, which have equivalent
		// contents.
	}
}
