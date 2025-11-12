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
	"regexp"
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

var whitespacePattern = regexp.MustCompile(`[ \t\r\n]+`)

func buildAllFeatureInfo(file *File, r *report.Report) {
	for m := range file.AllMembers() {
		if !m.IsEnumValue() {
			buildFeatureInfo(m, r)
		}
	}
}

// buildFeatureInfo builds feature information for a feature field.
//
// A feature field is any field which sets either of the editions_defaults or
// feature_support fields.
func buildFeatureInfo(field Member, r *report.Report) {
	builtins := field.Context().builtins()

	defaults := field.Options().Field(builtins.EditionDefaults)
	support := field.Options().Field(builtins.EditionSupport).AsMessage()

	if defaults.IsZero() && support.IsZero() {
		return
	}

	mistake := report.Notef("this is likely a mistake, but it is not rejected by protoc")

	info := new(rawFeatureInfo)
	if defaults.IsZero() {
		r.Warnf("expected feature field to set `%s`", builtins.EditionDefaults.Name()).Apply(
			report.Snippet(field.AST().Options()), mistake,
		)
	} else {
		for def := range seq.Values(defaults.Elements()) {
			def := def.AsMessage()
			value := def.Field(builtins.EditionDefaultsKey)
			key, _ := value.AsInt()
			edition := syntax.Syntax(key)

			if value.IsZero() {
				r.Warnf("missing `%s.%s`",
					builtins.EditionDefaultsKey.Container().Name(),
					builtins.EditionDefaultsKey.Name(),
				).Apply(
					report.Snippet(def.AsValue().ValueAST()),
					mistake,
				)
			} else if !edition.IsConstraint() {
				r.Warnf("unexpected `%s` in `%s.%s`",
					syntax.EditionLegacy.DescriptorName(),
					builtins.EditionDefaultsKey.Container().Name(),
					builtins.EditionDefaultsKey.Name(),
				).Apply(
					report.Snippet(value.ValueAST()),
					mistake,
					report.Helpf("this should be a released edition or `%s`",
						syntax.EditionLegacy.DescriptorName()),
				)
			}

			value = def.Field(builtins.EditionDefaultsValue)

			// We can't use eval() here because we would need to run the whole
			// parser on the contents of the quoted string.
			var bits rawValueBits
			if value.IsZero() {
				r.Warnf("missing value for `%s.%s`",
					builtins.EditionDefaultsKey.Container().Name(),
					builtins.EditionDefaultsKey.Name(),
				).Apply(
					report.Snippet(def.AsValue().ValueAST()), mistake,
				)
			} else {
				text, _ := value.AsString()
				switch {
				case field.Element().IsEnum():
					ev := field.Element().MemberByName(text)
					if ev.IsZero() {
						r.Warnf("expected quoted enum value in `%s.%s`",
							builtins.EditionDefaultsKey.Container().Name(),
							builtins.EditionDefaultsKey.Name(),
						).Apply(
							report.Snippet(value.ValueAST()),
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
						r.Warnf("expected quoted bool in `%s.%s`",
							builtins.EditionDefaultsValue.Container().Name(),
							builtins.EditionDefaultsValue.Name(),
						).Apply(
							report.Snippet(value.ValueAST()),
							report.Snippetf(field.TypeAST(), "expected due to this"),
							report.Helpf("`value` must one of \"true\" or \"false\""),
							mistake,
						)
					}
				default:
					r.Warn(errTypeConstraint{
						want: "`bool` or enum type",
						got:  field.Element(),
						decl: field.TypeAST(),
					}).Apply(
						report.Snippetf(defaults.KeyAST(), "expected because this makes `%s` into a feature", field.Name()),
						report.Helpf("features should have `bool` or enum type"),
						mistake,
					)
					continue
				}
			}

			// Cook up a value corresponding to the thing we just evaluated.
			var copied rawValue
			if !value.IsZero() {
				copied = *value.Raw()
			}
			copied.field = field.toRef(field.Context())
			copied.bits = bits
			raw := field.Context().arenas.values.NewCompressed(copied)

			// Push this information onto the edition defaults list.
			info.defaults = append(info.defaults, featureDefault{
				edition: edition,
				value:   id.ID[Value](raw),
			})
		}
	}

	// Sort the defaults by their editions.
	slices.SortStableFunc(info.defaults, func(a, b featureDefault) int {
		return cmp.Compare(a.edition, b.edition)
	})

	if len(info.defaults) > 0 && !slicesx.Among(info.defaults[0].edition, syntax.EditionLegacy, syntax.Proto2) {
		r.Warnf("`%s` does not cover all editions", builtins.EditionDefaults.Name()).Apply(
			report.Snippet(defaults.ValueAST()),
			report.Helpf(
				"`%s` must specify a default for `%s` or `%s` to cover all editions",
				builtins.EditionDefaults.Name(),
				syntax.Proto2.DescriptorName(),
				syntax.EditionLegacy.DescriptorName(),
			),
			mistake,
		)
	}

	// Insert a default value so FeatureSet.Lookup always returns *something*.
	info.defaults = slices.Insert(info.defaults, 0, featureDefault{
		edition: syntax.Unknown,
		value: id.ID[Value](field.Context().arenas.values.NewCompressed(rawValue{
			field: field.toRef(field.Context()),
		})),
	})

	if support.IsZero() {
		r.Warnf("expected feature field to set `%s`", builtins.EditionSupport.Name()).Apply(
			report.Snippet(field.AST().Options()), mistake,
		)
	} else {
		value := support.Field(builtins.EditionSupportIntroduced)
		n, _ := value.AsInt()
		info.introduced = syntax.Syntax(n)
		if value.IsZero() {
			r.Warnf("expected `%s.%s` to be set",
				builtins.EditionSupportIntroduced.Container().Name(),
				builtins.EditionSupportIntroduced.Name(),
			).Apply(
				report.Snippet(support.AsValue().ValueAST()),
				mistake,
			)
		} else if info.introduced == syntax.Unknown {
			r.Warnf("unexpected `%s` in `%s`",
				info.introduced.DescriptorName(),
				builtins.EditionSupportIntroduced.Name(),
			).Apply(
				report.Snippet(value.ValueAST()),
				mistake,
			)
		}

		value = support.Field(builtins.EditionSupportDeprecated)
		n, _ = value.AsInt()
		info.deprecated = syntax.Syntax(n)
		if !value.IsZero() && info.deprecated == syntax.Unknown {
			r.Warnf("unexpected `%s` in `%s`",
				info.deprecated.DescriptorName(),
				builtins.EditionSupportDeprecated.Name(),
			).Apply(
				report.Snippet(value.ValueAST()),
				mistake,
			)
		}

		value = support.Field(builtins.EditionSupportRemoved)
		n, _ = value.AsInt()
		info.removed = syntax.Syntax(n)
		if !value.IsZero() && info.removed == syntax.Unknown {
			r.Warnf("unexpected `%s` in `%s`",
				info.removed.DescriptorName(),
				builtins.EditionSupportRemoved.Name(),
			).Apply(
				report.Snippet(value.ValueAST()),
				mistake,
			)
		}

		value = support.Field(builtins.EditionSupportWarning)
		info.deprecationWarning, _ = value.AsString()
	}

	field.Raw().featureInfo = info
}

func validateAllFeatures(file *File, r *report.Report) {
	builtins := file.builtins()

	features := file.Options().Field(builtins.FileFeatures)
	validateFeatures(features.AsMessage(), r)
	file.features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
		options: features.ID(),
	}))

	for ty := range seq.Values(file.AllTypes()) {
		if !ty.MapField().IsZero() {
			// Map entries never have features.
			continue
		}

		parent := file.features
		if !ty.Parent().IsZero() {
			parent = ty.Parent().Raw().features
		}

		option := builtins.MessageFeatures
		if ty.IsEnum() {
			option = builtins.EnumFeatures
		}

		features := ty.Options().Field(option)
		validateFeatures(features.AsMessage(), r)
		ty.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
			options: features.ID(),
			parent:  parent,
		}))

		for member := range seq.Values(ty.Members()) {
			option := builtins.FieldFeatures
			if member.IsEnumValue() {
				option = builtins.EnumFeatures
			}

			features := member.Options().Field(option)
			validateFeatures(features.AsMessage(), r)
			member.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  ty.Raw().features,
			}))
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			features := oneof.Options().Field(builtins.OneofFeatures)
			validateFeatures(features.AsMessage(), r)
			oneof.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  ty.Raw().features,
			}))
		}
		for extns := range seq.Values(ty.ExtensionRanges()) {
			features := extns.Options().Field(builtins.RangeFeatures)
			validateFeatures(features.AsMessage(), r)
			extns.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  ty.Raw().features,
			}))
		}
	}
	for field := range seq.Values(file.AllExtensions()) {
		parent := file.features
		if !field.Parent().IsZero() {
			parent = field.Parent().Raw().features
		}

		features := field.Options().Field(builtins.FieldFeatures)
		validateFeatures(features.AsMessage(), r)
		field.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
			options: features.ID(),
			parent:  parent,
		}))
	}
	for service := range seq.Values(file.Services()) {
		features := service.Options().Field(builtins.ServiceFeatures)
		validateFeatures(features.AsMessage(), r)
		service.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
			options: features.ID(),
			parent:  file.features,
		}))

		for method := range seq.Values(service.Methods()) {
			features := method.Options().Field(builtins.MethodFeatures)
			validateFeatures(features.AsMessage(), r)
			method.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  service.Raw().features,
			}))
		}
	}
}

