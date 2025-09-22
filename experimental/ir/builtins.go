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
	"reflect"

	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// builtinIDs contains [intern.ID]s for symbols with special meaning in the
// language.
// builtins contains those symbols that are built into the language, and which the compiler cannot
// handle not being present. This field is only present in the Context
// for descriptor.proto.
//
// This is resolved using reflection in [resolveLangSymbols]. The names of the
// fields of this type must match those in builtinIDs that names its symbol.
type builtins struct {
	FileOptions      Member
	MessageOptions   Member
	FieldOptions     Member
	OneofOptions     Member
	RangeOptions     Member
	EnumOptions      Member
	EnumValueOptions Member
	ServiceOptions   Member
	MethodOptions    Member

	MapEntry      Member
	Packed        Member
	OptionTargets Member

	EditionDefaults, EditionDefaultsKey, EditionDefaultsValue Member

	EditionSupport           Member
	EditionSupportIntroduced Member
	EditionSupportDeprecated Member
	EditionSupportWarning    Member
	EditionSupportRemoved    Member

	FeatureSet      Type
	FeaturePresence Member
	FeatureEnumType Member
	FeaturePacked   Member
	FeatureGroup    Member

	FileFeatures      Member
	MessageFeatures   Member
	FieldFeatures     Member
	OneofFeatures     Member
	RangeFeatures     Member
	EnumFeatures      Member
	EnumValueFeatures Member
	ServiceFeatures   Member
	MethodFeatures    Member
}

// builtinIDs is all of the interning IDs of names in [builtins], plus some
// others. This lives inside of [Session] and is constructed once.
type builtinIDs struct {
	DescriptorFile intern.ID `intern:"google/protobuf/descriptor.proto"`
	AnyPath        intern.ID `intern:"google.protobuf.Any"`

	FileOptions      intern.ID `intern:"google.protobuf.FileDescriptorProto.options"`
	MessageOptions   intern.ID `intern:"google.protobuf.DescriptorProto.options"`
	FieldOptions     intern.ID `intern:"google.protobuf.FieldDescriptorProto.options"`
	OneofOptions     intern.ID `intern:"google.protobuf.OneofDescriptorProto.options"`
	RangeOptions     intern.ID `intern:"google.protobuf.DescriptorProto.ExtensionRange.options"`
	EnumOptions      intern.ID `intern:"google.protobuf.EnumDescriptorProto.options"`
	EnumValueOptions intern.ID `intern:"google.protobuf.EnumValueDescriptorProto.options"`
	ServiceOptions   intern.ID `intern:"google.protobuf.ServiceDescriptorProto.options"`
	MethodOptions    intern.ID `intern:"google.protobuf.MethodDescriptorProto.options"`

	MapEntry      intern.ID `intern:"google.protobuf.MessageOptions.map_entry"`
	Packed        intern.ID `intern:"google.protobuf.FieldOptions.packed"`
	OptionTargets intern.ID `intern:"google.protobuf.FieldOptions.targets"`

	FileUninterpreted      intern.ID `intern:"google.protobuf.FileOptions.uninterpreted_options"`
	MessageUninterpreted   intern.ID `intern:"google.protobuf.MessageOptions.uninterpreted_options"`
	FieldUninterpreted     intern.ID `intern:"google.protobuf.FieldOptions.uninterpreted_options"`
	OneofUninterpreted     intern.ID `intern:"google.protobuf.OneofOptions.uninterpreted_options"`
	EnumUninterpreted      intern.ID `intern:"google.protobuf.EnumOptions.uninterpreted_options"`
	EnumValueUninterpreted intern.ID `intern:"google.protobuf.EnumValueOptions.uninterpreted_options"`

	EditionDefaults      intern.ID `intern:"google.protobuf.FieldOptions.edition_defaults"`
	EditionDefaultsKey   intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.edition"`
	EditionDefaultsValue intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.value"`

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
	RangeFeatures     intern.ID `intern:"google.protobuf.ExtensionRangeOptions.features"`
	EnumFeatures      intern.ID `intern:"google.protobuf.EnumOptions.features"`
	EnumValueFeatures intern.ID `intern:"google.protobuf.EnumValueOptions.features"`
	ServiceFeatures   intern.ID `intern:"google.protobuf.ServiceOptions.features"`
	MethodFeatures    intern.ID `intern:"google.protobuf.MethodOptions.features"`
}

func resolveBuiltins(c *Context) {
	if !c.File().IsDescriptorProto() {
		return
	}

	// If adding a new kind of symbol to resolve, add it to this map.
	kinds := map[reflect.Type]struct {
		kind SymbolKind
		wrap func(arena.Untyped, reflect.Value)
	}{
		reflect.TypeFor[Member](): {
			kind: SymbolKindField,
			wrap: makeBuiltinWrapper(c, wrapMember),
		},
		reflect.TypeFor[Type](): {
			kind: SymbolKindMessage,
			wrap: makeBuiltinWrapper(c, wrapType),
		},
	}

	c.dpBuiltins = new(builtins)
	v := reflect.ValueOf(c.dpBuiltins).Elem()
	ids := reflect.ValueOf(c.session.builtins)
	for i := range v.NumField() {
		field := v.Field(i)
		fmt.Println(v.Type().Field(i).Name)
		id := ids.FieldByName(v.Type().Field(i).Name).Interface().(intern.ID) //nolint:errcheck
		kind := kinds[field.Type()]

		ref := c.exported.lookup(c, id)
		sym := wrapSymbol(c, ref)
		if sym.Kind() != kind.kind {
			panic(fmt.Errorf(
				"missing descriptor.proto symbol: %s `%s`; got kind %s",
				kind.kind.noun(), c.session.intern.Value(id), sym.Kind(),
			))
		}
		kind.wrap(sym.raw.data, field)
	}
}

// makeBuiltinWrapper helps construct reflection shims for resolveBuiltins.
func makeBuiltinWrapper[T any, Raw any](c *Context, wrap func(*Context, ref[Raw]) T) func(arena.Untyped, reflect.Value) {
	return func(p arena.Untyped, out reflect.Value) {
		x := wrap(c, ref[Raw]{ptr: arena.Pointer[Raw](p)})
		out.Set(reflect.ValueOf(x))
	}
}
