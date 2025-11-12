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
	"unicode"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Map of a def kind to the valid parents it can have.
//
// We use taxa.Set here because it already exists and is pretty cheap.
var validDefParents = [...]taxa.Set{
	ast.DefKindMessage:   taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindEnum:      taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindService:   taxa.NewSet(taxa.TopLevel),
	ast.DefKindExtend:    taxa.NewSet(taxa.TopLevel, taxa.Message, taxa.Group),
	ast.DefKindField:     taxa.NewSet(taxa.Message, taxa.Group, taxa.Extend, taxa.Oneof),
	ast.DefKindOneof:     taxa.NewSet(taxa.Message, taxa.Group),
	ast.DefKindGroup:     taxa.NewSet(taxa.Message, taxa.Group, taxa.Extend),
	ast.DefKindEnumValue: taxa.NewSet(taxa.Enum),
	ast.DefKindMethod:    taxa.NewSet(taxa.Service),
	ast.DefKindOption: taxa.NewSet(
		taxa.TopLevel, taxa.Message, taxa.Enum, taxa.Service,
		taxa.Oneof, taxa.Group, taxa.Method,
	),
}

// legalizeDef legalizes a definition.
//
// It will mark the definition as corrupt if it encounters any particularly
// egregious problems.
func legalizeDef(p *parser, parent classified, def ast.DeclDef) {
	kind := def.Classify()
	if !validDefParents[kind].Has(parent.what) {
		p.Error(errBadNest{parent: parent, child: def, validParents: validDefParents[kind]})
	}

	switch kind {
	case ast.DefKindMessage, ast.DefKindEnum, ast.DefKindService, ast.DefKindOneof, ast.DefKindExtend:
		legalizeTypeDefLike(p, taxa.Classify(def), def)
	case ast.DefKindField, ast.DefKindEnumValue, ast.DefKindGroup:
		legalizeFieldLike(p, taxa.Classify(def), def, parent)
	case ast.DefKindOption:
		legalizeOption(p, def)
	case ast.DefKindMethod:
		legalizeMethod(p, def)
	}
}

// legalizeTypeDefLike legalizes something that resembles a type definition:
// namely, messages, enums, oneofs, services, and extension blocks.
func legalizeTypeDefLike(p *parser, what taxa.Noun, def ast.DeclDef) {
	switch {
	case def.Name().IsZero():
		def.MarkCorrupt()
		kw := taxa.Keyword(def.Keyword())
		p.Errorf("missing name %v", kw.After()).Apply(
			report.Snippet(def),
		)

	case what == taxa.Extend:
		legalizePath(p, what.In(), def.Name(), pathOptions{AllowAbsolute: true})

	case def.Name().AsIdent().IsZero():
		def.MarkCorrupt()
		kw := taxa.Keyword(def.Keyword())

		err := errUnexpected{
			what:  def.Name(),
			where: kw.After(),
			want:  taxa.Ident.AsSet(),
		}
		// Look for a separator, and use that instead. We can't "just" pick out
		// the first separator, because def.Name might be a one-component
		// extension path, e.g. (a.b.c).
		def.Name().Components(func(pc ast.PathComponent) bool {
			if pc.Separator().IsZero() {
				return true
			}

			err = errUnexpected{
				what:  pc.Separator(),
				where: taxa.Ident.In(),

				repeatUnexpected: true,
			}
			return false
		})

		p.Error(err).Apply(
			report.Notef("the name of a %s must be a single identifier", what),
			// TODO: Include a help that says to stick this into a file with
			// the right package.
		)
	}

	for mod := range def.Prefixes() {
		isType := what == taxa.Message || what == taxa.Enum

		if isType && mod.Prefix().IsTypeModifier() {
			p.Error(errRequiresEdition{
				edition: syntax.Edition2024,
				node:    mod.PrefixToken(),
				decl:    p.syntaxNode,

				unimplemented: p.syntax >= syntax.Edition2024,
			})
			continue
		}

		suggestExport := isType && mod.Prefix() == keyword.Public
		d := p.Error(errUnexpectedMod{
			mod:      mod.PrefixToken(),
			where:    what.On(),
			syntax:   p.syntax,
			noDelete: suggestExport,
		})

		if suggestExport {
			d.Apply(report.SuggestEdits(mod, "replace with `export`", report.Edit{
				Start: 0, End: mod.Span().Len(),
				Replace: "export",
			}))
		}
	}

	hasValue := !def.Equals().IsZero() || !def.Value().IsZero()
	if hasValue {
		p.Error(errUnexpected{
			what:  source.Join(def.Equals(), def.Value()),
			where: what.In(),
			got:   taxa.Classify(def.Value()),
		})
	}

	if sig := def.Signature(); !sig.IsZero() {
		p.Error(errHasSignature{def})
	}

	if def.Body().IsZero() {
		// NOTE: There is currently no way to trip this diagnostic, because
		// a message with no body is interpreted as a field.
		p.Errorf("missing body for %v", what).Apply(
			report.Snippet(def),
		)
	}

	if options := def.Options(); !options.IsZero() {
		p.Error(errHasOptions{def})
	}
}

