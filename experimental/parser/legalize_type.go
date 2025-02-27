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
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
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
		if prefixed.Prefix() != keyword.Stream {
			p.Errorf("only the %s modifier may appear in %s", taxa.KeywordStream, what).Apply(
				report.Snippet(prefixed.PrefixToken()),
			)
		}

		if prefixed.Type().Kind() == ast.TypeKindPath {
			legalizePath(p, what.In(), ty.AsPath().Path, pathOptions{AllowAbsolute: true})
			break
		}

		ty = prefixed.Type()
		fallthrough
	default:
		p.Errorf("only message types may appear in %s", what).Apply(
			report.Snippet(ty),
		)
	}
}

// legalizeFieldType legalizes the type of a message field.
func legalizeFieldType(p *parser, ty ast.TypeAny) {
	switch ty.Kind() {
	case ast.TypeKindPath:
		legalizePath(p, taxa.Field.In(), ty.AsPath().Path, pathOptions{AllowAbsolute: true})

	case ast.TypeKindPrefixed:
		ty := ty.AsPrefixed()
		if ty.Prefix() == keyword.Stream {
			p.Errorf("the %s modifier may only appear in a %s", taxa.KeywordStream, taxa.Signature).Apply(
				report.Snippet(ty.PrefixToken()),
			)
		}
		inner := ty.Type()
		switch inner.Kind() {
		case ast.TypeKindPath:
			legalizeFieldType(p, inner)
		case ast.TypeKindPrefixed:
			p.Error(errMoreThanOne{
				first:  ty.PrefixToken(),
				second: inner.AsPrefixed().PrefixToken(),
				what:   taxa.TypePrefix,
			})
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
			p.Errorf("generic types other than `map` are not supported").Apply(
				report.Snippet(ty.Path()),
			)
		case ty.Args().Len() != 2:
			p.Errorf("expected exactly two type arguments, got %d", ty.Args().Len()).Apply(
				report.Snippet(ty.Args()),
			)
		default:
			k, v := ty.AsMap()
			if !k.AsPath().AsPredeclared().IsMapKey() {
				p.Error(errUnexpected{
					what:  k,
					where: taxa.MapKey.In(),
					got:   "non-comparable type",
				}).Apply(
					report.Helpf(
						"a map key must be one of the following types: %s",
						iterx.Join(iterx.Filter(
							predeclared.All(),
							func(p predeclared.Name) bool { return p.IsMapKey() },
						), ", "),
					),
				)
			}

			switch v.Kind() {
			case ast.TypeKindPath:
				legalizeFieldType(p, v)
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
