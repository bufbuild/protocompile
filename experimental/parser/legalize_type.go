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
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// legalizeMethodParams legalizes part of the signature of a method.
func legalizeMethodParams(p *parser, list ast.TypeList, what taxa.Noun) {
	if list.Len() != 1 {
		p.Errorf("expected exactly one type in %s, got %d", what, list.Len()).Apply(
			report.Snippet(list),
		)
		return
	}

	ty := list.At(0)
	switch ty.Kind() {
	case ast.TypeKindPath:
		legalizePath(p, what.In(), ty.AsPath().Path, pathOptions{AllowAbsolute: true})
	case ast.TypeKindPrefixed:
		prefixed := ty.AsPrefixed()
		var mod ast.TypePrefixed
		for {
			switch {
			case !prefixed.Prefix().IsMethodTypeModifier():
				p.Error(errUnexpectedMod{
					mod:    prefixed.PrefixToken(),
					where:  taxa.Signature.In(),
					syntax: p.syntax,
				})
			case !mod.IsZero():
				p.Error(errMoreThanOne{
					first:  mod.PrefixToken(),
					second: prefixed.PrefixToken(),
					what:   taxa.KeywordStream,
				})
			default:
				mod = prefixed
			}

			switch prefixed.Type().Kind() {
			case ast.TypeKindPath:
				legalizePath(p, what.In(), ty.AsPath().Path, pathOptions{AllowAbsolute: true})
				return
			case ast.TypeKindPrefixed:
				prefixed = prefixed.Type().AsPrefixed()
				continue
			}
			break
		}

		ty = prefixed.Type()
		fallthrough
	default:
		p.Error(errUnexpected{
			what:  ty,
			where: what.In(),
			want:  taxa.NewSet(taxa.MessageType),
		})
	}
}

