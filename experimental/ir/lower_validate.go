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

package ir

import (
	"path"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// diagnoseUnusedImports generates diagnostics for each unused import.
func diagnoseUnusedImports(f File, r *report.Report) {
	for imp := range seq.Values(f.Imports()) {
		if imp.Used {
			continue
		}

		r.Warnf("unused import \"%s\"", f.Path()).Apply(
			report.Snippet(imp.Decl.ImportPath()),
			report.SuggestEdits(imp.Decl, "delete it", report.Edit{
				Start: 0, End: imp.Decl.Span().Len(),
			}),
			report.Helpf("no symbols from this file are referenced"),
		)
	}
}

// validateConstraints validates miscellaneous constraints that depend on the
// whole IR being constructed properly.
func validateConstraints(f File, r *report.Report) {
	builtins := f.Context().builtins()

	// https://protobuf.com/docs/language-spec#option-validation
	javaUTF8 := f.Options().Field(builtins.JavaUTF8)
	if !javaUTF8.IsZero() && f.Syntax().IsEdition() {
		want := "DEFAULT"
		if b, _ := javaUTF8.AsBool(); b {
			want = "VERIFY"
		}

		r.Errorf("cannot set `%s` in %s", javaUTF8.Field().Name(), taxa.EditionMode).Apply(
			report.Snippet(javaUTF8.MessageKeys().At(0)),
			javaUTF8.suggestEdit("features.(pb.java).utf8_validation", want, "replace with `features.(pb.java).utf8_validation`"),
		)
	}
	optimize := f.Options().Field(builtins.OptimizeFor)
	if v, _ := optimize.AsInt(); v != 3 { // google.protobuf.FileOptions.LITE_RUNTIME
		for imp := range seq.Values(f.Imports()) {
			impOptimize := imp.Options().Field(builtins.OptimizeFor)
			if v, _ := impOptimize.AsInt(); v == 3 { // google.protobuf.FileOptions.LITE_RUNTIME
				r.Errorf("`LITE_RUNTIME` file imported in non-`LITE_RUNTIME` file").Apply(
					report.Snippet(imp.Decl.ImportPath()),
					report.Snippetf(optimize.AST(), "optimization level set here"),
					report.Snippetf(impOptimize.AST(), "`%s` set as `LITE_RUNTIME` here", path.Base(imp.Path())),
					report.Helpf("files using `LITE_RUNTIME` compile to types that use `MessageLite` or "+
						"equivalent in some runtimes, which ordinary message types cannot depend on"),
				)
			}
		}
	}
	defaultPresence := f.FeatureSet().Lookup(builtins.FeaturePresence).Value()
	if v, _ := defaultPresence.AsInt(); v == 3 { // google.protobuf.FeatureSet.LEGACY_REQUIRED
		r.Errorf("cannot set `LEGACY_REQUIRED` at the file level").Apply(
			report.Snippet(defaultPresence.AST()),
		)
	}

	for ty := range seq.Values(f.AllTypes()) {
		if ty.IsEnum() {
			if ty.Members().Len() == 0 {
				r.Errorf("%s must define at least one value", taxa.EnumType).Apply(
					report.Snippet(ty.AST()),
				)
				continue
			}

			first := ty.Members().At(0)
			if first.Number() != 0 && !ty.IsClosedEnum() {
				// Figure out why this enum is open.
				feature := ty.FeatureSet().Lookup(builtins.FeatureEnum)
				why := feature.Value().AST().Span()
				if feature.IsDefault() {
					why = f.AST().Syntax().Value().Span()
				}

				r.Errorf("first value of open enum must be `0`").Apply(
					report.Snippet(first.AST().Value()),
					report.Snippetf(why, "`%s` specified as open here", ty.FullName()),
					report.Helpf("open enums must define a zero value, and it must be the first one"),
				)
			}

			continue
		}

		validateMessageSet(ty, r)

		for oneof := range seq.Values(ty.Oneofs()) {
			if oneof.Members().Len() == 0 {
				r.Errorf("oneof must define at least one member").Apply(
					report.Snippet(oneof.AST()),
				)
			}
		}
	}

	for m := range f.AllMembers() {
		// https://protobuf.com/docs/language-spec#field-option-validation
		validatePacked(m, r)
		validateCType(m, r)
		validateLazy(m, r)
		validateJSType(m, r)

		validatePresence(m, r)
		validateUTF8(m, r)
		validateMessageEncoding(m, r)

		// NOTE: extensions already cannot be map fields, so we don't need to
		// validate them.
		if m.IsExtension() && !m.IsMap() {
			extendee := m.Container()

			if extendee.IsMessageSet() {
				if m.IsRepeated() {
					_, repeated := iterx.Find(m.AST().Type().Prefixes(), func(ty ast.TypePrefixed) bool {
						return ty.Prefix() == keyword.Repeated
					})

					r.Errorf("repeated message set extension").Apply(
						report.Snippet(repeated.PrefixToken()),
						report.Snippetf(extendee.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
						report.Helpf("message set extensions must be singular message fields"),
					)
				}
				if !m.Element().IsMessage() {
					r.Errorf("non-message message set extension").Apply(
						report.Snippet(m.AST().Type().RemovePrefixes()),
						report.Snippetf(extendee.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
						report.Helpf("message set extensions must be singular message fields"),
					)
				}
			}
		}
	}
}

func validateMessageSet(ty Type, r *report.Report) {
	if !ty.IsMessageSet() {
		return
	}

	f := ty.Context().File()
	builtins := ty.Context().builtins()
	if f.Syntax() == syntax.Proto3 {
		r.Errorf("%s are not supported", taxa.MessageSet).Apply(
			report.Snippet(ty.AST()),
			report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
			report.Snippetf(f.AST().Syntax().Value(), "\"proto3\" specified here"),
			report.Helpf("%ss cannot be defined in \"proto3\"", taxa.MessageSet),
			report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
		)
		return
	}

	r.Warnf("%ss are deprecated", taxa.MessageSet).Apply(
		report.Snippet(ty.AST()),
		report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
		report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
	)

	for member := range seq.Values(ty.Members()) {
		r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
			report.Snippet(member.AST()),
			report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
			report.Helpf("message set types may only declare extension ranges"),
		)
	}

	if ty.ExtensionRanges().Len() == 0 {
		r.Errorf("%s `%s` declares no %ss", taxa.MessageSet, ty.FullName(), taxa.Extensions).Apply(
			report.Snippet(ty.AST()),
			report.Snippetf(ty.Options().Field(builtins.MessageSet).MessageKeys().At(0), "declared as message set here"),
		)
	}
}

func validatePresence(m Member, r *report.Report) {
	if m.IsEnumValue() {
		return
	}

	builtins := m.Context().builtins()
	feature := m.FeatureSet().Lookup(builtins.FeaturePresence)
	if !feature.IsExplicit() {
		return
	}

	switch {
	case !m.IsSingular():
		what := "repeated"
		if m.IsMap() {
			what = "map"
		}

		r.Errorf("expected singular field, found %s field", what).Apply(
			report.Snippet(m.TypeAST()),
			report.Snippetf(
				feature.Value().MessageKeys().At(0),
				"`%s` set here", feature.Field().Name(),
			),
			report.Helpf("`%s` can only be set on singular fields", feature.Field().Name()),
		)

	case m.Presence() == presence.Shared:
		r.Errorf("expected singular field, found oneof member").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(m.Oneof().AST(), "defined in this oneof"),
			report.Snippetf(
				feature.Value().MessageKeys().At(0),
				"`%s` set here", feature.Field().Name(),
			),
			report.Helpf("`%s` cannot be set on oneof members", feature.Field().Name()),
			report.Helpf("all oneof members have explicit presence"),
		)

	case m.IsExtension():
		r.Errorf("expected singular field, found extension").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(
				feature.Value().MessageKeys().At(0),
				"`%s` set here", feature.Field().Name(),
			),
			report.Helpf("`%s` cannot be set on extensions", feature.Field().Name()),
			report.Helpf("all singular extensions have explicit presence"),
		)
	}

	switch v, _ := feature.Value().AsInt(); v {
	case 1: // EXPLICIT
	case 2: // IMPLICIT
		if m.Element().IsMessage() {
			r.Error(errTypeConstraint{
				want: taxa.MessageType,
				got:  m.Element(),
				decl: m.TypeAST(),
			}).Apply(
				report.Snippet(m.TypeAST()),
				report.Snippetf(
					feature.Value().AST(),
					"implicit presence set here",
				),
				report.Helpf("all message-typed fields explicit presence"),
			)
		}
	case 3: // LEGACY_REQUIRED
		r.Warnf("required fields are deprecated").Apply(
			report.Snippet(feature.Value().AST()),
			report.Helpf(
				"do not attempt to change this to `EXPLICIT` if the field is "+
					"already in-use; doing so is a wire protocol break"),
		)
	}
}

