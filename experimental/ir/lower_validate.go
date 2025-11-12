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
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/report/tags"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var asciiIdent = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]*$`)

// diagnoseUnusedImports generates diagnostics for each unused import.
func diagnoseUnusedImports(f *File, r *report.Report) {
	for imp := range seq.Values(f.Imports()) {
		if imp.Used {
			continue
		}

		r.Warnf("unused import %s", imp.Decl.ImportPath().AsLiteral().Text()).Apply(
			report.Snippet(imp.Decl.ImportPath()),
			report.SuggestEdits(imp.Decl, "delete it", report.Edit{
				Start: 0, End: imp.Decl.Span().Len(),
			}),
			report.Helpf("no symbols from this file are referenced"),
			report.Tag(tags.UnusedImport),
		)
	}
}

// validateConstraints validates miscellaneous constraints that depend on the
// whole IR being constructed properly.
func validateConstraints(f *File, r *report.Report) {
	validateFileOptions(f, r)

	for ty := range seq.Values(f.AllTypes()) {
		validateReservedNames(ty, r)
		switch {
		case ty.IsEnum():
			validateEnum(ty, r)

		case ty.IsMessageSet():
			validateMessageSet(ty, r)
			validateExtensionDeclarations(ty, r)

		case ty.IsMessage():
			for oneof := range seq.Values(ty.Oneofs()) {
				validateOneof(oneof, r)
			}
			validateExtensionDeclarations(ty, r)
		}

		for rr := range seq.Values(ty.ExtensionRanges()) {
			validateExtensionRange(rr, r)
		}
	}

	for m := range f.AllMembers() {
		// https://protobuf.com/docs/language-spec#field-option-validation
		validatePacked(m, r)
		validateCType(m, r)
		validateLazy(m, r)
		validateJSType(m, r)
		validateDefault(m, r)

		validatePresence(m, r)
		validateUTF8(m, r)
		validateMessageEncoding(m, r)

		// NOTE: extensions already cannot be map fields, so we don't need to
		// validate them.
		if m.IsExtension() && !m.IsMap() {
			extendee := m.Container()
			if extendee.IsMessageSet() {
				validateMessageSetExtension(m, r)
			}

			validateDeclaredExtension(m, r)
		}
	}

	i := 0
	for p := range f.arenas.messages.Values() {
		i++
		m := id.WrapRaw(f, id.ID[MessageValue](i), p)
		for v := range m.Fields() {
			// This is a simple way of picking up all of the option values
			// without tripping over custom defaults, which we explicitly should
			// *not* validate.
			validateUTF8Values(v, r)
		}
	}

	for e := range seq.Values(f.AllExtends()) {
		validateExtend(e, r)
	}
}

func validateEnum(ty Type, r *report.Report) {
	builtins := ty.Context().builtins()

	if ty.Members().Len() == 0 {
		r.Errorf("%s must define at least one value", taxa.EnumType).Apply(
			report.Snippet(ty.AST()),
		)
		return
	}

	// Check if allow_alias is actually used. This does not happen in
	// lower_numbers.go because we want to be able to include the allow_alias
	// option span in the diagnostic.
	if ty.AllowsAlias() {
		// Check to see if there are at least two enum values with the same
		// number.
		var hasAlias bool
		numbers := make(map[int32]struct{})
		for member := range seq.Values(ty.Members()) {
			if !mapsx.AddZero(numbers, member.Number()) {
				hasAlias = true
				break
			}
		}

		if !hasAlias {
			option := ty.Options().Field(builtins.AllowAlias)
			r.Errorf("`%s` requires at least one aliasing %s", option.Field().Name(), taxa.EnumValue).Apply(
				report.Snippet(option.OptionSpan()),
			)
		}
	}

	first := ty.Members().At(0)
	if first.Number() != 0 && !ty.IsClosedEnum() {
		// Figure out why this enum is open.
		feature := ty.FeatureSet().Lookup(builtins.FeatureEnum)
		why := feature.Value().ValueAST().Span()
		if feature.IsDefault() {
			why = ty.Context().AST().Syntax().Value().Span()
		}

		r.Errorf("first value of open enum must be zero").Apply(
			report.Snippet(first.AST().Value()),
			report.PageBreak,
			report.Snippetf(why, "this makes `%s` an open enum", ty.FullName()),
			report.Helpf("open enums must define a zero value, and it must be the first one"),
		)
	}
}

