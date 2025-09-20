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
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

func buildAllFeatureInfo(f File, r *report.Report) {
	for ty := range seq.Values(f.AllTypes()) {
		for field := range seq.Values(ty.Members()) {
			buildFeatureInfo(field, r)
		}
	}
	for extn := range seq.Values(f.AllExtensions()) {
		buildFeatureInfo(extn, r)
	}
}

// buildFeatureInfo builds feature information for a feature field.
//
// A feature field is any field which sets either of the editions_defaults or
// feature_support fields.
func buildFeatureInfo(field Member, r *report.Report) {
	dp := field.Context().imports.DescriptorProto().Context()
	ed := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionDefaults})
	es := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionSupport})

	defaults := field.Options().Field(ed)
	support := field.Options().Field(es).AsMessage()

	if defaults.IsZero() && support.IsZero() {
		return
	}

	edk := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionDefaultKey})
	edv := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionDefaultValue})

	esi := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionSupportIntroduced})
	esd := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionSupportDeprecated})
	esw := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionSupportWarning})
	esr := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.editionSupportRemoved})

	mistake := report.Notef("protoc accepts this, but it is very likely a mistake")

	info := new(rawFeatureInfo)
	if defaults.IsZero() {
		r.Warnf("expected feature field to set `edition_defaults`").Apply(
			report.Snippet(field.AST()),
		)
	} else {
		for def := range seq.Values(defaults.Elements()) {
			def := def.AsMessage()
			value := def.Field(edk)
			key, _ := value.AsInt()
			var edition syntax.Syntax
			if value.IsZero() {
				r.Warnf("missing edition key in `edition_defaults`").Apply(
					report.Snippet(defaults.AST()),
					mistake,
				)
			} else {
				edition = syntax.FromEnum(int32(key))
				if edition == syntax.Unknown && key != syntax.EditionLegacyNumber {
					enum := value.Field().Element().MemberByNumber(int32(key))
					name := any(enum.Name())
					if enum.IsZero() {
						name = key
					}

					r.Warnf("unexpected value in `EditionDefault.edition`", name).Apply(
						report.Snippet(value.AST()),
						mistake,
					)
				}
			}

			value = def.Field(edv)

			// We can't use eval() here because we would need to run the whole
			// parser on the contents of the quoted string.
			var bits rawValueBits
			if value.IsZero() {
				r.Warnf("missing default value in `edition_defaults`").Apply(
					report.Snippet(defaults.AST()), mistake,
				)
			} else {
				text, _ := value.AsString()
				switch {
				case field.Element().IsEnum():
					ev := field.Element().MemberByName(text)
					if ev.IsZero() {
						r.Warnf("expected quoted enum value in `EditionDefault.value`").Apply(
							report.Snippet(value.AST()),
							report.Snippetf(field.TypeAST(), "expected due to this"),
							report.Helpf("`value` must be the name of a value in `%s`", field.Element().FullName()),
							mistake,
						)
					}
					bits = rawValueBits(ev.Number())
				case field.Element().Predeclared() == predeclared.Bool:
					switch text {
					case "false":
						bits = 0
					case "true":
						bits = 1
					default:
						r.Warnf("expected quoted bool in `EditionDefault.value`").Apply(
							report.Snippet(value.AST()),
							report.Snippetf(field.TypeAST(), "expected due to this"),
							report.Helpf("`value` must one of \"true\" or \"false\""),
							mistake,
						)
					}
				default:
					r.Warnf("expected `bool` or enum typed field for feature").Apply(
						report.Snippet(value.Field().TypeAST()),
						mistake,
					)
					continue
				}
			}

			if edition == syntax.Unknown && key != syntax.EditionLegacyNumber {
				// Discard invalid editions.
				continue
			}

			// Cook up a value corresponding to the thing we just evaluated.
			raw := field.Context().arenas.values.NewCompressed(rawValue{
				expr:       value.AST(),
				optionPath: def.AsValue().OptionPath(),
				field:      ref[rawMember]{ptr: field.Context().arenas.members.Compress(field.raw)},
				bits:       bits,
			})

			// Push this information onto the edition defaults list.
			info.defaults = append(info.defaults, featureDefault{
				edition: edition,
				value:   raw,
			})
		}
	}

	// Sort the defaults by their editions.
	slices.SortStableFunc(info.defaults, func(a, b featureDefault) int {
		return cmp.Compare(a.edition, b.edition)
	})

	if len(info.defaults) > 0 && !slicesx.Among(info.defaults[0].edition, syntax.Unknown, syntax.Proto2) {
		r.Warnf("`editions_default` does not cover all editions").Apply(
			report.Snippet(defaults.AST()),
			report.Helpf("`editions_default` must specify a default for `EDITION_LEGACY` or `EDITION_PROTO2` to cover all editions"),
			mistake,
		)
	}

	if support.IsZero() {
		r.Warnf("expected feature field to set `feature_support`").Apply(
			report.Snippet(field.AST()),
		)
	} else {
		value := support.Field(esi)
		n, _ := value.AsInt()
		info.introduced = syntax.FromEnum(int32(n))
		if value.IsZero() {
			r.Warnf("expected `FeatureSupport.introduced` to be set").Apply(
				report.Snippet(support.AsValue().AST()),
				mistake,
			)
		}
		if info.introduced == syntax.Unknown {
			r.Warnf("unexpected value `%s` in `FeatureSupport.introduced`", value.AST().Span().Text()).Apply(
				report.Snippet(value.AST()),
				mistake,
			)
		}

		value = support.Field(esd)
		n, _ = value.AsInt()
		info.deprecated = syntax.FromEnum(int32(n))
		if !value.IsZero() && info.deprecated == syntax.Unknown {
			r.Warnf("unexpected value `%s` in `FeatureSupport.deprecated`", value.AST().Span().Text()).Apply(
				report.Snippet(value.AST()),
				mistake,
			)
		}

		value = support.Field(esr)
		n, _ = value.AsInt()
		info.removed = syntax.FromEnum(int32(n))
		if !value.IsZero() && info.removed == syntax.Unknown {
			r.Warnf("unexpected value `%s` in `FeatureSupport.removed`", value.AST().Span().Text()).Apply(
				report.Snippet(value.AST()),
				mistake,
			)
		}

		value = support.Field(esw)
		info.deprecationWarning, _ = value.AsString()
	}

	field.raw.featureInfo = info
}

