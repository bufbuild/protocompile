// Copyright 2020-2026 Buf Technologies, Inc.
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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast/predeclared"
	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/erredition"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

func buildAllFeatureInfo(file *File, r *report.Report) {
	for m := range file.AllMembers() {
		if m.IsEnumValue() {
			buildEnumValueFeatureInfo(m, r)
		} else {
			buildFeatureInfo(m, r)
		}
	}
}

// validateFeatureExtensions checks the message type of each FeatureSet
// extension in file. Like protoc, every field of that type is a feature, and
// must set edition_defaults.
func validateFeatureExtensions(file *File, r *report.Report) {
	builtins := file.builtins()
	seen := make(map[FullName]bool)
	for extn := range seq.Values(file.AllExtensions()) {
		ty := extn.Element()
		if extn.Container() != builtins.FeatureSet || !ty.IsMessage() || seen[ty.FullName()] {
			continue
		}
		seen[ty.FullName()] = true

		usedHere := report.Snippetf(extn.AST().Name(), "`%s` is used as features here", ty.Name())
		mistake := report.Notef("this is likely a mistake, but it is not rejected by protoc")
		for field := range seq.Values(ty.Members()) {
			if field.Options().Field(builtins.EditionDefaults).IsZero() {
				var where source.Spanner = field.AST().Options()
				if field.AST().Options().IsZero() {
					where = field.AST().Name()
				}
				r.Warnf("expected feature field to set `%s`", builtins.EditionDefaults.Name()).Apply(
					report.Snippet(where), usedHere, mistake,
				)
			}

			support := field.Options().Field(builtins.EditionSupport).AsMessage()
			if !support.IsZero() && support.Field(builtins.EditionSupportIntroduced).IsZero() {
				r.Warnf("expected `%s.%s` to be set",
					builtins.EditionSupportIntroduced.Container().Name(),
					builtins.EditionSupportIntroduced.Name(),
				).Apply(
					report.Snippet(support.AsValue().ValueAST()), usedHere, mistake,
				)
			}
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
	// Ordinary options may set feature_support without edition_defaults, such
	// as cc_enable_arenas. Feature fields are checked for edition_defaults by
	// validateFeatureExtensions.
	if !defaults.IsZero() {
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
		buildFeatureSupport(info, support, r)
	}

	field.Raw().featureInfo = info
}

// buildEnumValueFeatureInfo builds lifetime information for an enum value
// that sets feature_support. protoc checks it whenever the value is used in
// an option or feature.
func buildEnumValueFeatureInfo(value Member, r *report.Report) {
	builtins := value.Context().builtins()
	support := value.Options().Field(builtins.EnumValueSupport).AsMessage()
	if support.IsZero() {
		return
	}

	info := new(rawFeatureInfo)
	buildFeatureSupport(info, support, r)
	value.Raw().featureInfo = info
}

// buildFeatureSupport fills in the lifetime fields of info from a
// FeatureSupport message.
func buildFeatureSupport(info *rawFeatureInfo, support MessageValue, r *report.Report) {
	builtins := support.Context().builtins()
	mistake := report.Notef("this is likely a mistake, but it is not rejected by protoc")

	value := support.Field(builtins.EditionSupportIntroduced)
	n, _ := value.AsInt()
	info.introduced = syntax.Syntax(n)
	if !value.IsZero() && info.introduced == syntax.Unknown {
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

	value = support.Field(builtins.EditionSupportRemovalErr)
	info.removalErr, _ = value.AsString()

	validateFeatureSupport(newFeatureSupport(support), r)
}

// validateEnumValueSupport checks the feature_support of each enum value
// usable by an enum-typed field that sets feature_support. Like protoc, a
// value's support is merged over the field's, and must not widen it.
func validateEnumValueSupport(file *File, r *report.Report) {
	builtins := file.builtins()
	for field := range file.AllMembers() {
		enum := field.Element()
		if field.IsEnumValue() || !enum.IsEnum() {
			continue
		}
		support := field.Options().Field(builtins.EditionSupport).AsMessage()
		if support.IsZero() {
			continue
		}
		parent := newFeatureSupport(support)

		for value := range seq.Values(enum.Members()) {
			support := value.Options().Field(builtins.EnumValueSupport).AsMessage()
			if support.IsZero() {
				continue
			}
			own := newFeatureSupport(support)
			merged := parent.mergedWith(own)
			inherits := report.Snippetf(value.AST().Name(),
				"`%s` inherits `%s` from `%s`", value.Name(), builtins.EditionSupport.Name(), field.Name())

			// Problems with the value's own support are reported on the value.
			if validateFeatureSupport(own, nil) {
				validateFeatureSupport(merged, r, inherits)
			}

			widens := func(what string, valueEdition, fieldEdition Value) {
				r.Errorf("`%s` is %s after `%s`", value.Name(), what, field.Name()).Apply(
					report.Snippet(valueEdition.ValueAST()),
					report.Snippetf(fieldEdition.ValueAST(), "`%s` is %s here", field.Name(), what),
				)
			}
			if editionOf(merged.introduced) < editionOf(parent.introduced) {
				r.Errorf("`%s` is introduced before `%s`", value.Name(), field.Name()).Apply(
					report.Snippet(merged.introduced.ValueAST()),
					report.Snippetf(parent.introduced.ValueAST(), "`%s` is introduced here", field.Name()),
				)
			}
			if !parent.removed.IsZero() && editionOf(merged.removed) > editionOf(parent.removed) {
				widens("removed", merged.removed, parent.removed)
			}
			if !parent.deprecated.IsZero() && editionOf(merged.deprecated) > editionOf(parent.deprecated) {
				widens("deprecated", merged.deprecated, parent.deprecated)
			}
		}
	}
}

// featureSupport holds the fields of a FeatureSupport message. Unset fields
// are zero.
type featureSupport struct {
	builtins                                             *builtins
	introduced, deprecated, removed, warning, removalErr Value
}

func newFeatureSupport(support MessageValue) featureSupport {
	builtins := support.Context().builtins()
	return featureSupport{
		builtins:   builtins,
		introduced: support.Field(builtins.EditionSupportIntroduced),
		deprecated: support.Field(builtins.EditionSupportDeprecated),
		removed:    support.Field(builtins.EditionSupportRemoved),
		warning:    support.Field(builtins.EditionSupportWarning),
		removalErr: support.Field(builtins.EditionSupportRemovalErr),
	}
}

// mergedWith returns s with any fields set in other replacing its own, like
// merging two FeatureSupport messages.
func (s featureSupport) mergedWith(other featureSupport) featureSupport {
	pick := func(a, b Value) Value {
		if !b.IsZero() {
			return b
		}
		return a
	}
	s.introduced = pick(s.introduced, other.introduced)
	s.deprecated = pick(s.deprecated, other.deprecated)
	s.removed = pick(s.removed, other.removed)
	s.warning = pick(s.warning, other.warning)
	s.removalErr = pick(s.removalErr, other.removalErr)
	return s
}

// editionOf returns the edition an edition-valued field is set to.
func editionOf(v Value) syntax.Syntax {
	n, _ := v.AsInt()
	return syntax.Syntax(n)
}

// validateFeatureSupport diagnoses inconsistent lifetimes in s, which protoc
// rejects for every field and enum value, and returns whether s is valid. If r
// is nil, nothing is reported. Each diagnostic starts with the options in
// extra, so they can provide its primary snippet.
func validateFeatureSupport(s featureSupport, r *report.Report, extra ...report.DiagnosticOption) bool {
	ok := true
	fail := func(format string, args ...any) *report.Diagnostic {
		ok = false
		if r == nil {
			return nil
		}
		return r.Errorf(format, args...)
	}
	requires := func(set Value, missing Member) {
		if d := fail("`%s` requires `%s` to be set", set.Field().Name(), missing.Name()); d != nil {
			d.Apply(extra...)
			d.Apply(report.Snippet(set.KeyAST()))
		}
	}
	ordered := func(format string, later, earlier Value) {
		if d := fail(format, later.Field().Name(), earlier.Field().Name()); d != nil {
			d.Apply(extra...)
			d.Apply(
				report.Snippet(later.ValueAST()),
				report.Snippetf(earlier.ValueAST(), "`%s` set here", earlier.Field().Name()),
			)
		}
	}

	introduced, deprecated, removed := editionOf(s.introduced), editionOf(s.deprecated), editionOf(s.removed)
	if !s.deprecated.IsZero() {
		if !s.introduced.IsZero() && deprecated < introduced {
			ordered("`%s` must not be before `%s`", s.deprecated, s.introduced)
		}
		if s.warning.IsZero() {
			requires(s.deprecated, s.builtins.EditionSupportWarning)
		}
	} else if !s.warning.IsZero() {
		requires(s.warning, s.builtins.EditionSupportDeprecated)
	}

	if !s.removed.IsZero() {
		if !s.deprecated.IsZero() && deprecated >= removed {
			ordered("`%s` must be before `%s`", s.deprecated, s.removed)
		}
		if !s.introduced.IsZero() && removed < introduced {
			ordered("`%s` must not be before `%s`", s.removed, s.introduced)
		}
		// protoc exempts options introduced and removed in the same edition.
		if s.removalErr.IsZero() && introduced != removed {
			requires(s.removed, s.builtins.EditionSupportRemovalErr)
		}
	} else if !s.removalErr.IsZero() {
		requires(s.removalErr, s.builtins.EditionSupportRemoved)
	}
	return ok
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
			// Do not set features on map entry fields here.
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

		// Oneof features must be resolved before member features, since a field
		// declared inside a oneof inherits from the oneof.
		for oneof := range seq.Values(ty.Oneofs()) {
			features := oneof.Options().Field(builtins.OneofFeatures)
			validateFeatures(features.AsMessage(), r)
			oneof.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  ty.Raw().features,
			}))
		}
		for member := range seq.Values(ty.Members()) {
			option := builtins.FieldFeatures
			if member.IsEnumValue() {
				option = builtins.EnumValueFeatures
			}

			// A field belonging to a oneof inherits features from the oneof.
			parent := ty.Raw().features
			if oneof := member.Oneof(); !oneof.IsZero() {
				parent = oneof.Raw().features
			}

			features := member.Options().Field(option)
			validateFeatures(features.AsMessage(), r)
			member.Raw().features = id.ID[FeatureSet](file.arenas.features.NewCompressed(rawFeatureSet{
				options: features.ID(),
				parent:  parent,
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

		// A base feature must never resolve to its `*_UNKNOWN` sentinel (zero).
		if field := feature.Field(); field.Container() == builtins.FeatureSet && !field.IsExtension() {
			if sentinel := feature.AsEnum(); !sentinel.IsZero() && sentinel.Number() == 0 {
				r.Errorf("feature field `%s` must resolve to a known value", field.Name()).Apply(
					report.Snippet(feature.ValueAST()),
					report.Helpf("`%s` is only a placeholder for an unset feature", sentinel.Name()),
				)
				continue
			}
		}

		diagnoseLifetime(feature, info, r)
		diagnoseEnumLifetimes(feature, r)
	}
}

// validateOptionLifetimes validates that options which declare a
// feature_support, such as cc_enable_arenas, are compatible with the current
// edition. Features are validated separately by [validateFeatures].
func validateOptionLifetimes(file *File, r *report.Report) {
	validateOptionLifetimesIn(file.Options(), r)
	for ty := range seq.Values(file.AllTypes()) {
		validateOptionLifetimesIn(ty.Options(), r)
		for member := range seq.Values(ty.Members()) {
			validateOptionLifetimesIn(member.Options(), r)
		}
		for oneof := range seq.Values(ty.Oneofs()) {
			validateOptionLifetimesIn(oneof.Options(), r)
		}
		for extns := range seq.Values(ty.ExtensionRanges()) {
			validateOptionLifetimesIn(extns.Options(), r)
		}
	}
	for extn := range seq.Values(file.AllExtensions()) {
		validateOptionLifetimesIn(extn.Options(), r)
	}
	for service := range seq.Values(file.Services()) {
		validateOptionLifetimesIn(service.Options(), r)
		for method := range seq.Values(service.Methods()) {
			validateOptionLifetimesIn(method.Options(), r)
		}
	}
}

func validateOptionLifetimesIn(options MessageValue, r *report.Report) {
	if options.IsZero() {
		return
	}

	builtins := options.Context().builtins()
	for value := range options.Fields() {
		field := value.Field()
		switch {
		case field.Element() == builtins.FeatureSet:
			continue // Validated by validateFeatures.
		case field == builtins.JavaMultipleFiles:
			continue // Has a more specific diagnostic in validateFileOptions.
		}

		diagnoseLifetime(value, field.FeatureInfo(), r)
		diagnoseEnumLifetimes(value, r)
		for elem := range seq.Values(value.Elements()) {
			validateOptionLifetimesIn(elem.AsMessage(), r)
		}
	}
}

// diagnoseEnumLifetimes diagnoses each enum value set in value whose
// feature_support does not permit it in the current edition.
func diagnoseEnumLifetimes(value Value, r *report.Report) {
	if !value.Field().Element().IsEnum() {
		return
	}
	for elem := range seq.Values(value.Elements()) {
		enum := elem.AsEnum()
		diagnoseLifetimeOf(value.Context(), enum.FeatureInfo(), enum.Name(), elem.AST(), r)
	}
}

// diagnoseLifetime diagnoses value if its field's feature_support does not
// permit it in the current edition.
func diagnoseLifetime(value Value, info FeatureInfo, r *report.Report) {
	diagnoseLifetimeOf(value.Context(), info, value.Field().Name(), value.KeyAST(), r)
}

// diagnoseLifetimeOf diagnoses what, spanning where, if info does not permit
// it in the current edition.
func diagnoseLifetimeOf(file *File, info FeatureInfo, what string, where source.Spanner, r *report.Report) {
	if info.IsZero() {
		return
	}

	edition := file.Syntax()

	// We check these in reverse order, because the user might have set
	// introduced == deprecated == removed, and protoc doesn't enforce
	// any relationship between these.
	switch {
	case info.IsRemoved(edition), info.IsDeprecated(edition):
		r.SoftError(info.IsRemoved(edition), erredition.TooNew{
			Current:          edition,
			Decl:             file.AST().Syntax(),
			Removed:          info.Removed(),
			Deprecated:       info.Deprecated(),
			RemovedReason:    info.RemovalError(),
			DeprecatedReason: info.DeprecationWarning(),
			What:             what,
			Where:            where,
		})

	case !info.IsIntroduced(edition):
		r.Error(erredition.TooOld{
			Current: edition,
			Decl:    file.AST().Syntax(),
			Intro:   info.Introduced(),
			What:    what,
			Where:   where,
		})
	}
}
