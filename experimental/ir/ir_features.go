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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast/syntax"
	"github.com/bufbuild/protocompile/internal/arena"
)

// FeatureSet represents the Editions-mediated features of a particular
// declaration.
type FeatureSet struct {
	withContext
	raw *rawFeatureSet
}

// Feature is a feature setting retrieved from a [FeatureSet].
type Feature struct {
	withContext
	raw rawFeature
}

// FeatureInfo represents information about a message field being used as a
// feature. This corresponds to the edition_defaults and feature_support options
// on a field.
type FeatureInfo struct {
	withContext
	raw *rawFeatureInfo
}

type rawFeatureSet struct {
	features map[featureKey]rawFeature
	parent   arena.Pointer[rawFeatureSet]
	options  arena.Pointer[rawValue]
}

type rawFeature struct {
	// Can't be a ref because it might not be imported by this file at all.
	value                            Value
	isCustom, isInherited, isDefault bool
}

type rawFeatureInfo struct {
	defaults                        []featureDefault // Sorted by edition.
	introduced, deprecated, removed syntax.Syntax
	deprecationWarning              string
}

type featureKey struct {
	extension, field *rawMember
}

type featureDefault struct {
	edition syntax.Syntax
	value   arena.Pointer[rawValue]
}

// Parent returns the feature set of the parent scope for this feature.
//
// Returns zero if this is the feature set for the file.
func (fs FeatureSet) Parent() FeatureSet {
	if fs.IsZero() || fs.raw.parent.Nil() {
		return FeatureSet{}
	}
	return FeatureSet{
		fs.withContext,
		fs.Context().arenas.features.Deref(fs.raw.parent),
	}
}

// Options returns the value of the google.protobuf.FeatureSet message that
// this FeatureSet is built from.
func (fs FeatureSet) Options() MessageValue {
	if fs.IsZero() {
		return MessageValue{}
	}
	return wrapValue(fs.Context(), fs.raw.options).AsMessage()
}

// Lookup looks up a feature with the given google.protobuf.FeatureSet member.
func (fs FeatureSet) Lookup(field Member) Feature {
	return fs.LookupCustom(Member{}, field)
}

// LookupCustom looks up a custom feature in the given extension's field.
func (fs FeatureSet) LookupCustom(extension, field Member) Feature {
	if fs.IsZero() {
		return Feature{}
	}
	// First, check if this value is cached.
	key := featureKey{extension.raw, field.raw}
	if f, ok := fs.raw.features[key]; ok {
		return Feature{fs.withContext, f}
	}

	raw := rawFeature{isCustom: !extension.IsZero()}

	// Check to see if it's set in the options message.
	options := fs.Options()
	if !options.IsZero() && !extension.IsZero() {
		// If the extension is not set, this will zero out options, so we'll
		// just go to the next one.
		options = options.Field(extension).AsMessage()
	}

	if !options.IsZero() {
		raw.value = options.Field(field)
	}

	if raw.value.IsZero() {
		if parent := fs.Parent(); !parent.IsZero() {
			// If parent is non-nil, recurse.
			raw = fs.Parent().LookupCustom(extension, field).raw
			raw.isInherited = true
		} else {
			// Otherwise, we need to look for the edition default.
			raw.value = field.FeatureInfo().Default(fs.Context().File().Syntax())
			raw.isInherited = true
			raw.isDefault = true
		}
	}

	if raw.value.IsZero() {
		return Feature{}
	}

	if fs.raw.features == nil {
		fs.raw.features = make(map[featureKey]rawFeature)
	}
	fs.raw.features[key] = raw
	return Feature{fs.withContext, raw}
}

// Field returns the field corresponding to this feature value.
func (f Feature) Field() Member {
	return f.Value().Field()
}

// IsCustom returns whether this is a custom feature.
func (f Feature) IsCustom() bool {
	return !f.IsZero() && f.raw.isCustom
}

// IsInherited returns whether this feature value was inherited from its parent.
func (f Feature) IsInherited() bool {
	return !f.IsZero() && f.raw.isInherited
}

// IsExplicit returns whether this feature was set explicitly.
func (f Feature) IsExplicit() bool {
	return !f.IsZero() && !f.raw.isInherited
}

// IsDefault returns whether this feature was inherited from edition defaults.
// An explicit setting to the default will return false for this method.
func (f Feature) IsDefault() bool {
	return !f.IsZero() && f.raw.isDefault
}

// Type returns the type of this feature. May be zero if there is no specified
// default value for this feature in the current edition.
func (f Feature) Type() Type {
	return f.Field().Element()
}

// Value returns the value of this feature. May be zero if there is no specified
// value for this feature, given the current edition.
func (f Feature) Value() Value {
	return f.raw.value
}

// Default returns the default value for this feature.
func (f FeatureInfo) Default(edition syntax.Syntax) Value {
	if f.IsZero() {
		return Value{}
	}

	idx, ok := slices.BinarySearchFunc(f.raw.defaults, edition, func(a featureDefault, b syntax.Syntax) int {
		return cmp.Compare(a.edition, b)
	})
	if !ok && idx > 0 {
		idx-- // We're looking for the greatest lower bound.
	}
	return wrapValue(f.Context(), f.raw.defaults[idx].value)
}

// Introduced returns which edition this feature is first allowed in.
func (f FeatureInfo) Introduced() syntax.Syntax {
	if f.IsZero() {
		return syntax.Unknown
	}
	return f.raw.introduced
}

// Deprecated returns whether this feature has been deprecated, and in which
// edition.
func (f FeatureInfo) Deprecated() syntax.Syntax {
	if f.IsZero() {
		return syntax.Unknown
	}
	return f.raw.deprecated
}

// Removed returns whether this feature has been removed, and in which
// edition.
func (f FeatureInfo) Removed() syntax.Syntax {
	if f.IsZero() {
		return syntax.Unknown
	}
	return f.raw.removed
}

// DeprecationWarning returns the literal text of the deprecation warning for
// this feature, if it has been deprecated.
func (f FeatureInfo) DeprecationWarning() string {
	if f.IsZero() {
		return ""
	}
	return f.raw.deprecationWarning
}