func validateAllFeatures(f File, r *report.Report) {
	dp := f.Context().imports.DescriptorProto().Context()
	fileFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.fileFeatures})
	messageFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.messageFeatures})
	fieldFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.fieldFeatures})
	oneofFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.oneofFeatures})
	enumFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.enumFeatures})
	enumValueFeatures := wrapMember(dp, ref[rawMember]{ptr: dp.builtins.enumValueFeatures})

	validateFeatures(f.Options().Field(fileFeatures).AsMessage(), r)
	f.Context().features = f.Context().arenas.features.NewCompressed(rawFeatureSet{
		options: f.Context().options,
	})

	for ty := range seq.Values(f.AllTypes()) {
		if !ty.MapField().IsZero() {
			// Map entries never have features.
			continue
		}

		parent := f.Context().File().Context().features
		if !ty.Parent().IsZero() {
			parent = ty.Parent().raw.features
		}

		if ty.IsEnum() {
			validateFeatures(ty.Options().Field(enumFeatures).AsMessage(), r)
		} else {
			validateFeatures(ty.Options().Field(messageFeatures).AsMessage(), r)
		}

		ty.raw.features = f.Context().arenas.features.NewCompressed(rawFeatureSet{
			options: f.Context().options,
			parent:  parent,
		})

		for field := range seq.Values(ty.Members()) {
			if field.IsEnumValue() {
				validateFeatures(field.Options().Field(enumValueFeatures).AsMessage(), r)
			} else {
				validateFeatures(field.Options().Field(fieldFeatures).AsMessage(), r)
			}

			field.raw.features = f.Context().arenas.features.NewCompressed(rawFeatureSet{
				options: f.Context().options,
				parent:  ty.raw.features,
			})
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			validateFeatures(oneof.Options().Field(oneofFeatures).AsMessage(), r)
			oneof.raw.features = f.Context().arenas.features.NewCompressed(rawFeatureSet{
				options: f.Context().options,
				parent:  ty.raw.features,
			})
		}
	}
	for field := range seq.Values(f.AllExtensions()) {
		parent := f.Context().File().Context().features
		if !field.Parent().IsZero() {
			parent = field.Parent().raw.features
		}

		validateFeatures(field.Options().Field(fieldFeatures).AsMessage(), r)
		field.raw.features = f.Context().arenas.features.NewCompressed(rawFeatureSet{
			options: f.Context().options,
			parent:  parent,
		})
	}
}