// legalizeFieldType legalizes the type of a message field.
func legalizeFieldType(p *parser, what taxa.Noun, ty ast.TypeAny, topLevel bool, mod ast.TypePrefixed, oneof ast.DeclDef) {
	expected := taxa.TypePath.AsSet()
	if oneof.IsZero() {
		switch p.syntax {
		case syntax.Proto2:
			expected = taxa.NewSet(
				taxa.KeywordRequired, taxa.KeywordOptional, taxa.KeywordRepeated)
		case syntax.Proto3:
			expected = taxa.NewSet(
				taxa.TypePath, taxa.KeywordOptional, taxa.KeywordRepeated)
		default:
			expected = taxa.NewSet(
				taxa.TypePath, taxa.KeywordRepeated)
		}
	}

	switch ty.Kind() {
	case ast.TypeKindPath:
		if topLevel && p.syntax == syntax.Proto2 && oneof.IsZero() {
			p.Error(errUnexpected{
				what: ty,
				want: expected,
			}).Apply(
				report.SuggestEdits(ty, "use the `optional` modifier", report.Edit{
					Replace: "optional ",
				}),
				report.Notef("modifiers are required in %s", syntax.Proto2),
			)
		}

		legalizePath(p, what.In(), ty.AsPath().Path, pathOptions{AllowAbsolute: true})

	case ast.TypeKindPrefixed:
		ty := ty.AsPrefixed()
		if !mod.IsZero() {
			p.Errorf("multiple modifiers on %v type", taxa.Field).Apply(
				report.Snippet(ty.PrefixToken()),
				report.Snippetf(mod.PrefixToken(), "previous one is here"),
				justify(p.File().Stream(), ty.PrefixToken().Span(), "delete it", justified{
					Edit:    report.Edit{Start: 0, End: ty.PrefixToken().Span().Len()},
					justify: justifyRight,
				}),
			)
		} else {
			if mod.IsZero() {
				mod = ty
			}

			if !oneof.IsZero() {
				d := p.Error(errUnexpected{
					what: ty.PrefixToken(),
					want: expected,
				}).Apply(
					report.Snippetf(oneof, "within this %s", taxa.Oneof),
					justify(p.File().Stream(), ty.PrefixToken().Span(), "delete it", justified{
						Edit:    report.Edit{Start: 0, End: ty.PrefixToken().Span().Len()},
						justify: justifyRight,
					}),
					report.Notef("fields defined as part of a %s may not have modifiers applied to them", taxa.Oneof),
				)
				if ty.Prefix() == keyword.Repeated {
					d.Apply(report.Helpf(
						"to emulate a repeated field in a %s, define a local message type with a single repeated field",
						taxa.Oneof))
				}
			} else {
				switch k := ty.Prefix(); k {
				case keyword.Required:
					switch p.syntax {
					case syntax.Proto2:
						// TODO: This appears verbatim in lower_validate. Move this check
						// into IR lowering?
						p.Warnf("required fields are deprecated").Apply(
							report.Snippet(ty.PrefixToken()),
							report.Helpf(
								"do not attempt to change this to `optional` if the field is "+
									"already in-use; doing so is a wire protocol break"),
						)
					default:
						p.Error(errUnexpected{
							what: ty.PrefixToken(),
							want: expected,
						}).Apply(
							justify(p.File().Stream(), ty.PrefixToken().Span(), "delete it", justified{
								Edit:    report.Edit{Start: 0, End: ty.PrefixToken().Span().Len()},
								justify: justifyRight,
							}),
							report.Helpf("required fields are only permitted in %s; even then, their use is strongly discouraged",
								syntax.Proto2),
						)
					}

				case keyword.Optional:
					if p.syntax.IsEdition() {
						p.Error(errUnexpected{
							what: ty.PrefixToken(),
							want: expected,
						}).Apply(
							justify(p.File().Stream(), ty.PrefixToken().Span(), "delete it", justified{
								Edit:    report.Edit{Start: 0, End: ty.PrefixToken().Span().Len()},
								justify: justifyRight,
							}),
							report.Helpf(
								"in %s, the presence behavior of a singular field "+
									"is controlled with `[feature.field_presence = ...]`, with "+
									"the default being equivalent to %s %s",
								taxa.EditionMode, syntax.Proto2, taxa.KeywordOptional),
							report.Helpf("see <https://protobuf.com/docs/language-spec#field-presence>"),
						)
					}

				case keyword.Repeated:
					break

				default:
					d := p.Error(errUnexpectedMod{
						mod:      ty.PrefixToken(),
						where:    what.On(),
						syntax:   p.syntax,
						noDelete: k == keyword.Option,
					})

					if k == keyword.Option {
						d.Apply(report.SuggestEdits(ty.PrefixToken(), "replace with `optional`", report.Edit{
							Start: 0, End: ty.PrefixToken().Span().Len(),
							Replace: "optional",
						}))
					}
				}
			}
		}

		inner := ty.Type()
		switch inner.Kind() {
		case ast.TypeKindPath, ast.TypeKindPrefixed:
			legalizeFieldType(p, what, inner, false, mod, oneof)
		default:
			p.Error(errUnexpected{
				what:  inner,
				where: taxa.Classify(ty.PrefixToken()).After(),
				want:  taxa.TypePath.AsSet(),
			})
		}

	case ast.TypeKindGeneric:
		ty := ty.AsGeneric()
		switch {
		case ty.Path().AsPredeclared() != predeclared.Map:
			p.Errorf("generic types other than %s are not supported", taxa.PredeclaredMap).Apply(
				report.Snippet(ty.Path()),
			)
		case !oneof.IsZero():
			p.Errorf("map fields are not allowed inside of a %s", taxa.Oneof).Apply(
				report.Snippet(ty),
				report.Helpf(
					"to emulate a map field in a %s, fine a local message type with a single map field",
					taxa.Oneof),
			)

		case ty.Args().Len() != 2:
			p.Errorf("expected exactly two type arguments, got %d", ty.Args().Len()).Apply(
				report.Snippet(ty.Args()),
			)
		default:
			k, v := ty.AsMap()

			switch k.Kind() {
			case ast.TypeKindPath:
				legalizeFieldType(p, what, k, false, ast.TypePrefixed{}, oneof)
			case ast.TypeKindPrefixed:
				p.Error(errUnexpected{
					what:  k.AsPrefixed().PrefixToken(),
					where: taxa.MapKey.In(),
				})
			default:
				p.Error(errUnexpected{
					what:  k,
					where: taxa.MapKey.In(),
					want:  taxa.TypePath.AsSet(),
				})
			}

			switch v.Kind() {
			case ast.TypeKindPath:
				legalizeFieldType(p, what, v, false, ast.TypePrefixed{}, oneof)
			case ast.TypeKindPrefixed:
				p.Error(errUnexpected{
					what:  v.AsPrefixed().PrefixToken(),
					where: taxa.MapValue.In(),
				})
			default:
				p.Error(errUnexpected{
					what:  v,
					where: taxa.MapValue.In(),
					want:  taxa.TypePath.AsSet(),
				})
			}
		}
	}
}
