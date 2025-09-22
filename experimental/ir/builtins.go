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

	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// builtins contains those symbols that are built into the language, and
// which the compiler cannot handle not being present.
//
// See [resolveLangSymbols] for where they are resolved.
type builtins struct {
	fileOptions,
	messageOptions,
	fieldOptions,
	oneofOptions,
	enumOptions,
	enumValueOptions arena.Pointer[rawMember]

	mapEntry, packed, optionTargets arena.Pointer[rawMember]

	editionDefaults,
	editionDefaultKey,
	editionDefaultValue arena.Pointer[rawMember]

	editionSupport,
	editionSupportIntroduced,
	editionSupportDeprecated,
	editionSupportWarning,
	editionSupportRemoved arena.Pointer[rawMember]

	featureSet arena.Pointer[rawType]
	featurePresence,
	featureEnumType,
	featurePacked,
	featureGroup arena.Pointer[rawMember]

	fileFeatures,
	messageFeatures,
	fieldFeatures,
	oneofFeatures,
	enumFeatures,
	enumValueFeatures arena.Pointer[rawMember]
}

// builtinIDs contains [intern.ID]s for symbols with special meaning in the
// language.
type builtinIDs struct {
	DescriptorFile intern.ID `intern:"google/protobuf/descriptor.proto"`
	AnyPath        intern.ID `intern:"google.protobuf.Any"`

	FileOptions      intern.ID `intern:"google.protobuf.FileDescriptorProto.options"`
	MessageOptions   intern.ID `intern:"google.protobuf.DescriptorProto.options"`
	FieldOptions     intern.ID `intern:"google.protobuf.FieldDescriptorProto.options"`
	OneofOptions     intern.ID `intern:"google.protobuf.OneofDescriptorProto.options"`
	EnumOptions      intern.ID `intern:"google.protobuf.EnumDescriptorProto.options"`
	EnumValueOptions intern.ID `intern:"google.protobuf.EnumValueDescriptorProto.options"`

	MapEntry      intern.ID `intern:"google.protobuf.MessageOptions.map_entry"`
	Packed        intern.ID `intern:"google.protobuf.FieldOptions.packed"`
	OptionTargets intern.ID `intern:"google.protobuf.FieldOptions.targets"`

	FileUninterpreted      intern.ID `intern:"google.protobuf.FileOptions.uninterpreted_options"`
	MessageUninterpreted   intern.ID `intern:"google.protobuf.MessageOptions.uninterpreted_options"`
	FieldUninterpreted     intern.ID `intern:"google.protobuf.FieldOptions.uninterpreted_options"`
	OneofUninterpreted     intern.ID `intern:"google.protobuf.OneofOptions.uninterpreted_options"`
	EnumUninterpreted      intern.ID `intern:"google.protobuf.EnumOptions.uninterpreted_options"`
	EnumValueUninterpreted intern.ID `intern:"google.protobuf.EnumValueOptions.uninterpreted_options"`

	EditionDefaults     intern.ID `intern:"google.protobuf.FieldOptions.edition_defaults"`
	EditionDefaultKey   intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.edition"`
	EditionDefaultValue intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.value"`

	EditionSupport           intern.ID `intern:"google.protobuf.FieldOptions.feature_support"`
	EditionSupportIntroduced intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_introduced"`
	EditionSupportDeprecated intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_deprecated"`
	EditionSupportWarning    intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.deprecation_warning"`
	EditionSupportRemoved    intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_removed"`

	FeatureSet      intern.ID `intern:"google.protobuf.FeatureSet"`
	FeaturePresence intern.ID `intern:"google.protobuf.FeatureSet.field_presence"`
	FeatureEnumType intern.ID `intern:"google.protobuf.FeatureSet.enum_type"`
	FeaturePacked   intern.ID `intern:"google.protobuf.FeatureSet.repeated_field_encoding"`
	FeatureGroup    intern.ID `intern:"google.protobuf.FeatureSet.message_encoding"`

	FileFeatures      intern.ID `intern:"google.protobuf.FileOptions.features"`
	MessageFeatures   intern.ID `intern:"google.protobuf.MessageOptions.features"`
	FieldFeatures     intern.ID `intern:"google.protobuf.FieldOptions.features"`
	OneofFeatures     intern.ID `intern:"google.protobuf.OneofOptions.features"`
	EnumFeatures      intern.ID `intern:"google.protobuf.EnumOptions.features"`
	EnumValueFeatures intern.ID `intern:"google.protobuf.EnumValueOptions.features"`
}