// validateFeatures validates that the given features are compatible with the
// current edition.
func validateFeatures(features MessageValue, r *report.Report) {
	if features.IsZero() {
		return
	}

	defer r.AnnotateICE(report.Snippetf(
		features.AsValue().ValueAST(),
		"while validating this features message",
	))

	builtins := features.Context().builtins()
	edition := features.Context().Syntax()
	for feature := range features.Fields() {
		if msg := feature.AsMessage(); !msg.IsZero() {
			validateFeatures(msg, r)
			continue
		}

		info := feature.Field().FeatureInfo()
		if info.IsZero() {
			r.Warnf("non-feature field set within `%s`", features.AsValue().Field().Name()).Apply(
				report.Snippet(feature.ValueAST()),
				report.Helpf("a feature field is a field which sets the `%s` and `%s` options",
					builtins.EditionDefaults.Name(),
					builtins.EditionSupport.Name(),
				),
			)
			continue
		}

		// We check these in reverse order, because the user might have set
		// introduced == deprecated == removed, and protoc doesn't enforce
		// any relationship between these.
		switch {
		case info.IsRemoved(edition), info.IsDeprecated(edition):
			r.SoftError(info.IsRemoved(edition), errEditionTooNew{
				file:       features.Context(),
				removed:    info.Removed(),
				deprecated: info.Deprecated(),
				warning:    info.DeprecationWarning(),
				what:       feature.Field().Name(),
				where:      feature.KeyAST(),
			})

		case !info.IsIntroduced(edition):
			r.Error(errEditionTooOld{
				file:  features.Context(),
				intro: info.Introduced(),
				what:  feature.Field().Name(),
				where: feature.KeyAST(),
			})
		}
	}
}