// legalizeFieldLike legalizes something that resembles a field definition:
// namely, fields, groups, and enum values.
func legalizeFieldLike(p *parser, what taxa.Noun, def ast.DeclDef, parent classified) {
	if def.Name().IsZero() {
		def.MarkCorrupt()
		p.Errorf("missing name %v", what.In()).Apply(
			report.Snippet(def),
		)
	} else if def.Name().AsIdent().IsZero() {
		def.MarkCorrupt()
		p.Error(errUnexpected{
			what:  def.Name(),
			where: what.In(),
			want:  taxa.Ident.AsSet(),
		})
	}
	tag := taxa.FieldTag
	if def.Classify() == ast.DefKindEnumValue {
		tag = taxa.EnumValue
	}
	if def.Value().IsZero() {
		p.Errorf("missing %v in declaration", tag).Apply(
			report.Snippet(def),
			// TODO: We do not currently provide a suggested field number for
			// cases where that is permitted, such as for non-extension-fields.
			//
			// However, that cannot happen until after IR lowering. Once that's
			// implemented, we must come back here and set it up so that this
			// diagnostic can be overridden by a later one, probably using
			// diagnostic tags.
		)
	} else {
		legalizeValue(p, def.Span(), ast.ExprAny{}, def.Value(), tag.In())
	}

	if sig := def.Signature(); !sig.IsZero() {
		p.Error(errHasSignature{def})
	}

	switch what {
	case taxa.Group:
		if def.Body().IsZero() {
			p.Errorf("missing body for %v", what).Apply(
				report.Snippet(def),
			)
		}

		name := def.Name().AsIdent().Text()
		var capitalized bool
		for _, r := range name {
			capitalized = unicode.IsUpper(r)
			break
		}
		if !capitalized {
			p.Errorf("group names must start with an uppercase letter").Apply(
				report.Snippet(def.Name()),
			)
		}

		if p.syntax == syntax.Proto2 {
			p.Warnf("group syntax is deprecated").Apply(
				report.Snippet(def.Type().RemovePrefixes()),
				report.Notef("group syntax is not available in proto3 or editions"),
			)
		} else {
			p.Errorf("group syntax is not supported").Apply(
				report.Snippet(def.Type().RemovePrefixes()),
				report.Notef("group syntax is only available in proto2"),
			)
		}

	case taxa.Field, taxa.EnumValue:
		if body := def.Body(); !body.IsZero() {
			p.Error(errUnexpected{
				what:  body,
				where: what.In(),
			})
		}
	}

	if options := def.Options(); !options.IsZero() {
		legalizeCompactOptions(p, options)
	}

	if what == taxa.Field || what == taxa.Group {
		var oneof ast.DeclDef
		if parent.what == taxa.Oneof {
			oneof, _ = parent.Spanner.(ast.DeclDef)
		}
		legalizeFieldType(p, what, def.Type(), true, ast.TypePrefixed{}, oneof)
	}
}

// legalizeOption legalizes an option definition (see legalize_option.go).
func legalizeOption(p *parser, def ast.DeclDef) {
	if sig := def.Signature(); !sig.IsZero() {
		p.Error(errHasSignature{def})
	}

	if body := def.Body(); !body.IsZero() {
		p.Error(errUnexpected{
			what:  body,
			where: taxa.Option.In(),
		})
	}

	if options := def.Options(); !options.IsZero() {
		p.Error(errHasOptions{def})
	}

	legalizeOptionEntry(p, def.AsOption().Option, def.Span())
}

// legalizeMethod legalizes a service method.
func legalizeMethod(p *parser, def ast.DeclDef) {
	if def.Name().IsZero() {
		def.MarkCorrupt()
		p.Errorf("missing name %v", taxa.Method.In()).Apply(
			report.Snippet(def),
		)
	} else if def.Name().AsIdent().IsZero() {
		def.MarkCorrupt()
		p.Error(errUnexpected{
			what:  def.Name(),
			where: taxa.Method.In(),
			want:  taxa.Ident.AsSet(),
		})
	}

	hasValue := !def.Equals().IsZero() || !def.Value().IsZero()
	if hasValue {
		p.Error(errUnexpected{
			what:  source.Join(def.Equals(), def.Value()),
			where: taxa.Method.In(),
			got:   taxa.Classify(def.Value()),
		})
	}

	sig := def.Signature()
	if sig.IsZero() {
		def.MarkCorrupt()
		p.Errorf("missing %v in %v", taxa.Signature, taxa.Method).Apply(
			report.Snippet(def),
		)
	} else {
		// There are cases where part of the signature is present, but the
		// span for one or the other half is zero because there were no brackets
		// or type.
		if sig.Inputs().Span().IsZero() {
			def.MarkCorrupt()
			p.Errorf("missing %v in %v", taxa.MethodIns, taxa.Method).Apply(
				report.Snippetf(def.Name(), "expected %s after this", taxa.Parens),
			)
		} else {
			legalizeMethodParams(p, sig.Inputs(), taxa.MethodIns)
		}

		if sig.Outputs().Span().IsZero() {
			def.MarkCorrupt()
			var after source.Spanner
			var expected taxa.Noun
			switch {
			case !sig.Returns().IsZero():
				after = sig.Returns()
				expected = taxa.Parens
			case !sig.Inputs().IsZero():
				after = sig.Inputs()
				expected = taxa.ReturnsParens
			default:
				after = def.Name()
				expected = taxa.ReturnsParens
			}

			p.Errorf("missing %v in %v", taxa.MethodOuts, taxa.Method).Apply(
				report.Snippetf(after, "expected %s after this", expected),
			)
		} else {
			legalizeMethodParams(p, sig.Outputs(), taxa.MethodOuts)
		}
	}

	for mod := range def.Prefixes() {
		p.Error(errUnexpectedMod{
			mod:    mod.PrefixToken(),
			where:  taxa.Method.On(),
			syntax: p.syntax,
		})
	}

	// Methods are unique in that they can end in either a ; or a {}.
	// The parser already checks for defs to end with either one of these,
	// so we don't need to do anything here.

	if options := def.Options(); !options.IsZero() {
		p.Error(errHasOptions{def}).Apply(
			report.Notef(
				"service method options are applied using %v; declarations in the %v following the method definition",
				taxa.KeywordOption, taxa.Braces,
			),
			// TODO: Generate a suggestion for this.
		)
	}
}