func resolveBuiltins(c *Context) {
	if !c.File().IsDescriptorProto() {
		return
	}

	names := &c.session.builtinIDs
	c.builtins = &builtins{
		fileOptions: mustResolve[rawMember](c, names.FileOptions, SymbolKindField),

		messageOptions: mustResolve[rawMember](c, names.MessageOptions, SymbolKindField),
		fieldOptions:   mustResolve[rawMember](c, names.FieldOptions, SymbolKindField),
		oneofOptions:   mustResolve[rawMember](c, names.OneofOptions, SymbolKindField),

		enumOptions:      mustResolve[rawMember](c, names.EnumOptions, SymbolKindField),
		enumValueOptions: mustResolve[rawMember](c, names.EnumValueOptions, SymbolKindField),

		mapEntry:      mustResolve[rawMember](c, names.MapEntry, SymbolKindField),
		packed:        mustResolve[rawMember](c, names.Packed, SymbolKindField),
		optionTargets: mustResolve[rawMember](c, names.OptionTargets, SymbolKindField),

		editionDefaults:     mustResolve[rawMember](c, names.EditionDefaults, SymbolKindField),
		editionDefaultKey:   mustResolve[rawMember](c, names.EditionDefaultKey, SymbolKindField),
		editionDefaultValue: mustResolve[rawMember](c, names.EditionDefaultValue, SymbolKindField),

		editionSupport:           mustResolve[rawMember](c, names.EditionSupport, SymbolKindField),
		editionSupportIntroduced: mustResolve[rawMember](c, names.EditionSupportIntroduced, SymbolKindField),
		editionSupportDeprecated: mustResolve[rawMember](c, names.EditionSupportDeprecated, SymbolKindField),
		editionSupportWarning:    mustResolve[rawMember](c, names.EditionSupportWarning, SymbolKindField),
		editionSupportRemoved:    mustResolve[rawMember](c, names.EditionSupportRemoved, SymbolKindField),

		featureSet:      mustResolve[rawType](c, names.FeatureSet, SymbolKindMessage),
		featurePresence: mustResolve[rawMember](c, names.FeaturePresence, SymbolKindField),
		featureEnumType: mustResolve[rawMember](c, names.FeatureEnumType, SymbolKindField),
		featurePacked:   mustResolve[rawMember](c, names.FeaturePacked, SymbolKindField),
		featureGroup:    mustResolve[rawMember](c, names.FeatureGroup, SymbolKindField),

		fileFeatures:      mustResolve[rawMember](c, names.FileFeatures, SymbolKindField),
		messageFeatures:   mustResolve[rawMember](c, names.MessageFeatures, SymbolKindField),
		fieldFeatures:     mustResolve[rawMember](c, names.FieldFeatures, SymbolKindField),
		oneofFeatures:     mustResolve[rawMember](c, names.OneofFeatures, SymbolKindField),
		enumFeatures:      mustResolve[rawMember](c, names.EnumFeatures, SymbolKindField),
		enumValueFeatures: mustResolve[rawMember](c, names.EnumValueFeatures, SymbolKindField),
	}
}

// mustResolve resolves a descriptor.proto name, and panics if it's not found.
func mustResolve[Raw any](c *Context, id intern.ID, kind SymbolKind) arena.Pointer[Raw] {
	ref := c.exported.lookup(c, id)
	sym := wrapSymbol(c, ref)
	if sym.Kind() != kind {
		panic(fmt.Errorf(
			"missing descriptor.proto symbol: %s `%s`; got kind %s",
			kind.noun(), c.session.intern.Value(id), sym.Kind(),
		))
	}
	return arena.Pointer[Raw](sym.raw.data)
}