func prettyEdition(s syntax.Syntax) string {
	if !s.IsValid() || !s.IsEdition() {
		return fmt.Sprintf("\"%s\"", s)
	}
	return fmt.Sprintf("Edition %s", s)
}

type errEditionTooOld struct {
	file  *File
	intro syntax.Syntax

	what  any
	where source.Spanner
}

func (e errEditionTooOld) Diagnose(d *report.Diagnostic) {
	kind := "syntax"
	if e.file.Syntax().IsEdition() {
		kind = "edition"
	}

	d.Apply(
		report.Message("`%s` is not supported in %s", e.what, prettyEdition(e.file.Syntax())),
		report.Snippet(e.where),
		report.Snippetf(e.file.AST().Syntax().Value(), "%s specified here", kind),
	)

	if e.intro != syntax.Unknown {
		d.Apply(report.Helpf("`%s` requires at least %s", e.what, prettyEdition(e.intro)))
	}
}

type errEditionTooNew struct {
	file                *File
	deprecated, removed syntax.Syntax
	warning             string

	what  any
	where source.Spanner
}

func (e errEditionTooNew) Diagnose(d *report.Diagnostic) {
	kind := "syntax"
	if e.file.Syntax().IsEdition() {
		kind = "edition"
	}

	err := "not supported"
	if !e.isRemoved() {
		err = "deprecated"
	}

	d.Apply(
		report.Message("`%s` is %s in %s", e.what, err, prettyEdition(e.file.Syntax())),
		report.Snippet(e.where),
		report.Snippetf(e.file.AST().Syntax().Value(), "%s specified here", kind),
	)

	if e.isRemoved() {
		if e.isDeprecated() {
			d.Apply(report.Helpf("deprecated since %s, removed in %s", prettyEdition(e.deprecated), prettyEdition(e.removed)))
		} else {
			d.Apply(report.Helpf("removed in %s", prettyEdition(e.removed)))
		}
	} else if e.isDeprecated() {
		if e.removed != syntax.Unknown {
			d.Apply(report.Helpf("deprecated since %s, to be removed in %s", prettyEdition(e.deprecated), prettyEdition(e.removed)))
		} else {
			d.Apply(report.Helpf("deprecated since %s", prettyEdition(e.deprecated)))
		}
	}

	if e.warning != "" {
		// Canonicalize whitespace. Some built-in deprecation warnings have
		// double spaces after periods.
		text := whitespacePattern.ReplaceAllString(e.warning, " ")
		d.Apply(report.Helpf("deprecated: %s", text))
	}
}

func (e errEditionTooNew) isDeprecated() bool {
	return e.deprecated != syntax.Unknown && e.deprecated <= e.file.Syntax()
}
func (e errEditionTooNew) isRemoved() bool {
	return e.removed != syntax.Unknown && e.removed <= e.file.Syntax()
}