// validateFeatures validates that the given features are compatible with the
// current edition.
func validateFeatures(features MessageValue, r *report.Report) {
	if features.IsZero() {
		return
	}

	defer r.AnnotateICE(report.Snippetf(
		features.AsValue().AST(),
		"while validating this features message",
	))

	edition := features.Context().File().Syntax()
	for feature := range features.Fields() {
		if msg := feature.AsMessage(); !msg.IsZero() {
			validateFeatures(features, r)
			continue
		}

		info := feature.Field().FeatureInfo()
		if info.IsZero() {
			r.Warnf("non-feature field set within `features`").Apply(
				report.Snippet(feature.AST()),
				report.Helpf("a feature field is a field which sets the `edition_defaults` and `feature_support` options"),
			)
			continue
		}

		key := func() report.Spanner {
			key := feature.FieldAST().Key()
			if key.IsZero() {
				return feature.OptionPath()
			}
			return key
		}

		if intro := info.Introduced(); edition < intro {
			d := r.Errorf("`%s` is not supported in %s", feature.Field().Name(), edition.PrettyString()).Apply(
				report.Snippet(key()),
			)
			if intro != syntax.Unknown {
				d.Apply(report.Helpf(
					"`%s` requires at least %s",
					feature.Field().Name(), intro.PrettyString(),
				))
			}
			continue
		}

		if removed := info.Removed(); removed != syntax.Unknown && removed <= edition {
			d := r.Errorf("`%s` is not supported in %s", feature.Field().Name(), edition.PrettyString()).Apply(
				report.Snippet(key()),
				report.Helpf(
					"`%s` ended support in %s",
					feature.Field().Name(), removed.PrettyString(),
				),
			)

			if deprecated := info.Deprecated(); deprecated != syntax.Unknown {
				d.Apply(report.Helpf(
					"it has been deprecated since %s", deprecated.PrettyString(),
				))
			}
			continue
		}

		if deprecated := info.Deprecated(); deprecated != syntax.Unknown && deprecated <= edition {
			// Transform help text into something that is somewhat compatible
			// with our diagnostic style.
			helps := strings.Split(info.DeprecationWarning(), ". ") // Split sentences.
			for i, help := range helps {
				help = strings.TrimSpace(help)
				help = strings.TrimSuffix(help, ".")
				if help == "" {
					continue
				}

				// Lowercase the first rune.
				r, _ := stringsx.Rune(help, 0)
				sz := utf8.RuneLen(r)
				r = unicode.ToLower(r)
				helps[i] = string(r) + help[sz:]
			}
			helps = slices.DeleteFunc(helps, func(s string) bool { return s == "" })
			helps = append(helps, fmt.Sprintf("it has been deprecated since %s", deprecated.PrettyString()))

			d := r.Warnf("`%s` is deprecated", feature.Field().Name()).Apply(
				report.Snippet(key()),
			)
			for _, help := range helps {
				d.Apply(report.Helpf("%v", help))
			}
		}
	}
}

