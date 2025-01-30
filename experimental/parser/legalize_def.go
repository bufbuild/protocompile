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
	if def.IsCorrupt() {
		return
	}

	kind := def.Classify()
	if !validDefParents[kind].Has(parent.what) {
		p.Error(errBadNest{parent: parent, child: def})
	}

	switch kind {
	case ast.DefKindMessage, ast.DefKindEnum, ast.DefKindService, ast.DefKindOneof, ast.DefKindExtend:
		legalizeTypeDefLike(p, taxa.Classify(def), def)
	case ast.DefKindField, ast.DefKindEnumValue, ast.DefKindGroup:
		legalizeFieldLike(p, taxa.Classify(def), def)
	case ast.DefKindOption:
		legalizeOption(p, def)
	case ast.DefKindMethod:
		legalizeMethod(p, def)
	}
}

// legalizeMessageLike legalizes something that resembles a type definition:
// namely, messages, enums, oneofs, services, and extension blocks.
func legalizeTypeDefLike(p *parser, what taxa.Noun, def ast.DeclDef) {
	switch {
	case def.Name().IsZero():
		def.MarkCorrupt()
		kw := taxa.Keyword(def.Keyword().Text())
		p.Errorf("missing name %v", kw.After()).Apply(
			report.Snippet(def),
		)

	case what == taxa.Extend:
		legalizePath(p, what.In(), def.Name(), pathOptions{})

	case what != taxa.Extend && def.Name().AsIdent().IsZero():
		def.MarkCorrupt()
		kw := taxa.Keyword(def.Keyword().Text())
		p.Error(errUnexpected{
			what:  def.Name(),
			where: kw.After(),
			want:  taxa.Ident.AsSet(),
		}).Apply(
			report.Notef("the name of a %s must be a single identifier", what),
			// TODO: Include a help that says to stick this into a file with
			// the right package.
		)
	}

	hasValue := !def.Equals().IsZero() || !def.Value().IsZero()
	if hasValue {
		p.Error(errUnexpected{
			what:  report.Join(def.Equals(), def.Value()),
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

// legalizeMessageLike legalizes something that resembles a field definition:
// namely, fields, groups, and enum values.
func legalizeFieldLike(p *parser, what taxa.Noun, def ast.DeclDef) {
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

	// NOTE: We do not legalize a missing value for fields and enum values
	// here; instead, that happens during IR lowering. This is because we want
	// to be able to include a suggested field number, but we cannot do that
	// until much later, when we have evaluated expressions.

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

	if what == taxa.Field {
		legalizeFieldType(p, def.Type())
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
			what:  report.Join(def.Equals(), def.Value()),
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
				report.Snippet(def),
			)
		} else {
			legalizeMethodParams(p, sig.Inputs(), taxa.MethodIns)
		}

		if sig.Outputs().Span().IsZero() {
			def.MarkCorrupt()
			p.Errorf("missing %v in %v", taxa.MethodOuts, taxa.Method).Apply(
				report.Snippet(def),
			)
		} else {
			legalizeMethodParams(p, sig.Outputs(), taxa.MethodOuts)
		}
	}

	// Methods are unique in that they can be either end in a ; or a {}.
	// The parser already checks for defs to end with either one of these,
	// so we don't need to do anything here.

	if options := def.Options(); !options.IsZero() {
		p.Error(errHasOptions{def}).Apply(
			report.Notef("service method options are applied using %v", taxa.KeywordOption),
			report.Notef("declarations in the %v following the method definition", taxa.Braces),
			// TODO: Generate a suggestion for this.
		)
	}
}