func validateFileOptions(f *File, r *report.Report) {
	builtins := f.builtins()

	// https://protobuf.com/docs/language-spec#option-validation
	javaUTF8 := f.Options().Field(builtins.JavaUTF8)
	if !javaUTF8.IsZero() && f.Syntax().IsEdition() {
		want := "DEFAULT"
		if b, _ := javaUTF8.AsBool(); b {
			want = "VERIFY"
		}

		r.Errorf("cannot set `%s` in %s", javaUTF8.Field().Name(), taxa.EditionMode).Apply(
			report.Snippet(javaUTF8.KeyAST()),
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
					report.Snippetf(optimize.ValueAST(), "optimization level set here"),
					report.Snippetf(impOptimize.ValueAST(), "`%s` set as `LITE_RUNTIME` here", path.Base(imp.Path())),
					report.Helpf("files using `LITE_RUNTIME` compile to types that use `MessageLite` or "+
						"equivalent in some runtimes, which ordinary message types cannot depend on"),
				)
			}
		}
	}

	defaultPresence := f.FeatureSet().Lookup(builtins.FeaturePresence).Value()
	if v, _ := defaultPresence.AsInt(); v == 3 { // google.protobuf.FeatureSet.LEGACY_REQUIRED
		r.Errorf("cannot set `LEGACY_REQUIRED` at the file level").Apply(
			report.Snippet(defaultPresence.ValueAST()),
		)
	}
}

func validateReservedNames(ty Type, r *report.Report) {
	for name := range seq.Values(ty.ReservedNames()) {
		member := ty.MemberByInternedName(name.InternedName())
		if member.IsZero() {
			continue
		}

		r.Errorf("use of reserved %s name", member.noun()).Apply(
			report.Snippet(member.AST().Name()),
			report.Snippetf(name.AST(), "`%s` reserved here", member.Name()),
		)
	}
}

func validateOneof(oneof Oneof, r *report.Report) {
	if oneof.Members().Len() == 0 {
		r.Errorf("oneof must define at least one member").Apply(
			report.Snippet(oneof.AST()),
		)
	}
}

func validateExtensionRange(rr ReservedRange, r *report.Report) {
	if rr.Context().Syntax() != syntax.Proto3 {
		return
	}

	r.Errorf("%s in \"proto3\"", taxa.Extensions).Apply(
		report.Snippet(rr.AST()),
		report.PageBreak,
		report.Snippetf(rr.Context().AST().Syntax().Value(), "\"proto3\" specified here"),
		report.Helpf("extension numbers cannot be reserved in \"proto3\""),
	)
}

func validateExtend(extend Extend, r *report.Report) {
	if extend.Extensions().Len() == 0 {
		r.Errorf("%s must declare at least one %s", taxa.Extend, taxa.Extension).Apply(
			report.Snippet(extend.AST()),
		)
	}

	if extend.Context().Syntax() != syntax.Proto3 {
		return
	}

	builtins := extend.Context().builtins()
	if slicesx.Among(extend.Extendee(),
		builtins.FileOptions.Element(),
		builtins.MessageOptions.Element(),
		builtins.FieldOptions.Element(),
		builtins.RangeOptions.Element(),
		builtins.OneofOptions.Element(),
		builtins.EnumOptions.Element(),
		builtins.EnumValueOptions.Element(),
		builtins.ServiceOptions.Element(),
		builtins.MethodOptions.Element(),
	) {
		return
	}

	r.Error(errTypeConstraint{
		want: "built-in options message",
		got:  extend.Extendee(),
		decl: extend.AST().Type(),
	}).Apply(
		report.PageBreak,
		report.Snippetf(extend.Context().AST().Syntax().Value(), "\"proto3\" specified here"),
		report.Helpf("extendees in \"proto3\" files are restricted to an `google.protobuf.*Options` message types", taxa.Extend),
	)
}

