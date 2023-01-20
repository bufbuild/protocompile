// Copyright 2020-2022 Buf Technologies, Inc.
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

package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/reporter"
)

func TestBasicValidation(t *testing.T) {
	t.Parallel()
	testCases := map[string]struct {
		contents    string
		expectedErr string
	}{
		"success_large_negative_integer": {
			contents: `message Foo { optional double bar = 1 [default = -18446744073709551615]; }`,
		},
		"success_large_negative_integer_bom": {
			// with byte order marker
			contents: string([]byte{0xEF, 0xBB, 0xBF}) + `message Foo { optional double bar = 1 [default = -18446744073709551615]; }`,
		},
		"success_large_positive_integer": {
			contents: `message Foo { optional double bar = 1 [default = 18446744073709551616]; }`,
		},
		"success_message_set_wire_format_w_ext": {
			contents: `message Foo { extensions 100 to max; option message_set_wire_format = true; } message Bar { } extend Foo { optional Bar bar = 536870912; }`,
		},
		"success_message_set_wire_format": {
			contents: `message Foo { option message_set_wire_format = true; extensions 1 to 100; }`,
		},
		"failure_message_set_wire_format_in_proto3": {
			contents:    `syntax = "proto3"; message Foo { option message_set_wire_format = true; extensions 1 to 100; }`,
			expectedErr: "test.proto:1:34: messages with message-set wire format are not allowed with proto3 syntax",
		},
		"failure_message_set_wire_format_non_ext_field": {
			contents:    `message Foo { optional double bar = 536870912; option message_set_wire_format = true; }`,
			expectedErr: "test.proto:1:15: messages with message-set wire format cannot contain non-extension fields",
		},
		"failure_message_set_wire_format_no_extension_range": {
			contents:    `message Foo { option message_set_wire_format = true; }`,
			expectedErr: "test.proto:1:15: messages with message-set wire format must contain at least one extension range",
		},
		"success_oneof_w_group": {
			contents: `message Foo { oneof bar { group Baz = 1 [deprecated=true] { optional int32 abc = 1; } } }`,
		},
		"failure_bad_syntax": {
			contents:    `syntax = "proto1";`,
			expectedErr: `test.proto:1:10: syntax value must be "proto2" or "proto3"`,
		},
		"failure_field_number_out_of_range": {
			contents:    `message Foo { optional string s = 5000000000; }`,
			expectedErr: `test.proto:1:35: tag number 5000000000 is higher than max allowed tag number (536870911)`,
		},
		"failure_field_number_reserved": {
			contents:    `message Foo { optional string s = 19500; }`,
			expectedErr: `test.proto:1:35: tag number 19500 is in disallowed reserved range 19000-19999`,
		},
		"failure_enum_value_number_out_of_range": {
			contents:    `enum Foo { V = 5000000000; }`,
			expectedErr: `test.proto:1:16: value 5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_value_number_out_of_range_negative": {
			contents:    `enum Foo { V = -5000000000; }`,
			expectedErr: `test.proto:1:16: value -5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_start_out_of_range": {
			contents:    `enum Foo { V = 0; reserved 5000000000; }`,
			expectedErr: `test.proto:1:28: range start 5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_start_out_of_range_negative": {
			contents:    `enum Foo { V = 0; reserved -5000000000; }`,
			expectedErr: `test.proto:1:28: range start -5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_both_out_of_range": {
			contents:    `enum Foo { V = 0; reserved 5000000000 to 5000000001; }`,
			expectedErr: `test.proto:1:28: range start 5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_end_out_of_range": {
			contents:    `enum Foo { V = 0; reserved 5 to 5000000000; }`,
			expectedErr: `test.proto:1:33: range end 5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_start_out_of_range_negative2": {
			contents:    `enum Foo { V = 0; reserved -5000000000 to -5; }`,
			expectedErr: `test.proto:1:28: range start -5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_both_out_of_range_negative": {
			contents:    `enum Foo { V = 0; reserved -5000000001 to -5000000000; }`,
			expectedErr: `test.proto:1:28: range start -5000000001 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_end_out_of_range_negative": {
			contents:    `enum Foo { V = 0; reserved -5000000000 to 5; }`,
			expectedErr: `test.proto:1:28: range start -5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_reserved_end_out_of_range2": {
			contents:    `enum Foo { V = 0; reserved -5 to 5000000000; }`,
			expectedErr: `test.proto:1:34: range end 5000000000 is out of range: should be between -2147483648 and 2147483647`,
		},
		"failure_enum_without_value": {
			contents:    `enum Foo { }`,
			expectedErr: `test.proto:1:1: enum Foo: enums must define at least one value`,
		},
		"failure_oneof_without_field": {
			contents:    `message Foo { oneof Bar { } }`,
			expectedErr: `test.proto:1:15: oneof must contain at least one field`,
		},
		"failure_extend_without_field": {
			contents:    `message Foo { extensions 1 to max; } extend Foo { }`,
			expectedErr: `test.proto:1:38: extend sections must define at least one extension`,
		},
		"failure_explicit_map_entry_option": {
			contents:    `message Foo { option map_entry = true; }`,
			expectedErr: `test.proto:1:34: message Foo: map_entry option should not be set explicitly; use map type instead`,
		},
		"success_explicit_map_entry_option": {
			// okay if explicit setting is false
			contents: `message Foo { option map_entry = false; }`,
		},
		"failure_proto2_requires_label": {
			contents:    `syntax = "proto2"; message Foo { string s = 1; }`,
			expectedErr: `test.proto:1:41: field Foo.s: field has no label; proto2 requires explicit 'optional' label`,
		},
		"failure_proto2_requires_label2": {
			contents:    `message Foo { string s = 1; }`, // syntax defaults to proto2
			expectedErr: `test.proto:1:22: field Foo.s: field has no label; proto2 requires explicit 'optional' label`,
		},
		"success_proto3_optional": {
			contents: `syntax = "proto3"; message Foo { optional string s = 1; }`,
		},
		"success_proto3_optional_ext": {
			contents: `syntax = "proto3"; import "google/protobuf/descriptor.proto"; extend google.protobuf.MessageOptions { optional string s = 50000; }`,
		},
		"failure_proto3_required": {
			contents:    `syntax = "proto3"; message Foo { required string s = 1; }`,
			expectedErr: `test.proto:1:34: field Foo.s: label 'required' is not allowed in proto3`,
		},
		"failure_extension_required": {
			contents:    `message Foo { extensions 1 to max; } extend Foo { required string sss = 100; }`,
			expectedErr: `test.proto:1:51: field sss: extension fields cannot be 'required'`,
		},
		"failure_proto3_group": {
			contents:    `syntax = "proto3"; message Foo { optional group Grp = 1 { } }`,
			expectedErr: `test.proto:1:43: field Foo.grp: groups are not allowed in proto3`,
		},
		"failure_proto3_extension_range": {
			contents:    `syntax = "proto3"; message Foo { extensions 1 to max; }`,
			expectedErr: `test.proto:1:45: message Foo: extension ranges are not allowed in proto3`,
		},
		"failure_proto3_detault": {
			contents:    `syntax = "proto3"; message Foo { string s = 1 [default = "abcdef"]; }`,
			expectedErr: `test.proto:1:48: field Foo.s: default values are not allowed in proto3`,
		},
		"failure_enum_value_number_duplicate": {
			contents:    `enum Foo { V1 = 1; V2 = 1; }`,
			expectedErr: `test.proto:1:25: enum Foo: values V1 and V2 both have the same numeric value 1; use allow_alias option if intentional`,
		},
		"success_enum_allow_alias_true": {
			contents: `enum Foo { option allow_alias = true; V1 = 1; V2 = 1; }`,
		},
		"success_enum_allow_alias_false": {
			contents: `enum Foo { option allow_alias = false; V1 = 1; V2 = 2; }`,
		},
		"failure_enum_allow_alias": {
			contents:    `enum Foo { option allow_alias = true; V1 = 1; V2 = 2; }`,
			expectedErr: `test.proto:1:33: enum Foo: allow_alias is true but no values are aliases`,
		},
		"success_enum_reserved": {
			contents: `syntax = "proto3"; enum Foo { V1 = 0; reserved 1 to 20; reserved "V2"; }`,
		},
		"failure_enum_value_in_reserved_range": {
			contents:    `enum Foo { V1 = 1; reserved 1 to 20; reserved "V2"; }`,
			expectedErr: `test.proto:1:17: enum Foo: value V1 is using number 1 which is in reserved range 1 to 20`,
		},
		"failure_enum_value_in_reserved_range2": {
			contents:    `enum Foo { V1 = 20; reserved 1 to 20; reserved "V2"; }`,
			expectedErr: `test.proto:1:17: enum Foo: value V1 is using number 20 which is in reserved range 1 to 20`,
		},
		"failure_enum_value_w_reserved_name": {
			contents:    `enum Foo { V2 = 0; reserved 1 to 20; reserved "V2"; }`,
			expectedErr: `test.proto:1:12: enum Foo: value V2 is using a reserved name`,
		},
		"success_enum_reserved2": {
			contents: `enum Foo { V0 = 0; reserved 1 to 20; reserved 21 to 40; reserved "V2"; }`,
		},
		"failure_enum_reserved_overlap": {
			contents:    `enum Foo { V0 = 0; reserved 1 to 20; reserved 20 to 40; reserved "V2"; }`,
			expectedErr: `test.proto:1:47: enum Foo: reserved ranges overlap: 1 to 20 and 20 to 40`,
		},
		"failure_proto3_enum_zero_value": {
			contents:    `syntax = "proto3"; enum Foo { FIRST = 1; }`,
			expectedErr: `test.proto:1:39: enum Foo: proto3 requires that first value in enum have numeric value of 0`,
		},
		"failure_message_number_conflict": {
			contents:    `syntax = "proto3"; message Foo { string s = 1; int32 i = 1; }`,
			expectedErr: `test.proto:1:58: message Foo: fields s and i both have the same tag 1`,
		},
		"failure_message_reserved_overlap": {
			contents:    `message Foo { reserved 1 to 10, 10 to 12; }`,
			expectedErr: `test.proto:1:33: message Foo: reserved ranges overlap: 1 to 10 and 10 to 12`,
		},
		"failure_message_extensions_overlap": {
			contents:    `message Foo { extensions 1 to 10, 10 to 12; }`,
			expectedErr: `test.proto:1:35: message Foo: extension ranges overlap: 1 to 10 and 10 to 12`,
		},
		"failure_message_reserved_extensions_overlap": {
			contents:    `message Foo { reserved 1 to 10; extensions 10 to 12; }`,
			expectedErr: `test.proto:1:44: message Foo: extension range 10 to 12 overlaps reserved range 1 to 10`,
		},
		"success_message_reserved_extensions": {
			contents: `message Foo { reserved 1, 2 to 10, 11 to 20; extensions 21 to 22; }`,
		},
		"failure_message_reserved_start_after_end": {
			contents:    `message Foo { reserved 10 to 1; }`,
			expectedErr: `test.proto:1:24: range, 10 to 1, is invalid: start must be <= end`,
		},
		"failure_message_extensions_start_after_end": {
			contents:    `message Foo { extensions 10 to 1; }`,
			expectedErr: `test.proto:1:26: range, 10 to 1, is invalid: start must be <= end`,
		},
		"failure_message_reserved_end_out_of_range": {
			contents:    `message Foo { reserved 1 to 5000000000; }`,
			expectedErr: `test.proto:1:29: range end 5000000000 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_reserved_start_out_of_range": {
			contents:    `message Foo { reserved 0 to 10; }`,
			expectedErr: `test.proto:1:24: range start 0 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_extensions_start_out_of_range": {
			contents:    `message Foo { extensions 3000000000; }`,
			expectedErr: `test.proto:1:26: range start 3000000000 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_extensions_both_out_of_range": {
			contents:    `message Foo { extensions 3000000000 to 3000000001; }`,
			expectedErr: `test.proto:1:26: range start 3000000000 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_extensions_start_out_of_range2": {
			contents:    `message Foo { extensions 0 to 10; }`,
			expectedErr: `test.proto:1:26: range start 0 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_extensions_end_out_of_range": {
			contents:    `message Foo { extensions 100 to 3000000000; }`,
			expectedErr: `test.proto:1:33: range end 3000000000 is out of range: should be between 1 and 536870911`,
		},
		"failure_message_reserved_name_duplicate": {
			contents:    `message Foo { reserved "foo", "foo"; }`,
			expectedErr: `test.proto:1:31: name "foo" is reserved multiple times`,
		},
		"failure_message_reserved_name_duplicate2": {
			contents:    `message Foo { reserved "foo"; reserved "foo"; }`,
			expectedErr: `test.proto:1:40: name "foo" is reserved multiple times`,
		},
		"failure_message_field_w_reserved_name": {
			contents:    `message Foo { reserved "foo"; optional string foo = 1; }`,
			expectedErr: `test.proto:1:47: message Foo: field foo is using a reserved name`,
		},
		"failure_message_field_w_reserved_number": {
			contents:    `message Foo { reserved 1 to 10; optional string foo = 1; }`,
			expectedErr: `test.proto:1:55: message Foo: field foo is using tag 1 which is in reserved range 1 to 10`,
		},
		"failure_message_field_w_number_in_ext_range": {
			contents:    `message Foo { extensions 1 to 10; optional string foo = 1; }`,
			expectedErr: `test.proto:1:57: message Foo: field foo is using tag 1 which is in extension range 1 to 10`,
		},
		"failure_group_name": {
			contents:    `message Foo { optional group foo = 1 { } }`,
			expectedErr: `test.proto:1:30: group foo should have a name that starts with a capital letter`,
		},
		"failure_oneof_group_name": {
			contents:    `message Foo { oneof foo { group bar = 1 { } } }`,
			expectedErr: `test.proto:1:33: group bar should have a name that starts with a capital letter`,
		},
		"failure_message_decl_start_w_option": {
			contents:    `enum Foo { option = 1; }`,
			expectedErr: `test.proto:1:19: syntax error: unexpected '='`,
		},
		"failure_message_decl_start_w_reserved": {
			contents:    `enum Foo { reserved = 1; }`,
			expectedErr: `test.proto:1:21: syntax error: unexpected '=', expecting string literal or int literal or '-'`,
		},
		"failure_message_decl_start_w_message": {
			contents:    `syntax = "proto3"; enum message { unset = 0; } message Foo { message bar = 1; }`,
			expectedErr: `test.proto:1:74: syntax error: unexpected '=', expecting '{'`,
		},
		"failure_message_decl_start_w_enum": {
			contents:    `syntax = "proto3"; enum enum { unset = 0; } message Foo { enum bar = 1; }`,
			expectedErr: `test.proto:1:68: syntax error: unexpected '=', expecting '{'`,
		},
		"failure_message_decl_start_w_reserved2": {
			contents:    `syntax = "proto3"; enum reserved { unset = 0; } message Foo { reserved bar = 1; }`,
			expectedErr: `test.proto:1:72: syntax error: unexpected identifier, expecting string literal or int literal`,
		},
		"failure_message_decl_start_w_extend": {
			contents:    `syntax = "proto3"; enum extend { unset = 0; } message Foo { extend bar = 1; }`,
			expectedErr: `test.proto:1:72: syntax error: unexpected '=', expecting '{'`,
		},
		"failure_message_decl_start_w_oneof": {
			contents:    `syntax = "proto3"; enum oneof { unset = 0; } message Foo { oneof bar = 1; }`,
			expectedErr: `test.proto:1:70: syntax error: unexpected '=', expecting '{'`,
		},
		"failure_message_decl_start_w_optional": {
			contents:    `syntax = "proto3"; enum optional { unset = 0; } message Foo { optional bar = 1; }`,
			expectedErr: `test.proto:1:76: syntax error: unexpected '='`,
		},
		"failure_message_decl_start_w_repeated": {
			contents:    `syntax = "proto3"; enum repeated { unset = 0; } message Foo { repeated bar = 1; }`,
			expectedErr: `test.proto:1:76: syntax error: unexpected '='`,
		},
		"failure_message_decl_start_w_required": {
			contents:    `syntax = "proto3"; enum required { unset = 0; } message Foo { required bar = 1; }`,
			expectedErr: `test.proto:1:76: syntax error: unexpected '='`,
		},
		"failure_extend_decl_start_w_optional": {
			contents:    `syntax = "proto3"; import "google/protobuf/descriptor.proto"; enum optional { unset = 0; } extend google.protobuf.MethodOptions { optional bar = 22222; }`,
			expectedErr: `test.proto:1:144: syntax error: unexpected '='`,
		},
		"failure_extend_decl_start_w_repeated": {
			contents:    `syntax = "proto3"; import "google/protobuf/descriptor.proto"; enum repeated { unset = 0; } extend google.protobuf.MethodOptions { repeated bar = 22222; }`,
			expectedErr: `test.proto:1:144: syntax error: unexpected '='`,
		},
		"failure_extend_decl_start_w_required": {
			contents:    `syntax = "proto3"; import "google/protobuf/descriptor.proto"; enum required { unset = 0; } extend google.protobuf.MethodOptions { required bar = 22222; }`,
			expectedErr: `test.proto:1:144: syntax error: unexpected '='`,
		},
		"failure_oneof_decl_start_w_optional": {
			contents:    `syntax = "proto3"; enum optional { unset = 0; } message Foo { oneof bar { optional bar = 1; } }`,
			expectedErr: `test.proto:1:75: syntax error: unexpected "optional"`,
		},
		"failure_oneof_decl_start_w_repeated": {
			contents:    `syntax = "proto3"; enum repeated { unset = 0; } message Foo { oneof bar { repeated bar = 1; } }`,
			expectedErr: `test.proto:1:75: syntax error: unexpected "repeated"`,
		},
		"failure_oneof_decl_start_w_required": {
			contents:    `syntax = "proto3"; enum required { unset = 0; } message Foo { oneof bar { required bar = 1; } }`,
			expectedErr: `test.proto:1:75: syntax error: unexpected "required"`,
		},
		"success_empty": {
			contents: ``,
		},
		"failure_junk_token": {
			contents:    `0`,
			expectedErr: `test.proto:1:1: syntax error: unexpected int literal`,
		},
		"failure_junk_token2": {
			contents:    `foobar`,
			expectedErr: `test.proto:1:1: syntax error: unexpected identifier`,
		},
		"failure_junk_token3": {
			contents:    `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`,
			expectedErr: `test.proto:1:1: syntax error: unexpected identifier`,
		},
		"failure_junk_token4": {
			contents:    `"abc"`,
			expectedErr: `test.proto:1:1: syntax error: unexpected string literal`,
		},
		"failure_junk_token5": {
			contents:    `0.0.0.0.0`,
			expectedErr: `test.proto:1:1: invalid syntax in float value: 0.0.0.0.0`,
		},
		"failure_junk_token6": {
			contents:    `0.0`,
			expectedErr: `test.proto:1:1: syntax error: unexpected float literal`,
		},
		"success_colon_before_list_literal": {
			contents: `option (opt) = {m: [{key: "a",value: {}}]};`,
		},
		"success_no_colon_before_list_literal": {
			contents: `option (opt) = {m [{key: "a",value: {}}]};`,
		},
		"success_colon_before_list_literal2": {
			contents: `option (opt) = {m: []};`,
		},
		"success_no_colon_before_list_literal2": {
			contents: `option (opt) = {m []};`,
		},
		"failure_duplicate_import": {
			contents:    `syntax = "proto3"; import "google/protobuf/descriptor.proto"; import "google/protobuf/descriptor.proto";`,
			expectedErr: `test.proto:1:63: "google/protobuf/descriptor.proto" was already imported at test.proto:1:20`,
		},
		"success_long_package_name": {
			contents: `syntax = "proto3"; package a012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789;`,
		},
		"failure_long_package_name": {
			contents:    `syntax = "proto3"; package ab012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789;`,
			expectedErr: `test.proto:1:28: package name (with whitespace removed) must be less than 512 characters long`,
		},
		"success_long_package_name2": {
			contents: `syntax = "proto3"; package a .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789;`,
		},
		"failure_long_package_name2": {
			contents:    `syntax = "proto3"; package ab .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789 .  a23456789;`,
			expectedErr: `test.proto:1:28: package name (with whitespace removed) must be less than 512 characters long`,
		},
		"success_long_package_name3": {
			contents: `syntax = "proto3"; package a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1;`,
		},
		"failure_long_package_name3": {
			contents:    `syntax = "proto3"; package a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2.a3.a4.a5.a6.a7.a8.a9.a0.a1.a2;`,
			expectedErr: `test.proto:1:28: package name may not contain more than 100 periods`,
		},
		"success_deep_nesting": {
			contents: `syntax = "proto3";
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
		},
		"failure_deep_nesting_message1": {
			contents: `syntax = "proto3";
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 { message _32 {
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
			expectedErr: `test.proto:9:86: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_message2": {
			contents: `syntax = "proto3";
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 { message _32 {
					   message _33 { }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
			expectedErr: `test.proto:9:86: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_map": {
			contents: `syntax = "proto3";
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					     map<string, string> m = 1;
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
			expectedErr: `test.proto:10:46: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_group1": {
			contents: `syntax = "proto2";
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					     optional group Foo = 1 { }
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
			expectedErr: `test.proto:10:55: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_group2": {
			contents: `syntax = "proto2";
					   message _01 { optional group Foo = 1 {
					   message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } }
					   } }`,
			expectedErr: `test.proto:10:72: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_extension_group1": {
			contents: `syntax = "proto2";
					   message Ext { extensions 1 to max; }
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					     extend Ext {
					       optional group Foo = 1 { }
					     }
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }`,
			expectedErr: `test.proto:12:57: message nesting depth must be less than 32`,
		},
		"failure_deep_nesting_extension_group2": {
			contents: `syntax = "proto2";
					   message Ext { extensions 1 to max; }
					   extend Ext { optional group Foo = 1 {
					   message _01 { message _02 { message _03 { message _04 {
					   message _05 { message _06 { message _07 { message _08 {
					   message _09 { message _10 { message _11 { message _12 {
					   message _13 { message _14 { message _15 { message _16 {
					   message _17 { message _18 { message _19 { message _20 {
					   message _21 { message _22 { message _23 { message _24 {
					   message _25 { message _26 { message _27 { message _28 {
					   message _29 { message _30 { message _31 {
					   } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } } } }
					   } }`,
			expectedErr: `test.proto:11:72: message nesting depth must be less than 32`,
		},
		"failure_message_invalid_reserved_name": {
			contents: `syntax = "proto3";
					   message Foo {
					     reserved "foo", "b_a_r9", " blah ";
					   }`,
			expectedErr: `test.proto:3:72: message Foo: reserved name " blah " is not a valid identifier`,
		},
		"failure_message_invalid_reserved_name2": {
			contents: `syntax = "proto3";
					   message Foo {
					     reserved "foo", "_bar123", "123";
					   }`,
			expectedErr: `test.proto:3:73: message Foo: reserved name "123" is not a valid identifier`,
		},
		"failure_message_invalid_reserved_name3": {
			contents: `syntax = "proto3";
					   message Foo {
					     reserved "foo" "_bar123" "@y!!";
					   }`,
			expectedErr: `test.proto:3:55: message Foo: reserved name "foo_bar123@y!!" is not a valid identifier`,
		},
		"failure_message_invalid_reserved_name4": {
			contents: `syntax = "proto3";
					   message Foo {
					     reserved "";
					   }`,
			expectedErr: `test.proto:3:55: message Foo: reserved name "" is not a valid identifier`,
		},
		"success_message_reserved_name": {
			contents: `syntax = "proto3";
					   message Foo {
					     reserved "foo", "_bar123", "A_B_C_1_2_3";
					   }`,
		},
		"failure_enum_invalid_reserved_name": {
			contents: `syntax = "proto3";
					   enum Foo {
					     BAR = 0;
					     reserved "foo", "b_a_r9", " blah ";
					   }`,
			expectedErr: `test.proto:4:72: enum Foo: reserved name " blah " is not a valid identifier`,
		},
		"failure_enum_invalid_reserved_name2": {
			contents: `syntax = "proto3";
					   enum Foo {
					     BAR = 0;
					     reserved "foo", "_bar123", "123";
					   }`,
			expectedErr: `test.proto:4:73: enum Foo: reserved name "123" is not a valid identifier`,
		},
		"failure_enum_invalid_reserved_name3": {
			contents: `syntax = "proto3";
					   enum Foo {
					     BAR = 0;
					     reserved "foo" "_bar123" "@y!!";
					   }`,
			expectedErr: `test.proto:4:55: enum Foo: reserved name "foo_bar123@y!!" is not a valid identifier`,
		},
		"failure_enum_invalid_reserved_name4": {
			contents: `syntax = "proto3";
					   enum Foo {
					     BAR = 0;
					     reserved "";
					   }`,
			expectedErr: `test.proto:4:55: enum Foo: reserved name "" is not a valid identifier`,
		},
		"success_enum_reserved_name": {
			contents: `syntax = "proto3";
					   enum Foo {
					     BAR = 0;
					     reserved "foo", "_bar123", "A_B_C_1_2_3";
					   }`,
		},
	}

	for name, tc := range testCases {
		tc := tc
		expectedPrefix := "success_"
		if tc.expectedErr != "" {
			expectedPrefix = "failure_"
		}
		assert.Truef(t, strings.HasPrefix(name, expectedPrefix), "expected test name %q to have %q prefix", name, expectedPrefix)

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			errs := reporter.NewHandler(nil)
			if ast, err := Parse("test.proto", strings.NewReader(tc.contents), errs); err == nil {
				_, _ = ResultFromAST(ast, true, errs)
			}

			err := errs.Error()
			if tc.expectedErr == "" {
				assert.NoError(t, err, "should succeed")
			} else if assert.NotNil(t, err, "should fail") {
				assert.Equal(t, tc.expectedErr, err.Error(), "bad error message")
			}
		})
	}
}
