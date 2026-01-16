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
	"github.com/bufbuild/protocompile/experimental/internal/erredition"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/report/tags"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
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
	validateNamingStyle(f, r)

	for ty := range seq.Values(f.AllTypes()) {
		validateReservedNames(ty, r)
		validateVisibility(ty, r)
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

	javaMultipleFiles := f.Options().Field(builtins.JavaMultipleFiles)
	if !javaMultipleFiles.IsZero() && f.Syntax() >= syntax.Edition2024 {
		want := "YES"
		if b, _ := javaMultipleFiles.AsBool(); !b {
			want = "NO"
		}

		r.Error(erredition.TooNew{
			Current:       f.Syntax(),
			Decl:          f.AST().Syntax(),
			Deprecated:    syntax.Edition2023,
			Removed:       syntax.Edition2024,
			RemovedReason: "`java_multiple_files` has been replaced with `features.(pb.java).nest_in_file_class`",
			What:          javaMultipleFiles.Field().Name(),
			Where:         javaMultipleFiles.KeyAST(),
		}).Apply(javaMultipleFiles.suggestEdit("features.(pb.java).nest_in_file_class", want, "replace with `features.(pb.java).nest_in_file_class`"))
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
			r.Error(erredition.TooNew{
				Current: m.Context().Syntax(),
				Decl:    m.Context().AST().Syntax(),
				Removed: syntax.Edition2023,

				What:  option.Field().Name(),
				Where: option.KeyAST(),
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
		r.Error(erredition.TooNew{
			Current:    m.Context().Syntax(),
			Decl:       m.Context().AST().Syntax(),
			Deprecated: syntax.Edition2023,
			Removed:    syntax.Edition2024,

			What:  ctype.Field().Name(),
			Where: ctype.KeyAST(),
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
			d.Apply(report.Helpf("this becomes a hard error in %s", syntax.Edition2023.Name()))
		}

	case m.IsExtension() && ctypeValue == 1: // google.protobuf.FieldOptions.CORD
		d := r.SoftErrorf(is2023, "cannot use `CORD` on an extension field").Apply(
			report.Snippet(m.AST()),
			report.Snippetf(ctype.ValueAST(), "`CORD` set here"),
		)

		if !is2023 {
			d.Apply(report.Helpf("this becomes a hard error in %s", syntax.Edition2023.Name()))
		}

	case is2023:
		r.Warn(erredition.TooNew{
			Current:    m.Context().Syntax(),
			Decl:       m.Context().AST().Syntax(),
			Deprecated: syntax.Edition2023,
			Removed:    syntax.Edition2024,

			What:  ctype.Field().Name(),
			Where: ctype.KeyAST(),
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

func validateVisibility(ty Type, r *report.Report) {
	key := ty.Context().builtins().FeatureVisibility
	if key.IsZero() {
		return
	}
	feature := ty.FeatureSet().Lookup(key)
	value, _ := feature.Value().AsInt()
	strict := value == 4 // STRICT
	var impliedExport bool
	switch value {
	case 0, 1: // DEFAULT_SYMBOL_VISIBILITY_UNKNOWN, EXPORT_ALL
		impliedExport = true
	case 2: // EXPORT_TOP_LEVEL
		impliedExport = ty.Parent().IsZero()
	case 3, 4: // LOCAL_ALL, STRICT
		impliedExport = false
	}

	var why source.Span
	if feature.IsDefault() {
		why = ty.Context().AST().Syntax().Value().Span()
	} else {
		why = feature.Value().ValueAST().Span()
	}

	vis := id.Wrap(ty.AST().Context().Stream(), ty.Raw().visibility)
	export := vis.Keyword() == keyword.Export
	if !ty.Raw().visibility.IsZero() && export == impliedExport {
		r.Warnf("redundant visibility modifier").Apply(
			report.Snippetf(vis, "specified here"),
			report.PageBreak,
			report.Snippetf(why, "this implies it"),
		)
	}

	if !strict || !export { // STRICT
		return
	}

	// STRICT requires that we check two things:
	//
	// 1. Nested types are not explicitly exported.
	// 2. Unless they are nested within a message that reserves all of its
	//    field numbers.
	parent := ty.Parent()
	if ty.Parent().IsZero() {
		return
	}

	start, end := parent.AbsoluteRange()

	// Find any gaps in the reserved ranges.
	gap := start
	ranges := slices.Collect(seq.Values(parent.ReservedRanges()))
	if len(ranges) > 0 {
		slices.SortFunc(ranges, cmpx.Join(
			cmpx.Key(func(r ReservedRange) int32 {
				start, _ := r.Range()
				return start
			}),
			cmpx.Key(func(r ReservedRange) int32 {
				start, end := r.Range()
				return end - start
			}),
		))

		// Skip all ranges whose end is less than start.
		for len(ranges) > 0 {
			_, end := ranges[0].Range()
			if end >= start {
				break
			}
			ranges = ranges[1:]
		}

		for _, rr := range ranges {
			a, b := rr.Range()
			if gap < a {
				// We're done, gap is not reserved.
				break
			}
			gap = b + 1
		}
	}

	if end <= gap {
		// If there are multiple reserved ranges, protoc rejects this, because it
		// doesn't do the same sophisticated interval sorting we do.
		switch {
		case parent.ReservedRanges().Len() != 1:
			d := r.Errorf("expected exactly one reserved range").Apply(
				report.Snippetf(vis, "nested type exported here"),
				report.Snippetf(parent.AST(), "... within this type"),
			)
			ranges := parent.ReservedRanges()
			if ranges.Len() > 0 {
				d.Apply(
					report.Snippetf(ranges.At(0).AST(), "one here"),
					report.Snippetf(ranges.At(1).AST(), "another here"),
				)
			}
			//nolint:dupword
			d.Apply(
				report.PageBreak,
				report.Snippetf(why, "`STRICT` specified here"),
				report.Helpf("in strict mode, nesting an exported type within another type "+
					"requires that that type declare `reserved 1 to max;`, even if all of its field "+
					"numbers are `reserved`"),
				report.Helpf("protoc erroneously rejects this, despite being equivalent"),
			)
		case ty.IsMessage():
			r.Errorf("nested message type marked as exported").Apply(
				report.Snippetf(vis, "nested type exported here"),
				report.Snippetf(parent.AST(), "... within this type"),
				report.PageBreak,
				report.Snippetf(why, "`STRICT` specified here"),
				report.Helpf("in strict mode, nested message types cannot be marked as "+
					"exported, even if all the field numbers of its parent are reserved"),
			)
		}

		return
	}

	// If this is true, the protoc check is bugged and we emit a warning...
	bugged := parent.ReservedRanges().Len() == 1
	//nolint:dupword
	d := r.SoftErrorf(!bugged, "%s `%s` does not reserve all field numbers", parent.noun(), parent.FullName()).Apply(
		report.Snippetf(vis, "nested type exported here"),
		report.Snippetf(parent.AST(), "... within this type"),
		report.PageBreak,
		report.Snippetf(why, "`STRICT` specified here"),
		report.Helpf("in strict mode, nesting an exported type within another type "+
			`requires that that type reserve every field number (the "C++ namespace exception"), `+
			"but this type does not reserve the field number %d", gap),
	)
	if bugged {
		d.Apply(report.Helpf("protoc erroneously accepts this code due to a bug: it only " +
			"checks that there is exactly one reserved range"))
	}
}

func validateNamingStyle(f *File, r *report.Report) {
	key := f.builtins().FeatureNamingStyle
	if key.IsZero() {
		return // Feature doesn't exist (pre-2024)
	}

	// Helper to check if STYLE2024 is enabled at a given scope.
	isStyle2024 := func(featureSet FeatureSet) bool {
		feature := featureSet.Lookup(key)
		value, _ := feature.Value().AsInt()
		return value == 1 // STYLE2024
	}

	// Validate package name (file-level scope).
	if isStyle2024(f.FeatureSet()) {
		pkg := f.Package()
		if pkg != "" && !isValidPackageName(string(pkg)) {
			r.Errorf("package name should be lower_snake_case").Apply(
				report.Snippetf(f.AST().Package().Path(), "this name violates STYLE2024"),
				report.Helpf("STYLE2024 requires package names to be lower_snake_case or dot.delimited.lower_snake_case"),
			)
		}
	}

	// Validate all services in the file.
	for svc := range seq.Values(f.Services()) {
		name := svc.Name()
		// PascalCase required for services.
		if isStyle2024(svc.FeatureSet()) && !isPascalCase(name) {
			r.Errorf("service name should be PascalCase").Apply(
				report.Snippetf(svc.AST().Name(), "this name violates STYLE2024"),
				report.Helpf("STYLE2024 requires service names to be PascalCase (e.g., MyService)"),
			)
		}

		// Validate RPC method names.
		for method := range seq.Values(svc.Methods()) {
			if method.IsZero() || !isStyle2024(method.FeatureSet()) {
				continue
			}
			methodName := method.Name()
			if !isPascalCase(methodName) {
				r.Errorf("RPC method name should be PascalCase").Apply(
					report.Snippetf(method.AST().Name(), "this name violates STYLE2024"),
					report.Helpf("STYLE2024 requires RPC method names to be PascalCase (e.g., GetMessage)"),
				)
			}
		}
	}

	// Validate naming conventions based on type.
	for ty := range seq.Values(f.AllTypes()) {
		name := ty.Name()
		switch {
		case ty.IsMessage():
			// PascalCase required for messages.
			if isStyle2024(ty.FeatureSet()) && !isPascalCase(name) {
				r.Errorf("%s name should be PascalCase", ty.noun()).Apply(
					report.Snippetf(ty.AST().Name(), "this name violates STYLE2024"),
					report.Helpf("STYLE2024 requires message names to be PascalCase (e.g., MyMessage)"),
				)
			}

			// Validate field names (check at field level).
			for field := range seq.Values(ty.Members()) {
				if field.IsZero() || field.IsGroup() || !isStyle2024(field.FeatureSet()) {
					continue
				}
				fieldName := field.Name()
				if !isSnakeCase(fieldName) {
					r.Errorf("field name should be snake_case").Apply(
						report.Snippetf(field.AST().Name(), "this name violates STYLE2024"),
						report.Helpf("STYLE2024 requires field names to be snake_case (e.g., my_field)"),
					)
				}
			}

			// Validate oneof names (check at oneof level).
			for oneof := range seq.Values(ty.Oneofs()) {
				if oneof.IsZero() || !isStyle2024(oneof.FeatureSet()) {
					continue
				}
				oneofName := oneof.Name()
				if !isSnakeCase(oneofName) {
					r.Errorf("oneof name should be snake_case").Apply(
						report.Snippetf(oneof.AST().Name(), "this name violates STYLE2024"),
						report.Helpf("STYLE2024 requires oneof names to be snake_case (e.g., my_choice)"),
					)
				}
			}
		case ty.IsEnum():
			// PascalCase required for enums.
			if isStyle2024(ty.FeatureSet()) && !isPascalCase(name) {
				r.Errorf("%s name should be PascalCase", ty.noun()).Apply(
					report.Snippetf(ty.AST().Name(), "this name violates STYLE2024"),
					report.Helpf("STYLE2024 requires enum names to be PascalCase (e.g., MyEnum)"),
				)
			}

			// Validate enum value names (check at value level).
			for value := range seq.Values(ty.Members()) {
				if value.IsZero() || !isStyle2024(value.FeatureSet()) {
					continue
				}
				valueName := value.Name()
				if !isScreamingSnakeCase(valueName) {
					r.Errorf("enum value name should be SCREAMING_SNAKE_CASE").Apply(
						report.Snippetf(value.AST().Name(), "this name violates STYLE2024"),
						report.Helpf("STYLE2024 requires enum value names to be SCREAMING_SNAKE_CASE (e.g., MY_VALUE)"),
					)
				}
			}
		}
	}
}

// isPascalCase checks if a name is in PascalCase format.
func isPascalCase(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Must start with uppercase letter.
	if !unicode.IsUpper(rune(name[0])) {
		return false
	}
	// Should not contain underscores.
	if strings.Contains(name, "_") {
		return false
	}
	// All characters should be letters or digits.
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isSnakeCase checks if a name is in snake_case format.
func isSnakeCase(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Must start with lowercase letter.
	if !unicode.IsLower(rune(name[0])) {
		return false
	}
	// Should not have leading underscore.
	if strings.HasPrefix(name, "_") {
		return false
	}
	// Should only contain lowercase letters, digits, and underscores.
	for i, r := range name {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
		// Underscore must be followed by a letter (not a digit or another underscore).
		if r == '_' && i+1 < len(name) {
			next := rune(name[i+1])
			if !unicode.IsLower(next) {
				return false
			}
		}
	}
	// Should not have consecutive underscores or end with underscore.
	if strings.Contains(name, "__") || strings.HasSuffix(name, "_") {
		return false
	}
	return true
}

// isScreamingSnakeCase checks if a name is in SCREAMING_SNAKE_CASE format.
func isScreamingSnakeCase(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Should only contain uppercase letters, digits, and underscores.
	for i, r := range name {
		if !unicode.IsUpper(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
		// Underscore must be followed by a letter (not a digit or another underscore).
		if r == '_' && i+1 < len(name) {
			next := rune(name[i+1])
			if !unicode.IsUpper(next) {
				return false
			}
		}
	}
	// Should not have consecutive underscores or start/end with underscore.
	if strings.Contains(name, "__") || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_") {
		return false
	}
	return true
}

// isValidPackageName checks if a package name is in lower_snake_case or dot.delimited.lower_snake_case format.
func isValidPackageName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Split on dots and validate each component.
	for part := range strings.SplitSeq(name, ".") {
		if len(part) == 0 {
			return false
		}
		// Each part should be lower_snake_case.
		if !isSnakeCase(part) {
			return false
		}
	}
	return true
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