func validateMessageSet(ty Type, r *report.Report) {
	f := ty.Context()
	builtins := ty.Context().builtins()

	if f.Syntax() == syntax.Proto3 {
		r.Errorf("%s are not supported", taxa.MessageSet).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.PageBreak,
			report.Snippetf(f.AST().Syntax().Value(), "\"proto3\" specified here"),
			report.Helpf("%ss cannot be defined in \"proto3\" only", taxa.MessageSet),
			report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
		)
		return
	}

	ok := true

	for member := range seq.Values(ty.Members()) {
		ok = false
		r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
			report.Snippet(member.AST()),
			report.PageBreak,
			report.Snippet(ty.AST().Stem()),
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Helpf("message set types may only declare extension ranges"),
		)
	}

	for oneof := range seq.Values(ty.Oneofs()) {
		ok = false
		r.Errorf("field declared in %s `%s`", taxa.MessageSet, ty.FullName()).Apply(
			report.Snippet(oneof.AST()),
			report.PageBreak,
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.Helpf("message set types may only declare extension ranges"),
		)
	}

	if ty.ExtensionRanges().Len() == 0 {
		ok = false
		r.Errorf("%s `%s` declares no %ss", taxa.MessageSet, ty.FullName(), taxa.Extensions).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
		)
	}

	if ok {
		r.Warnf("%ss are deprecated", taxa.MessageSet).Apply(
			report.Snippetf(ty.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(ty.AST().Stem()),
			report.Helpf("%ss are not implemented correctly in most Protobuf implementations", taxa.MessageSet),
		)
	}
}