// validatePacked validates constraints on the packed option and feature.
func validatePacked(m Member, r *report.Report) {
	if m.IsEnumValue() {
		return
	}

	builtins := m.Context().builtins()
	validate := func(v Value, span report.Span) {
		switch {
		case m.IsSingular() || m.IsMap():
			r.Errorf("expected repeated field, found singular field").Apply(
				report.Snippet(m.TypeAST()),
				report.Snippetf(span, "packed encoding set here"),
				report.Helpf("packed encoding encoding can only be set on repeated fields of integer, float, `bool`, or enum type"),
			)
		case !m.Element().IsPackable():
			r.Error(errTypeConstraint{
				want: "packable type",
				got:  m.Element(),
				decl: m.TypeAST(),
			}).Apply(
				report.Snippetf(span, "packed encoding set here"),
				report.Helpf("packed encoding encoding can only be set on repeated fields of integer, float, `bool`, or enum type"),
			)
		}
	}

	option := m.Options().Field(builtins.Packed)
	if !option.IsZero() {
		if m.Context().File().Syntax().IsEdition() {
			packed, _ := option.AsBool()
			want := "PACKED"
			if !packed {
				want = "EXPANDED"
			}

			r.Errorf("`packed` cannot be set in %s", taxa.EditionMode).Apply(
				report.Snippetf(option.MessageKeys().At(0), "`packed` set here"),
				report.Snippetf(m.Context().File().AST().Syntax().Value(), "edition set here"),
				option.suggestEdit(builtins.FeaturePacked.Name(), want, "replace with `%s`", builtins.FeaturePacked.Name()),
			)
		} else if v, _ := option.AsBool(); v {
			// Don't validate [packed = false], protoc accepts that.
			validate(option, option.AST().Span())
		}
	}

	feature := m.FeatureSet().Lookup(builtins.FeaturePacked)
	if feature.IsExplicit() {
		validate(feature.Value(), feature.Value().MessageKeys().At(0).Span())
	}
}

