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
	"reflect"
	"strings"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/arena"
	"github.com/bufbuild/protocompile/internal/intern"
)

// builtins contains those symbols that are built into the language, referenced
// by the compiler for lowering. This field is only present in the Context for
// descriptor.proto.
//
// Fields are resolved using reflection in [resolveBuiltins]. The names of the
// fields of this type must match the corresponding entries in [builtinIDs].
//
// Fields without a tag are required: any descriptor.proto missing one of them
// is considered genuinely broken, and [resolveBuiltins] emits an error
// diagnostic for each missing required symbol. Fields tagged
// `builtin:"optional"` may be absent without diagnostic — they correspond to
// post-proto2 or editions-only features, and older vendored copies of
// descriptor.proto will legitimately not contain them.
type builtins struct {
	FileOptions      Member
	MessageOptions   Member
	FieldOptions     Member
	OneofOptions     Member
	RangeOptions     Member `builtin:"optional"`
	EnumOptions      Member
	EnumValueOptions Member
	ServiceOptions   Member
	MethodOptions    Member

	JavaUTF8          Member
	JavaMultipleFiles Member
	OptimizeFor       Member
	MapEntry          Member
	Packed            Member
	OptionTargets     Member `builtin:"optional"`
	CType, JSType     Member
	Lazy              Member
	UnverifiedLazy    Member `builtin:"optional"`
	AllowAlias        Member
	MessageSet        Member
	JSONName          Member

	ExtnDecls        Member `builtin:"optional"`
	ExtnVerification Member `builtin:"optional"`
	ExtnDeclNumber   Member `builtin:"optional"`
	ExtnDeclName     Member `builtin:"optional"`
	ExtnDeclType     Member `builtin:"optional"`
	ExtnDeclReserved Member `builtin:"optional"`
	ExtnDeclRepeated Member `builtin:"optional"`

	FileDeprecated      Member
	MessageDeprecated   Member
	FieldDeprecated     Member
	EnumDeprecated      Member
	EnumValueDeprecated Member
	ServiceDeprecated   Member
	MethodDeprecated    Member

	EditionDefaults      Member `builtin:"optional"`
	EditionDefaultsKey   Member `builtin:"optional"`
	EditionDefaultsValue Member `builtin:"optional"`

	EditionSupport           Member `builtin:"optional"`
	EditionSupportIntroduced Member `builtin:"optional"`
	EditionSupportDeprecated Member `builtin:"optional"`
	EditionSupportWarning    Member `builtin:"optional"`
	EditionSupportRemoved    Member `builtin:"optional"`

	FeatureSet         Type   `builtin:"optional"`
	FeaturePresence    Member `builtin:"optional"`
	FeatureEnumType    Member `builtin:"optional"`
	FeaturePacked      Member `builtin:"optional"`
	FeatureUTF8        Member `builtin:"optional"`
	FeatureGroup       Member `builtin:"optional"`
	FeatureEnum        Member `builtin:"optional"`
	FeatureJSON        Member `builtin:"optional"`
	FeatureVisibility  Member `builtin:"optional"`
	FeatureNamingStyle Member `builtin:"optional"`

	FileFeatures      Member `builtin:"optional"`
	MessageFeatures   Member `builtin:"optional"`
	FieldFeatures     Member `builtin:"optional"`
	OneofFeatures     Member `builtin:"optional"`
	RangeFeatures     Member `builtin:"optional"`
	EnumFeatures      Member `builtin:"optional"`
	EnumValueFeatures Member `builtin:"optional"`
	ServiceFeatures   Member `builtin:"optional"`
	MethodFeatures    Member `builtin:"optional"`
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

	JavaUTF8          intern.ID `intern:"google.protobuf.FileOptions.java_string_check_utf8"`
	JavaMultipleFiles intern.ID `intern:"google.protobuf.FileOptions.java_multiple_files"`
	OptimizeFor       intern.ID `intern:"google.protobuf.FileOptions.optimize_for"`
	MapEntry          intern.ID `intern:"google.protobuf.MessageOptions.map_entry"`
	MessageSet        intern.ID `intern:"google.protobuf.MessageOptions.message_set_wire_format"`
	Packed            intern.ID `intern:"google.protobuf.FieldOptions.packed"`
	OptionTargets     intern.ID `intern:"google.protobuf.FieldOptions.targets"`
	CType             intern.ID `intern:"google.protobuf.FieldOptions.ctype"`
	JSType            intern.ID `intern:"google.protobuf.FieldOptions.jstype"`
	Lazy              intern.ID `intern:"google.protobuf.FieldOptions.lazy"`
	UnverifiedLazy    intern.ID `intern:"google.protobuf.FieldOptions.unverified_lazy"`
	AllowAlias        intern.ID `intern:"google.protobuf.EnumOptions.allow_alias"`
	JSONName          intern.ID `intern:"google.protobuf.FieldDescriptorProto.json_name"`

	ExtnDecls        intern.ID `intern:"google.protobuf.ExtensionRangeOptions.declaration"`
	ExtnVerification intern.ID `intern:"google.protobuf.ExtensionRangeOptions.verification"`
	ExtnDeclNumber   intern.ID `intern:"google.protobuf.ExtensionRangeOptions.Declaration.number"`
	ExtnDeclName     intern.ID `intern:"google.protobuf.ExtensionRangeOptions.Declaration.full_name"`
	ExtnDeclType     intern.ID `intern:"google.protobuf.ExtensionRangeOptions.Declaration.type"`
	ExtnDeclReserved intern.ID `intern:"google.protobuf.ExtensionRangeOptions.Declaration.reserved"`
	ExtnDeclRepeated intern.ID `intern:"google.protobuf.ExtensionRangeOptions.Declaration.repeated"`

	FileUninterpreted      intern.ID `intern:"google.protobuf.FileOptions.uninterpreted_option"`
	MessageUninterpreted   intern.ID `intern:"google.protobuf.MessageOptions.uninterpreted_option"`
	FieldUninterpreted     intern.ID `intern:"google.protobuf.FieldOptions.uninterpreted_option"`
	OneofUninterpreted     intern.ID `intern:"google.protobuf.OneofOptions.uninterpreted_option"`
	RangeUninterpreted     intern.ID `intern:"google.protobuf.ExtensionRangeOptions.uninterpreted_option"`
	EnumUninterpreted      intern.ID `intern:"google.protobuf.EnumOptions.uninterpreted_option"`
	EnumValueUninterpreted intern.ID `intern:"google.protobuf.EnumValueOptions.uninterpreted_option"`
	ServiceUninterpreted   intern.ID `intern:"google.protobuf.ServiceOptions.uninterpreted_option"`
	MethodUninterpreted    intern.ID `intern:"google.protobuf.MethodOptions.uninterpreted_option"`

	FileDeprecated      intern.ID `intern:"google.protobuf.FileOptions.deprecated"`
	MessageDeprecated   intern.ID `intern:"google.protobuf.MessageOptions.deprecated"`
	FieldDeprecated     intern.ID `intern:"google.protobuf.FieldOptions.deprecated"`
	EnumDeprecated      intern.ID `intern:"google.protobuf.EnumOptions.deprecated"`
	EnumValueDeprecated intern.ID `intern:"google.protobuf.EnumValueOptions.deprecated"`
	ServiceDeprecated   intern.ID `intern:"google.protobuf.ServiceOptions.deprecated"`
	MethodDeprecated    intern.ID `intern:"google.protobuf.MethodOptions.deprecated"`

	EditionDefaults      intern.ID `intern:"google.protobuf.FieldOptions.edition_defaults"`
	EditionDefaultsKey   intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.edition"`
	EditionDefaultsValue intern.ID `intern:"google.protobuf.FieldOptions.EditionDefault.value"`

	EditionSupport           intern.ID `intern:"google.protobuf.FieldOptions.feature_support"`
	EditionSupportIntroduced intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_introduced"`
	EditionSupportDeprecated intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_deprecated"`
	EditionSupportWarning    intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.deprecation_warning"`
	EditionSupportRemoved    intern.ID `intern:"google.protobuf.FieldOptions.FeatureSupport.edition_removed"`

	FeatureSet         intern.ID `intern:"google.protobuf.FeatureSet"`
	FeaturePresence    intern.ID `intern:"google.protobuf.FeatureSet.field_presence"`
	FeatureEnumType    intern.ID `intern:"google.protobuf.FeatureSet.enum_type"`
	FeaturePacked      intern.ID `intern:"google.protobuf.FeatureSet.repeated_field_encoding"`
	FeatureUTF8        intern.ID `intern:"google.protobuf.FeatureSet.utf8_validation"`
	FeatureGroup       intern.ID `intern:"google.protobuf.FeatureSet.message_encoding"`
	FeatureEnum        intern.ID `intern:"google.protobuf.FeatureSet.enum_type"`
	FeatureJSON        intern.ID `intern:"google.protobuf.FeatureSet.json_format"`
	FeatureVisibility  intern.ID `intern:"google.protobuf.FeatureSet.default_symbol_visibility"`
	FeatureNamingStyle intern.ID `intern:"google.protobuf.FeatureSet.enforce_naming_style"`

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

// resolveBuiltins resolves the symbols from descriptor.proto.
//
// For each required field (untagged in [builtins]) that cannot be resolved,
// an error diagnostic is emitted on the descriptor.proto file. Optional
// fields (tagged `builtin:"optional"`) silently remain zero when absent.
// Downstream accessors handle zero members gracefully, so non-editions files
// continue to compile against older vendored copies of descriptor.proto.
func resolveBuiltins(file *File, r *report.Report) {
	if !file.IsDescriptorProto() {
		return
	}

	// If adding a new kind of symbol to resolve, add it to this map.
	kinds := map[reflect.Type]struct {
		kind SymbolKind
		wrap func(arena.Untyped, reflect.Value)
	}{
		reflect.TypeFor[Member](): {
			kind: SymbolKindField,
			wrap: makeBuiltinWrapper[Member](file),
		},
		reflect.TypeFor[Type](): {
			kind: SymbolKindMessage,
			wrap: makeBuiltinWrapper[Type](file),
		},
	}

	file.dpBuiltins = new(builtins)
	v := reflect.ValueOf(file.dpBuiltins).Elem()
	ids := reflect.ValueOf(file.session.builtins)

	for i := range v.NumField() {
		field := v.Field(i)
		tyField := v.Type().Field(i)

		id := ids.FieldByName(tyField.Name).Interface().(intern.ID) //nolint:errcheck
		kind := kinds[field.Type()]

		ref := file.exported.lookup(id)
		sym := GetRef(file, ref)
		if sym.Kind() != kind.kind {
			if !isOptionalBuiltinField(tyField) {
				r.Errorf("`%s` is missing required symbol `%s`", file.Path(), file.session.intern.Value(id)).Apply(
					report.Snippet(file.AST()),
					report.Helpf("the descriptor.proto supplied to the compiler does not declare this %s; "+
						"it may be vendored from a version that predates this symbol, or may be genuinely corrupt", kind.kind.noun()),
				)
			}
			continue
		}
		kind.wrap(sym.Raw().data, field)
	}
}

// makeBuiltinWrapper helps construct reflection shims for resolveBuiltins.
func makeBuiltinWrapper[T ~id.Node[T, *File, Raw], Raw any](
	file *File,
) func(arena.Untyped, reflect.Value) {
	return func(p arena.Untyped, out reflect.Value) {
		x := id.Wrap(file, id.ID[T](p))
		out.Set(reflect.ValueOf(x))
	}
}

// isOptionalBuiltinField reports whether the given [builtins] field is tagged
// `builtin:"optional"`.
func isOptionalBuiltinField(f reflect.StructField) bool {
	for option := range strings.SplitSeq(f.Tag.Get("builtin"), ",") {
		if option == "optional" {
			return true
		}
	}
	return false
}

// optionalBuiltinIDs is the set of intern IDs for symbols declared optional in
// [builtins]. It is populated at session init and consulted when emitting
// diagnostics about user references to missing optional builtins.
func optionalBuiltinIDs(ids *builtinIDs) map[intern.ID]struct{} {
	out := make(map[intern.ID]struct{})
	bs := reflect.TypeFor[builtins]()
	idsV := reflect.ValueOf(*ids)
	for i := range bs.NumField() {
		f := bs.Field(i)
		if !isOptionalBuiltinField(f) {
			continue
		}
		idField := idsV.FieldByName(f.Name)
		if !idField.IsValid() {
			continue
		}
		out[idField.Interface().(intern.ID)] = struct{}{} //nolint:errcheck
	}
	return out
}