func validateMessageSetExtension(extn Member, r *report.Report) {
	builtins := extn.Context().builtins()
	extendee := extn.Container()
	if extn.IsRepeated() {
		_, repeated := iterx.Find(extn.AST().Type().Prefixes(), func(ty ast.TypePrefixed) bool {
			return ty.Prefix() == keyword.Repeated
		})

		r.Errorf("repeated message set extension").Apply(
			report.Snippet(repeated.PrefixToken()),
			report.PageBreak,
			report.Snippetf(extendee.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(extendee.AST().Stem()),
			report.Helpf("message set extensions must be singular message fields"),
		)
	}

	if !extn.Element().IsMessage() {
		r.Errorf("non-message message set extension").Apply(
			report.Snippet(extn.AST().Type().RemovePrefixes()),
			report.PageBreak,
			report.Snippetf(extendee.Options().Field(builtins.MessageSet).KeyAST(), "declared as message set here"),
			report.Snippet(extendee.AST().Stem()),
			report.Helpf("message set extensions must be singular message fields"),
		)
	}
}

func validateExtensionDeclarations(ty Type, r *report.Report) {
	builtins := ty.Context().builtins()

	// First, walk through all of the extension ranges to get their associated
	// option objects.
	options := make(map[MessageValue][]ReservedRange)
	for r := range seq.Values(ty.ExtensionRanges()) {
		if r.Options().IsZero() {
			continue
		}
		mapsx.Append(options, r.Options(), r)
	}

	// Now, walk through each grouping of extensions and match up their
	// declarations.
	for options, ranges := range options {
		rangeSpan := func() source.Span {
			return source.JoinSeq(iterx.Map(slices.Values(ranges), func(r ReservedRange) source.Span {
				return r.AST().Span()
			}))
		}

		decls := options.Field(builtins.ExtnDecls)
		verification := options.Field(builtins.ExtnVerification)
		if v, ok := verification.AsInt(); ok && (v == 1) != decls.IsZero() {
			if decls.IsZero() {
				r.Errorf("extension range requires declarations, but does not define any").Apply(
					report.Snippetf(verification.ValueAST(), "required by this option"),
					report.Snippet(rangeSpan()),
				)
			} else {
				r.Errorf("unverified extension range defines declarations").Apply(
					report.Snippetf(decls.OptionSpan(), "defined here"),
					report.Snippetf(verification.ValueAST(), "required by this option"),
				)
			}
		}

		if decls.IsZero() {
			continue
		}

		if len(ranges) > 1 {
			// An extension range with declarations and multiple ranges
			// is not allowed.
			r.Errorf("multi-range `extensions` with extension declarations").Apply(
				report.Snippetf(decls.KeyAST(), "declaration defined here"),
				report.Snippetf(rangeSpan(), "multiple ranges declared here"),
				report.Helpf("this is rejected by protoc due to a quirk in its internal representation of extension ranges"),
			)
		}

		var haveMissingField bool
		numbers := make(map[int32]struct{})
		for elem := range seq.Values(decls.Elements()) {
			decl := elem.AsMessage()

			number := decl.Field(builtins.ExtnDeclNumber)
			if n, ok := number.AsInt(); ok {
				// Find the range that contains n.
				var found bool
				for _, r := range ranges {
					start, end := r.Range()
					if int64(start) <= n && n <= int64(end) {
						found = true
						numbers[int32(n)] = struct{}{}
						break
					}
				}

				if !found {
					r.Errorf("out-of-range `%s` in extension declaration", number.Field().Name()).Apply(
						report.Snippet(number.ValueAST()),
						report.Snippetf(rangeSpan(), "%v must be among one of these ranges", n),
					)
				}
			} else {
				r.Errorf("extension declaration must specify `%s`", builtins.ExtnDeclNumber.Name()).Apply(
					report.Snippet(elem.AST()),
				)
				haveMissingField = true
			}

			validatePath := func(v Value, want any) bool {
				// First, check this is a valid name in the first place.
				s, _ := v.AsString()
				name := FullName(s)
				for component := range name.Components() {
					if !asciiIdent.MatchString(component) {
						d := r.Errorf("expected %s in `%s.%s`", want,
							v.Field().Container().Name(), v.Field().Name(),
						).Apply(
							report.Snippet(v.ValueAST()),
						)
						if strings.ContainsFunc(component, unicode.IsSpace) {
							d.Apply(report.Helpf("the name may not contain whitespace"))
						}
						return false
					}
				}

				if !name.Absolute() {
					d := r.Errorf("relative name in `%s.%s`",
						v.Field().Container().Name(), v.Field().Name(),
					).Apply(
						report.Snippet(v.ValueAST()),
					)

					if lit := v.ValueAST().AsLiteral(); !lit.IsZero() {
						str := lit.AsString()
						start := lit.Span().Start
						offset := str.RawContent().Start - start
						d.Apply(report.SuggestEdits(v.ValueAST(), "add a leading `.`", report.Edit{
							Start: offset, End: offset,
							Replace: ".",
						}))
					}
				}

				return true
			}

			// NOTE: name deduplication needs to wait until global linking,
			// similar to extension number deduplication.
			name := decl.Field(builtins.ExtnDeclName)
			if !name.IsZero() {
				validatePath(name, "fully-qualified name")
			} else if !haveMissingField {
				r.Errorf("extension declaration must specify `%s`", builtins.ExtnDeclName.Name()).Apply(
					report.Snippet(elem.AST()),
				)
				haveMissingField = true
			}

			tyName := decl.Field(builtins.ExtnDeclType)
			if !tyName.IsZero() {
				v, _ := tyName.AsString()
				if predeclared.Lookup(v) == predeclared.Unknown {
					ok := validatePath(tyName, "predeclared type or fully-qualified name")
					if ok {
						// Check to see whether this is a legit type.
						sym := ty.Context().FindSymbol(FullName(v).ToRelative())
						if !sym.IsZero() && !sym.Kind().IsType() {
							r.Warnf("expected type, got %s `%s`", sym.noun(), sym.FullName()).Apply(
								report.Snippet(tyName.ValueAST()),
								report.PageBreak,
								report.Snippetf(sym.Definition(), "`%s` declared here", sym.FullName()),
								report.Helpf("`%s.%s` must name a (possibly unimported) type", tyName.Field().Container().Name(), tyName.Field().Name()),
							)
						}
					}
				}
			} else if !haveMissingField {
				r.Errorf("extension declaration must specify `%s`", builtins.ExtnDeclType.Name()).Apply(
					report.Snippet(elem.AST()),
				)
				haveMissingField = true
			}
		}

		// Generate warnings for each range that is missing at least one value.
	missingDecls:
		for _, rr := range ranges {
			start, end := rr.Range()

			// The complexity of this loop is only O(decls), so `1 to max` will
			// not need to loop two billion times.
			for i := start; i <= end; i++ {
				if !mapsx.Contains(numbers, i) {
					r.Warnf("missing declaration for extension number `%v`", i).Apply(
						report.Snippetf(rr.AST(), "required by this range"),
						report.Notef("this is likely a mistake, but it is not rejected by protoc"),
					)
					break missingDecls // Only diagnose the first problematic range.
				}
			}
		}
	}
}

func validateDeclaredExtension(m Member, r *report.Report) {
	builtins := m.Context().builtins()

	// First, figure out whether this is a declared extension.
	extendee := m.Container()
	var decl MessageValue
	var elem Element
declSearch:
	for r := range extendee.Ranges(m.Number()) {
		decls := r.AsReserved().Options().Field(builtins.ExtnDecls)
		for v := range seq.Values(decls.Elements()) {
			msg := v.AsMessage()
			number := msg.Field(builtins.ExtnDeclNumber)
			if n, ok := number.AsInt(); ok && n == int64(m.Number()) {
				elem = v
				decl = msg
				break declSearch
			}
		}
	}
	if decl.IsZero() {
		return // Not a declared extension.
	}

	reserved := decl.Field(builtins.ExtnDeclReserved)
	if v, _ := reserved.AsBool(); v {
		r.Errorf("use of reserved extension number").Apply(
			report.Snippet(m.AST().Value()),
			report.PageBreak,
			report.Snippetf(elem.AST(), "extension declared here"),
			report.Snippetf(reserved.ValueAST(), "... and reserved here"),
		)
	}

	name := decl.Field(builtins.ExtnDeclName)
	if v, ok := name.AsString(); ok && m.FullName() != FullName(v).ToRelative() {
		r.Errorf("unexpected %s name", taxa.Extension).Apply(
			report.Snippetf(m.AST().Name(), "expected `%s`", v),
			report.PageBreak,
			report.Snippetf(name.ValueAST(), "expected name declared here"),
		)
	}

	tyName := decl.Field(builtins.ExtnDeclType)
	repeated := decl.Field(builtins.ExtnDeclRepeated)
	wantRepeated, _ := repeated.AsBool()

	if v, ok := tyName.AsString(); ok {
		ty := PredeclaredType(predeclared.Lookup(v))
		var sym Symbol
		if ty.IsZero() {
			sym = m.Context().FindSymbol(FullName(v).ToRelative())
			ty = sym.AsType()
		}

		if m.Element() != ty || wantRepeated != m.IsRepeated() {
			want := any(sym)
			if sym.IsZero() {
				if !ty.IsZero() {
					want = ty
				} else {
					want = fmt.Sprintf("unknown type `%s`", FullName(v).ToRelative())
				}
			}

			d := r.Error(errTypeCheck{
				want: want, got: m.Element(),
				wantRepeated: wantRepeated,
				gotRepeated:  m.IsRepeated(),

				expr:       m.TypeAST(),
				annotation: tyName.ValueAST(),
			})

			if wantRepeated {
				d.Apply(report.Snippetf(repeated.OptionSpan(), "`repeated` required here"))
			}

			if !sym.IsZero() && ty.IsZero() {
				d.Apply(report.Notef("`%s` is not a type; this indicates a bug in the extension declaration", sym.FullName()))
			}
		}
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
				feature.Value().KeyAST(),
				"`%s` set here", feature.Field().Name(),
			),
			report.Helpf("`%s` can only be set on singular fields", feature.Field().Name()),
		)

	case m.Presence() == presence.Shared:
		r.Errorf("expected singular field, found oneof member").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(m.Oneof().AST(), "defined in this oneof"),
			report.Snippetf(
				feature.Value().KeyAST(),
				"`%s` set here", feature.Field().Name(),
			),
			report.Helpf("`%s` cannot be set on oneof members", feature.Field().Name()),
			report.Helpf("all oneof members have explicit presence"),
		)

	case m.IsExtension():
		r.Errorf("expected singular field, found extension").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(
				feature.Value().KeyAST(),
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
					feature.Value().ValueAST(),
					"implicit presence set here",
				),
				report.Helpf("all message-typed fields explicit presence"),
			)
		}
	case 3: // LEGACY_REQUIRED
		r.Warnf("required fields are deprecated").Apply(
			report.Snippet(feature.Value().ValueAST()),
			report.Helpf(
				"do not attempt to change this to `EXPLICIT` if the field is "+
					"already in-use; doing so is a wire protocol break"),
		)
	}
}