func validateLazy(m Member, r *report.Report) {
	builtins := m.Context().builtins()

	validate := func(key Member) {
		lazy := m.Options().Field(key)
		if lazy.IsZero() {
			return
		}
		set, _ := lazy.AsBool()

		if !m.Element().IsMessage() {
			r.SoftError(set, errTypeConstraint{
				want: "message type",
				got:  m.Element(),
				decl: m.TypeAST(),
			}).Apply(
				report.Snippetf(lazy.MessageKeys().At(0), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` can only be set on message-typed fields", lazy.Field().Name()),
			)
		}

		if m.IsGroup() {
			r.SoftErrorf(set, "expected length-prefixed field").Apply(
				report.Snippet(m.AST()),
				report.Snippetf(m.AST().KeywordToken(), "groups are not length-prefixed"),
				report.Snippetf(lazy.MessageKeys().At(0), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` only makes sense for length-prefixed messages", lazy.Field().Name()),
			)
		}

		group := m.FeatureSet().Lookup(builtins.FeatureGroup)
		groupValue, _ := group.Value().AsInt()
		if groupValue == 2 { // FeatureSet.DELIMITED
			r.SoftErrorf(set, "expected length-prefixed field").Apply(
				report.Snippet(m.AST()),
				report.Snippetf(group.Value().AST(), "set to use delimited encoding here"),
				report.Snippetf(lazy.MessageKeys().At(0), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` only makes sense for length-prefixed messages", lazy.Field().Name()),
			)
		}
	}

	validate(builtins.Lazy)
	validate(builtins.UnverifiedLazy)
}

func validateJSType(m Member, r *report.Report) {
	builtins := m.Context().builtins()

	option := m.Options().Field(builtins.JSType)
	if option.IsZero() {
		return
	}

	ty := m.Element().Predeclared()
	if !ty.IsInt() || ty.Bits() != 64 {
		r.Error(errTypeConstraint{
			want: "64-bit integer type",
			got:  m.Element(),
			decl: m.TypeAST(),
		}).Apply(
			report.Snippetf(option.MessageKeys().At(0), "`%s` set here", option.Field().Name()),
			report.Helpf("`%s` is specifically for controlling the formatting of large integer types, "+
				"which lose precision when JavaScript converts them into 64-bit IEEE 754 floats", option.Field().Name()),
		)
	}
}

func validateUTF8(m Member, r *report.Report) {
	builtins := m.Context().builtins()

	feature := m.FeatureSet().Lookup(builtins.FeatureUTF8)
	if !feature.IsExplicit() {
		return
	}

	if m.Element().Predeclared() == predeclared.String {
		return
	}
	if k, v := m.Element().EntryFields(); k.Element().Predeclared() == predeclared.String ||
		v.Element().Predeclared() == predeclared.String {
		return
	}
	r.Error(errTypeConstraint{
		want: "`string`",
		got:  m.Element(),
		decl: m.TypeAST(),
	}).Apply(
		report.Snippetf(
			feature.Value().MessageKeys().At(0),
			"`%s` set here", feature.Field().Name(),
		),
		report.Helpf(
			"`%s` can only be set on `string` typed fields, "+
				"or map fields whose key or value is `string`",
			feature.Field().Name(),
		),
	)
}

func validateMessageEncoding(m Member, r *report.Report) {
	builtins := m.Context().builtins()
	feature := m.FeatureSet().Lookup(builtins.FeatureGroup)
	if !feature.IsExplicit() {
		return
	}

	if m.Element().IsMessage() && !m.IsMap() {
		return
	}

	d := r.Error(errTypeConstraint{
		want: taxa.MessageType,
		got:  m.Element(),
		decl: m.TypeAST(),
	}).Apply(
		report.Snippetf(
			feature.Value().MessageKeys().At(0),
			"`%s` set here", feature.Field().Name(),
		),
		report.Helpf(
			"`%s` can only be set on message-typed fields", feature.Field().Name(),
		),
	)

	if m.IsMap() {
		d.Apply(report.Helpf(
			"even though map fields count as repeated message-typed fields, "+
				"`%s` cannot be set on them",
			feature.Field().Name(),
		))
	}
}

func validateCType(m Member, r *report.Report) {
	builtins := m.Context().builtins()
	f := m.Context().File()

	ctype := m.Options().Field(builtins.CType)
	if ctype.IsZero() {
		return
	}

	ctypeValue, _ := ctype.AsInt()

	var want string
	switch ctypeValue {
	case 1: // FieldOptions.STRING
		want = "STRING"
	case 2: // FieldOptions.CORD
		want = "CORD"
	case 3: // FieldOptions.STRING_PIECE
		want = "VIEW"
	}

	is2023 := f.Syntax() == syntax.Edition2023
	switch {
	case f.Syntax() > syntax.Edition2023:
		r.Errorf("`%s` is not supported after %s",
			ctype.Field().Name(), prettyEdition(syntax.Edition2023)).Apply(
			report.Snippet(ctype.MessageKeys().At(0)),
			report.Snippetf(f.AST().Syntax().Value(), "edition declared here"),
			ctype.suggestEdit("features.(pb.cpp).string_type", want, "replace with `features.(pb.cpp).string_type`"),
		)

	case !m.Element().Predeclared().IsString():
		d := r.SoftError(is2023, errTypeConstraint{
			want: "`string` or `bytes`",
			got:  m.Element(),
			decl: m.TypeAST(),
		}).Apply(
			report.Snippetf(ctype.MessageKeys().At(0), "`%s` set here", ctype.Field().Name()),
		)

		if !is2023 {
			d.Apply(report.Helpf("this was previously accepted; it becomes a hard error in %s", prettyEdition(syntax.Edition2023)))
		}

	case m.IsExtension() && ctypeValue == 1: // google.protobuf.FieldOptions.CORD
		d := r.SoftErrorf(is2023, "cannot use `CORD` on an extension field").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(ctype.AST(), "`CORD` set here"),
		)

		if !is2023 {
			d.Apply(report.Helpf("this was previously accepted; it becomes a hard error in %s", prettyEdition(syntax.Edition2023)))
		}

	case is2023:
		r.Warnf("`%s` is not supported after %s",
			ctype.Field().Name(), prettyEdition(syntax.Edition2023)).Apply(
			report.Snippet(ctype.MessageKeys().At(0)),
			report.Snippetf(f.AST().Syntax().Value(), "edition declared here"),
			ctype.suggestEdit("features.(pb.cpp).string_type", want, "replace with `features.(pb.cpp).string_type`"),
		)
	}
}