// validateCustomFeatureMessages validates that every extension to FeatureSet
// satisfies the following conditions:
//
// 1. Every field must be optional and either bool or enum typed.
// 2. Every field sets the edition_defaults and feature_support options.
// 3. It has no extensions.
// 4. edition_defaults.key is a sensible value.
//
// This is also run on all types called "google.protobuf.FeatureSet", in which
// case it validates the above except for (3).
//
// For some reason, protoc does not error out on any of this! So we're forced
// to emit warnings only.
// func validateCustomFeatureMessages(f File, r *report.Report) {

// }

// func validateCustomFeatureMessage(ty Type, extension Member, r *report.Report) {
// 	dp := ty.Context().imports.DescriptorProto()
// 	ed := wrapMember(dp.Context(), ref[rawMember]{ptr: dp.Context().builtins.editionDefaults})
// 	ek := wrapMember(dp.Context(), ref[rawMember]{ptr: dp.Context().builtins.editionDefaultKey})
// 	ev := wrapMember(dp.Context(), ref[rawMember]{ptr: dp.Context().builtins.editionDefaultValue})

// 	for field := range seq.Values(ty.Members()) {
// 		if field.Presence() != presence.Explicit {
// 			r.Warnf("features must have explicit presence").Apply(
// 				report.Snippet(field.AST().Type()),
// 				report.Snippetf(extension.AST(), "`FeatureSet` extended here"),
// 				report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 			)
// 		}
// 		if !field.Element().IsEnum() && field.Element().Predeclared() != predeclared.Bool {
// 			r.Warnf("features must be `bool` or enum typed").Apply(
// 				report.Snippet(field.AST().Type().RemovePrefixes()),
// 				report.Snippetf(extension.AST(), "`FeatureSet` extended here"),
// 				report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 			)

// 			continue
// 		}

// 		options := field.Options()
// 		defaults := options.Field(ed)
// 		if defaults.IsZero() {
// 			r.Warnf("features must set the `edition_defaults` option").Apply(
// 				report.Snippet(field.AST()),
// 				report.Snippetf(extension.AST(), "`FeatureSet` extended here"),
// 				report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 			)
// 		} else {
// 			var found bool
// 			for def := range seq.Values(defaults.Elements()) {
// 				keyValue := def.AsMessage().Field(ek)
// 				if keyValue.IsZero() {
// 					r.Warnf("missing edition key in `edition_defaults`").Apply(
// 						report.Snippet(defaults.AST()),
// 						report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 					)
// 				} else {
// 					key, _ := keyValue.AsInt()
// 					switch key {
// 					case syntax.EditionLegacyNumber, syntax.EditionProto2Number:
// 						found = true
// 					case syntax.EditionProto3Number, syntax.Edition2023Number, syntax.Edition2024Number:

// 					default:
// 						value := keyValue.Field().Element().MemberByNumber(int32(key))
// 						name := any(value.Name())
// 						if value.IsZero() {
// 							name = key
// 						}

// 						r.Warnf("`%s` should not be used in `edition_defaults`", name).Apply(
// 							report.Snippet(keyValue.AST()),
// 							report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 						)
// 					}
// 				}

// 				value := def.AsMessage().Field(ev)
// 				if value.IsZero() {
// 					r.Warnf("missing default value in `edition_defaults`").Apply(
// 						report.Snippet(defaults.AST()),
// 						report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 					)
// 				} else {
// 					// TODO: use default value type-checking here.
// 				}
// 			}

// 			if !found {
// 				r.Warnf("`edition_defaults` should specify a default for all editions").Apply(
// 					report.Snippet(defaults.AST()),
// 					report.Helpf("including a default for `EDITION_LEGACY` or `EDITION_PROTO2` "+
// 						"satisfies this requirement"),
// 					report.Notef("this is an error, but it is accepted by protoc by mistake"),
// 				)
// 			}
// 		}

// 		support = options.Field(es)
// 	}
// }