// validatePacked validates constraints on the packed option and feature.
func validatePacked(m Member, r *report.Report) {
	builtins := m.Context().builtins()

	validate := func(span source.Span) {
		switch {
		case m.IsSingular() || m.IsMap():
			r.Errorf("expected repeated field, found singular field").Apply(
				report.Snippet(m.TypeAST()),
				report.Snippetf(span, "packed encoding set here"),
				report.Helpf("packed encoding can only be set on repeated fields of integer, float, `bool`, or enum type"),
			)
		case !m.Element().IsPackable():
			r.Error(errTypeConstraint{
				want: "packable type",
				got:  m.Element(),
				decl: m.TypeAST(),
			}).Apply(
				report.Snippetf(span, "packed encoding set here"),
				report.Helpf("packed encoding can only be set on repeated fields of integer, float, `bool`, or enum type"),
			)
		}
	}

	option := m.Options().Field(builtins.Packed)
	if !option.IsZero() {
		if m.Context().Syntax().IsEdition() {
			packed, _ := option.AsBool()
			want := "PACKED"
			if !packed {
				want = "EXPANDED"
			}
			r.Error(errEditionTooNew{
				file:    m.Context(),
				removed: syntax.Edition2023,

				what:  option.Field().Name(),
				where: option.KeyAST(),
			}).Apply(option.suggestEdit(
				builtins.FeaturePacked.Name(), want,
				"replace with `%s`", builtins.FeaturePacked.Name(),
			))
		} else if v, _ := option.AsBool(); v {
			// Don't validate [packed = false], protoc accepts that.
			validate(option.ValueAST().Span())
		}
	}

	feature := m.FeatureSet().Lookup(builtins.FeaturePacked)
	if feature.IsExplicit() {
		validate(feature.Value().KeyAST().Span())
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
				report.Snippetf(lazy.KeyAST(), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` can only be set on message-typed fields", lazy.Field().Name()),
			)
		}

		if m.IsGroup() {
			r.SoftErrorf(set, "expected length-prefixed field").Apply(
				report.Snippet(m.AST()),
				report.Snippetf(m.AST().KeywordToken(), "groups are not length-prefixed"),
				report.Snippetf(lazy.KeyAST(), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` only makes sense for length-prefixed messages", lazy.Field().Name()),
			)
		}

		group := m.FeatureSet().Lookup(builtins.FeatureGroup)
		groupValue, _ := group.Value().AsInt()
		if groupValue == 2 { // FeatureSet.DELIMITED
			d := r.SoftErrorf(set, "expected length-prefixed field").Apply(
				report.Snippet(m.AST()),
				report.Snippetf(lazy.KeyAST(), "`%s` set here", lazy.Field().Name()),
				report.Helpf("`%s` only makes sense for length-prefixed messages", lazy.Field().Name()),
			)

			if group.IsInherited() {
				d.Apply(report.PageBreak)
			}
			d.Apply(report.Snippetf(group.Value().ValueAST(), "set to use delimited encoding here"))
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
			report.Snippetf(option.KeyAST(), "`%s` set here", option.Field().Name()),
			report.Helpf("`%s` is specifically for controlling the formatting of large integer types, "+
				"which lose precision when JavaScript converts them into 64-bit IEEE 754 floats", option.Field().Name()),
		)
	}
}

func validateCType(m Member, r *report.Report) {
	builtins := m.Context().builtins()
	f := m.Context()

	ctype := m.Options().Field(builtins.CType)
	if ctype.IsZero() {
		return
	}

	ctypeValue, _ := ctype.AsInt()

	var want string
	switch ctypeValue {
	case 0: // FieldOptions.STRING
		want = "STRING"
	case 1: // FieldOptions.CORD
		want = "CORD"
	case 2: // FieldOptions.STRING_PIECE
		want = "VIEW"
	}

	is2023 := f.Syntax() == syntax.Edition2023
	switch {
	case f.Syntax() > syntax.Edition2023:
		r.Error(errEditionTooNew{
			file:       f,
			deprecated: syntax.Edition2023,
			removed:    syntax.Edition2024,

			what:  ctype.Field().Name(),
			where: ctype.KeyAST(),
		}).Apply(ctype.suggestEdit(
			"features.(pb.cpp).string_type", want,
			"replace with `features.(pb.cpp).string_type`",
		))

	case !m.Element().Predeclared().IsString():
		d := r.SoftError(is2023, errTypeConstraint{
			want: "`string` or `bytes`",
			got:  m.Element(),
			decl: m.TypeAST(),
		}).Apply(
			report.Snippetf(ctype.KeyAST(), "`%s` set here", ctype.Field().Name()),
		)

		if !is2023 {
			d.Apply(report.Helpf("this becomes a hard error in %s", prettyEdition(syntax.Edition2023)))
		}

	case m.IsExtension() && ctypeValue == 1: // google.protobuf.FieldOptions.CORD
		d := r.SoftErrorf(is2023, "cannot use `CORD` on an extension field").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(ctype.ValueAST(), "`CORD` set here"),
		)

		if !is2023 {
			d.Apply(report.Helpf("this becomes a hard error in %s", prettyEdition(syntax.Edition2023)))
		}

	case is2023:
		r.Warn(errEditionTooNew{
			file:       f,
			deprecated: syntax.Edition2023,
			removed:    syntax.Edition2024,

			what:  ctype.Field().Name(),
			where: ctype.KeyAST(),
		}).Apply(ctype.suggestEdit(
			"features.(pb.cpp).string_type", want,
			"replace with `features.(pb.cpp).string_type`",
		))
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
			feature.Value().KeyAST(),
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
			feature.Value().KeyAST(),
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

func validateDefault(m Member, r *report.Report) {
	option := m.PseudoOptions().Default
	if option.IsZero() {
		return
	}

	if file := m.Context(); file.Syntax() == syntax.Proto3 {
		r.Errorf("custom default in \"proto3\"").Apply(
			report.Snippet(option.OptionSpan()),
			report.PageBreak,
			report.Snippetf(file.AST().Syntax().Value(), "\"proto3\" specified here"),
			report.Helpf("custom defaults cannot be defined in \"proto3\" only"),
		)
	}

	if m.IsRepeated() || m.Element().IsMessage() {
		r.Error(errTypeConstraint{
			want: "singular scalar- or enum-typed field",
			got:  m.Element(),
			decl: m.TypeAST(),
		}).Apply(
			report.Snippetf(option.KeyAST(), "custom default specified here"),
			report.Helpf("custom defaults are only for non-repeated fields that have a non-message type"),
		)
	}

	if m.IsUnicode() {
		if s, _ := option.AsString(); !utf8.ValidString(s) {
			r.Warn(&errNotUTF8{value: option.Elements().At(0)}).Apply(
				report.Helpf("protoc erroneously accepts non-UTF-8 defaults for UTF-8 fields; for all other options, UTF-8 validation failure causes protoc to crash"),
			)
		}
	}

	// Warn if the zero value is used, because it's redundant.
	if option.IsZeroValue() {
		r.Warnf("redundant custom default").Apply(
			report.Snippetf(option.ValueAST(), "this is the zero value for `%s`", m.Element().FullName()),
			report.Helpf("fields without a custom default will default to the zero value, making this option redundant"),
		)
	}
}

// validateUTF8Values validates that strings in a value are actually UTF-8.
func validateUTF8Values(v Value, r *report.Report) {
	for elem := range seq.Values(v.Elements()) {
		if v.Field().IsUnicode() {
			if s, _ := elem.AsString(); !utf8.ValidString(s) {
				r.Error(&errNotUTF8{value: elem})
			}
		}
	}
}

// errNotUTF8 diagnoses a non-UTF8 value.
type errNotUTF8 struct {
	value Element
}

func (e *errNotUTF8) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("non-UTF-8 string literal"))

	if lit := e.value.AST().AsLiteral().AsString(); !lit.IsZero() {
		// Figure out the byte offset and the invalid byte. Because this will
		// necessarily have come from a \xNN escape, we should look for it.
		text := lit.Text()
		offset := 0
		var invalid byte
		for text != "" {
			r, n := utf8.DecodeRuneInString(text[offset:])
			if r == utf8.RuneError {
				invalid = text[offset]
				break
			}

			offset += n
		}

		// Now, find the invalid escape...
		var esc token.Escape
		for escape := range seq.Values(lit.Escapes()) {
			if escape.Byte == invalid {
				esc = escape
				break
			}
		}

		d.Apply(report.Snippetf(esc.Span, "non-UTF-8 byte"))
	} else {
		// String came from non-literal.
		d.Apply(report.Snippet(e.value.AST()))
	}

	d.Apply(
		report.Snippetf(e.value.Field().AST(), "this field requires a UTF-8 string"),
	)

	// Figure out where the relevant feature was set.
	builtins := e.value.Context().builtins()
	feature := e.value.Field().FeatureSet().Lookup(builtins.FeatureUTF8)
	if !feature.IsDefault() {
		if feature.IsInherited() {
			d.Apply(report.PageBreak)
		}
		d.Apply(report.Snippetf(feature.Value().ValueAST(), "UTF-8 required here"))
	} else {
		d.Apply(
			report.PageBreak,
			report.Snippetf(e.value.Context().AST().Syntax().Value(), "UTF-8 required here"),
		)
	}
}
