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

package linker_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/reporter"
)

func TestSimpleLink(t *testing.T) {
	t.Parallel()
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		}),
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_complex.proto")
	if !assert.Nil(t, err) {
		return
	}

	res, ok := fds[0].(linker.Result)
	require.True(t, ok)
	fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/desc_test_complex.protoset", linker.ResolverFromFile(fds[0]))
	prototest.CheckFiles(t, res, fdset, true)
}

func TestMultiFileLink(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"desc_test_defaults.proto", "desc_test_field_types.proto", "desc_test_options.proto", "desc_test_wellknowntypes.proto"} {
		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				ImportPaths: []string{"../internal/testdata"},
			}),
		}
		fds, err := compiler.Compile(context.Background(), name)
		if !assert.Nil(t, err) {
			continue
		}

		res, ok := fds[0].(linker.Result)
		require.True(t, ok)
		fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/all.protoset", linker.ResolverFromFile(fds[0]))
		prototest.CheckFiles(t, res, fdset, true)
	}
}

func TestProto3Optional(t *testing.T) {
	t.Parallel()
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		}),
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_proto3_optional.proto")
	if !assert.Nil(t, err) {
		return
	}

	fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/desc_test_proto3_optional.protoset", fds.AsResolver())

	res, ok := fds[0].(linker.Result)
	require.True(t, ok)
	prototest.CheckFiles(t, res, fdset, true)
}

func TestLinkerValidation(t *testing.T) {
	t.Parallel()
	testCases := map[string]struct {
		input map[string]string
		// Expected error message - leave empty if input is expected to succeed
		expectedErr string
	}{
		"success_multi_namespace": {
			input: map[string]string{
				"foo.proto":  `syntax = "proto3"; package namespace.a; import "foo2.proto"; import "foo3.proto"; import "foo4.proto"; message Foo{ b.Bar a = 1; b.Baz b = 2; b.Buzz c = 3; }`,
				"foo2.proto": `syntax = "proto3"; package namespace.b; message Bar{}`,
				"foo3.proto": `syntax = "proto3"; package namespace.b; message Baz{}`,
				"foo4.proto": `syntax = "proto3"; package namespace.b; message Buzz{}`,
			},
		},
		"failure_missing_import": {
			input: map[string]string{
				"foo.proto": `import "foo2.proto"; message fubar{}`,
			},
			expectedErr: `foo.proto:1:8: file not found: foo2.proto`,
		},
		"failure_import_cycle": {
			input: map[string]string{
				"foo.proto":  `import "foo2.proto"; message fubar{}`,
				"foo2.proto": `import "foo.proto"; message baz{}`,
			},
			// since files are compiled concurrently, there are two possible outcomes
			expectedErr: `foo.proto:1:8: cycle found in imports: "foo.proto" -> "foo2.proto" -> "foo.proto"` +
				` || foo2.proto:1:8: cycle found in imports: "foo2.proto" -> "foo.proto" -> "foo2.proto"`,
		},
		"failure_enum_cpp_scope": {
			input: map[string]string{
				"foo.proto": "enum foo { bar = 1; baz = 2; } enum fu { bar = 1; baz = 2; }",
			},
			expectedErr: `foo.proto:1:42: symbol "bar" already defined at foo.proto:1:12; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum`,
		},
		"failure_redefined_symbol": {
			input: map[string]string{
				"foo.proto": "message foo {} enum foo { V = 0; }",
			},
			expectedErr: `foo.proto:1:21: symbol "foo" already defined at foo.proto:1:9`,
		},
		"failure_duplicate_field_name": {
			input: map[string]string{
				"foo.proto": "message foo { optional string a = 1; optional string a = 2; }",
			},
			expectedErr: `foo.proto:1:54: symbol "foo.a" already defined at foo.proto:1:31`,
		},
		"failure_duplicate_symbols": {
			input: map[string]string{
				"foo.proto":  "message foo {}",
				"foo2.proto": "enum foo { V = 0; }",
			},
			// since files are compiled concurrently, there are two possible outcomes
			expectedErr: `foo.proto:1:9: symbol "foo" already defined at foo2.proto:1:6` +
				` || foo2.proto:1:6: symbol "foo" already defined at foo.proto:1:9`,
		},
		"failure_unsupported_type": {
			input: map[string]string{
				"foo.proto": "message foo { optional blah a = 1; }",
			},
			expectedErr: "foo.proto:1:24: field foo.a: unknown type blah",
		},
		"failure_invalid_method_field": {
			input: map[string]string{
				"foo.proto": "message foo { optional bar.baz a = 1; } service bar { rpc baz (foo) returns (foo); }",
			},
			expectedErr: "foo.proto:1:24: field foo.a: invalid type: bar.baz is a method, not a message or enum",
		},
		"failure_duplicate_extension": {
			input: map[string]string{
				"foo.proto": "message foo { extensions 1 to 2; } extend foo { optional string a = 1; } extend foo { optional int32 b = 1; }",
			},
			expectedErr: "foo.proto:1:106: extension with tag 1 for message foo already defined at foo.proto:1:69",
		},
		"failure_unknown_extendee": {
			input: map[string]string{
				"foo.proto": "package fu.baz; extend foobar { optional string a = 1; }",
			},
			expectedErr: "foo.proto:1:24: unknown extendee type foobar",
		},
		"failure_extend_service": {
			input: map[string]string{
				"foo.proto": "package fu.baz; service foobar{} extend foobar { optional string a = 1; }",
			},
			expectedErr: "foo.proto:1:41: extendee is invalid: fu.baz.foobar is a service, not a message",
		},
		"failure_conflict_method_message_input": {
			input: map[string]string{
				"foo.proto": "message foo{} message bar{} service foobar{ rpc foo(foo) returns (bar); }",
			},
			expectedErr: "foo.proto:1:53: method foobar.foo: invalid request type: foobar.foo is a method, not a message",
		},
		"failure_conflict_method_message_output": {
			input: map[string]string{
				"foo.proto": "message foo{} message bar{} service foobar{ rpc foo(bar) returns (foo); }",
			},
			expectedErr: "foo.proto:1:67: method foobar.foo: invalid response type: foobar.foo is a method, not a message",
		},
		"failure_invalid_extension_field": {
			input: map[string]string{
				"foo.proto": "package fu.baz; message foobar{ extensions 1; } extend foobar { optional string a = 2; }",
			},
			expectedErr: "foo.proto:1:85: extension fu.baz.a: tag 2 is not in valid range for extended type fu.baz.foobar",
		},
		"failure_unknown_type": {
			input: map[string]string{
				"foo.proto":  `package fu.baz; import public "foo2.proto"; message foobar{ optional baz a = 1; }`,
				"foo2.proto": `package fu.baz; import "foo3.proto"; message fizzle{ }`,
				"foo3.proto": "package fu.baz; message baz{ }",
			},
			expectedErr: "foo.proto:1:70: field fu.baz.foobar.a: unknown type baz; resolved to fu.baz which is not defined; consider using a leading dot",
		},
		"success_extension_types": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					package foo;
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.FileOptions           { optional string fil_foo = 12000; }
					extend google.protobuf.MessageOptions        { optional string msg_foo = 12000; }
					extend google.protobuf.FieldOptions          { optional string fld_foo = 12000 [(fld_foo) = "extension"]; }
					extend google.protobuf.OneofOptions          { optional string oof_foo = 12000; }
					extend google.protobuf.EnumOptions           { optional string enm_foo = 12000; }
					extend google.protobuf.EnumValueOptions      { optional string env_foo = 12000; }
					extend google.protobuf.ExtensionRangeOptions { optional string ext_foo = 12000; }
					extend google.protobuf.ServiceOptions        { optional string svc_foo = 12000; }
					extend google.protobuf.MethodOptions         { optional string mtd_foo = 12000; }
					option (fil_foo) = "file";
					message Foo {
						option (msg_foo) = "message";
						oneof foo {
							option (oof_foo) = "oneof";
							string bar = 1 [(fld_foo) = "field"];
						}
						extensions 100 to 200 [(ext_foo) = "extensionrange"];
					}
					enum Baz {
						option (enm_foo) = "enum";
						ZERO = 0 [(env_foo) = "enumvalue"];
					}
					service FooService {
						option (svc_foo) = "service";
						rpc Bar(Foo) returns (Foo) {
							option (mtd_foo) = "method";
						}
					}`,
			},
		},
		"failure_default_repeated": {
			input: map[string]string{
				"foo.proto": `package fu.baz; message foobar{ repeated string a = 1 [default = "abc"]; }`,
			},
			expectedErr: "foo.proto:1:56: field fu.baz.foobar.a: default value cannot be set because field is repeated",
		},
		"failure_default_message": {
			input: map[string]string{
				"foo.proto": "package fu.baz; message foobar{ optional foobar a = 1 [default = { a: {} }]; }",
			},
			expectedErr: "foo.proto:1:56: field fu.baz.foobar.a: default value cannot be set because field is a message",
		},
		"failure_default_string_message": {
			input: map[string]string{
				"foo.proto": `package fu.baz; message foobar{ optional string a = 1 [default = { a: "abc" }]; }`,
			},
			expectedErr: "foo.proto:1:66: field fu.baz.foobar.a: default value cannot be a message",
		},
		"failure_string_default_double": {
			input: map[string]string{
				"foo.proto": "package fu.baz; message foobar{ optional string a = 1 [default = 1.234]; }",
			},
			expectedErr: "foo.proto:1:66: field fu.baz.foobar.a: option default: expecting string, got double",
		},
		"failure_enum_default_not_found": {
			input: map[string]string{
				"foo.proto": "package fu.baz; enum abc { OK=0; NOK=1; } message foobar{ optional abc a = 1 [default = NACK]; }",
			},
			expectedErr: "foo.proto:1:89: field fu.baz.foobar.a: option default: enum fu.baz.abc has no value named NACK",
		},
		"failure_unknown_file_option": {
			input: map[string]string{
				"foo.proto": "option b = 123;",
			},
			expectedErr: "foo.proto:1:8: option b: field b of google.protobuf.FileOptions does not exist",
		},
		"failure_unknown_extension": {
			input: map[string]string{
				"foo.proto": "option (foo.bar) = 123;",
			},
			expectedErr: "foo.proto:1:8: unknown extension foo.bar",
		},
		"failure_invalid_option": {
			input: map[string]string{
				"foo.proto": "option uninterpreted_option = { };",
			},
			expectedErr: "foo.proto:1:8: invalid option 'uninterpreted_option'",
		},
		"failure_option_unknown_field": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f).b = 123;`,
			},
			expectedErr: "foo.proto:5:12: option (f).b: field b of foo does not exist",
		},
		"failure_option_wrong_type": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f).a = 123;`,
			},
			expectedErr: "foo.proto:5:16: option (f).a: expecting string, got integer",
		},
		"failure_extension_message_not_file": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (b) = 123;`,
			},
			expectedErr: "foo.proto:5:8: option (b): extension b should extend google.protobuf.FileOptions but instead extends foo",
		},
		"failure_option_message_not_extension": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (foo) = 123;`,
			},
			expectedErr: "foo.proto:5:8: invalid extension: foo is a message, not an extension",
		},
		"failure_option_field_not_extension": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (foo.a) = 123;`,
			},
			expectedErr: "foo.proto:5:8: invalid extension: foo.a is a field but not an extension",
		},
		"failure_option_not_repeated": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: [ 123 ] };`,
			},
			expectedErr: "foo.proto:5:19: option (f): value is an array but field is not repeated",
		},
		"failure_option_repeated_string_integer": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { repeated string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: [ "a", "b", 123 ] };`,
			},
			expectedErr: "foo.proto:5:31: option (f): expecting string, got integer",
		},
		"failure_option_non_repeated_override": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: "a" };
					option (f) = { a: "b" };`,
			},
			expectedErr: "foo.proto:6:8: option (f): non-repeated option field (f) already set",
		},
		"failure_option_non_repeated_override2": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: "a" };
					option (f).a = "b";`,
			},
			expectedErr: "foo.proto:6:12: option (f).a: non-repeated option field a already set",
		},
		"failure_option_int32_not_string": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { optional string a = 1; extensions 10 to 20; }
					extend foo { optional int32 b = 10; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: "a" };
					option (f).(b) = "b";`,
			},
			expectedErr: "foo.proto:6:18: option (f).(b): expecting int32, got string",
		},
		"failure_option_required_field_unset": {
			input: map[string]string{
				"foo.proto": `
					import "google/protobuf/descriptor.proto";
					message foo { required string a = 1; required string b = 2; }
					extend google.protobuf.FileOptions { optional foo f = 20000; }
					option (f) = { a: "a" };`,
			},
			expectedErr: "foo.proto:1:1: error in file options: some required fields missing: (f).b",
		},
		"failure_message_set_wire_format_scalar": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional int32 bar = 1; }",
			},
			expectedErr: "foo.proto:1:99: messages with message-set wire format cannot contain scalar extensions, only messages",
		},
		"success_message_set_wire_format": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional Foo bar = 1; }",
			},
		},
		"failure_tag_out_of_range": {
			input: map[string]string{
				"foo.proto": "message Foo { extensions 1 to max; } extend Foo { optional int32 bar = 536870912; }",
			},
			expectedErr: "foo.proto:1:72: extension bar: tag 536870912 is not in valid range for extended type Foo",
		},
		"success_tag_message_set_wire_format": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to max; } extend Foo { optional Foo bar = 536870912; }",
			},
		},
		"failure_message_set_wire_format_scalar2": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional int32 bar = 1; }",
			},
			expectedErr: "foo.proto:1:99: messages with message-set wire format cannot contain scalar extensions, only messages",
		},
		"success_message_set_wire_format2": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional Foo bar = 1; }",
			},
		},
		"failure_message_set_wire_format_repeated": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { repeated Foo bar = 1; }",
			},
			expectedErr: "foo.proto:1:90: messages with message-set wire format cannot contain repeated extensions, only optional",
		},
		"success_large_extension_message_set_wire_format": {
			input: map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to max; } extend Foo { optional Foo bar = 536870912; }",
			},
		},
		"failure_string_value_leading_dot": {
			input: map[string]string{
				"foo.proto": `syntax = "proto3"; package com.google; import "google/protobuf/wrappers.proto"; message Foo { google.protobuf.StringValue str = 1; }`,
			},
			expectedErr: "foo.proto:1:95: field com.google.Foo.str: unknown type google.protobuf.StringValue; resolved to com.google.protobuf.StringValue which is not defined; consider using a leading dot",
		},
		"success_group_message_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo {
					  optional group Bar = 1 { optional string name = 1; }
					}
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo).bar.name = "abc"; }`,
			},
		},
		"failure_group_extension_not_exist": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo {
					  optional group Bar = 1 { optional string name = 1; }
					}
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo).Bar.name = "abc"; }`,
			},
			expectedErr: "foo.proto:7:28: message Baz: option (foo).Bar.name: field Bar of Foo does not exist",
		},
		"success_group_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions {
					  optional group Foo = 10001 { optional string name = 1; }
					}
					message Bar { option (foo).name = "abc"; }`,
			},
		},
		"failure_group_not_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions {
					  optional group Foo = 10001 { optional string name = 1; }
					}
					message Bar { option (Foo).name = "abc"; }`,
			},
			expectedErr: "foo.proto:6:22: message Bar: invalid extension: Foo is a message, not an extension",
		},
		"success_group_custom_option": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo {
					  optional group Bar = 1 { optional string name = 1; }
					}
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo) = { Bar< name: "abc" > }; }`,
			},
		},
		"failure_group_custom_option": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo {
					  optional group Bar = 1 { optional string name = 1; }
					}
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo) = { bar< name: "abc" > }; }`,
			},
			expectedErr: "foo.proto:7:32: message Baz: option (foo): field bar not found (did you mean the group named Bar?)",
		},
		"success_group_custom_option2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo { extensions 1 to 10; }
					extend Foo { optional group Bar = 10 { optional string name = 1; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo) = { [bar]< name: "abc" > }; }`,
			},
		},
		"failure_group_extension_field_not_found": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo { extensions 1 to 10; }
					extend Foo { optional group Bar = 10 { optional string name = 1; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo) = { [Bar]< name: "abc" > }; }`,
			},
			expectedErr: "foo.proto:6:33: message Baz: option (foo): invalid extension: Bar is a message, not an extension",
		},
		"failure_oneof_extension_already_set": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { oneof bar { string baz = 1; string buzz = 2; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz { option (foo) = { baz: "abc" buzz: "xyz" }; }`,
			},
			expectedErr: `foo.proto:5:43: message Baz: option (foo): oneof "bar" already has field "baz" set`,
		},
		"failure_oneof_extension_already_set2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { oneof bar { string baz = 1; string buzz = 2; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz {
					  option (foo).baz = "abc";
					  option (foo).buzz = "xyz";
					}`,
			},
			expectedErr: `foo.proto:7:16: message Baz: option (foo).buzz: oneof "bar" already has field "baz" set`,
		},
		"failure_oneof_extension_already_set3": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { oneof bar { google.protobuf.DescriptorProto baz = 1; google.protobuf.DescriptorProto buzz = 2; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz {
					  option (foo).baz.name = "abc";
					  option (foo).buzz.name = "xyz";
					}`,
			},
			expectedErr: `foo.proto:7:16: message Baz: option (foo).buzz.name: oneof "bar" already has field "baz" set`,
		},
		"failure_oneof_extension_already_set4": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { oneof bar { google.protobuf.DescriptorProto baz = 1; google.protobuf.DescriptorProto buzz = 2; } }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz {
					  option (foo).baz.options.(foo).baz.name = "abc";
					  option (foo).baz.options.(foo).buzz.name = "xyz";
					}`,
			},
			expectedErr: `foo.proto:7:34: message Baz: option (foo).baz.options.(foo).buzz.name: oneof "bar" already has field "baz" set`,
		},
		"success_repeated_extensions": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { repeated string strs = 1; repeated Foo foos = 2; }
					extend google.protobuf.FileOptions { optional Foo foo = 10001; }
					option (foo) = {
					  strs: []
					  foos []
					};`,
			},
		},
		"failure_repeated_primitive_no_leading_colon": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { repeated string strs = 1; repeated Foo foos = 2; }
					extend google.protobuf.FileOptions { optional Foo foo = 10001; }
					option (foo) = {
					  strs []
					  foos []
					};`,
			},
			expectedErr: `foo.proto:6:8: syntax error: unexpected value, expecting ':'`,
		},
		"success_extension_repeated_field_values": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { repeated string strs = 1; repeated Foo foos = 2; }
					extend google.protobuf.FileOptions { optional Foo foo = 10001; }
					option (foo) = {
					  strs: ['abc', 'def']
					  foos [<strs:'foo'>, <strs:'bar'>]
					};`,
			},
		},
		"failure_extension_unexpected_string_literal": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { repeated string strs = 1; repeated Foo foos = 2; }
					extend google.protobuf.FileOptions { optional Foo foo = 10001; }
					option (foo) = {
					  strs ['abc', 'def']
					  foos [<strs:'foo'>, <strs:'bar'>]
					};`,
			},
			expectedErr: `foo.proto:6:9: syntax error: unexpected string literal, expecting '{' or '<' or ']'`,
		},
		"failure_extension_enum_value_not_message": {
			input: map[string]string{
				"foo.proto": `
					package foo.bar;
					message M {
					  enum E { M = 0; }
					  optional M F1 = 1;
					  extensions 2 to 2;
					  extend M { optional string F2 = 2; }
					}`,
			},
			expectedErr: `foo.proto:6:10: extendee is invalid: foo.bar.M.M is an enum value, not a message`,
		},
		"failure_json_name_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions {
					  string foobar = 10001 [json_name="FooBar"];
					}`,
			},
			expectedErr: "foo.proto:4:26: field foobar: option json_name is not allowed on extensions",
		},
		"failure_json_name_looks_like_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string foobar = 10001 [json_name="[FooBar]"];
					}`,
			},
			expectedErr: "foo.proto:3:36: field Foo.foobar: option json_name value cannot start with '[' and end with ']'; that is reserved for representing extensions",
		},
		"success_json_name_not_quite_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string foobar = 10001 [json_name="[FooBar"];
					}`,
			},
		},
		"failure_synthetic_map_entry_reference": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  map<string,string> bar = 1;
					}
					message Baz {
					  Foo.BarEntry e = 1;
					}`,
			},
			expectedErr: "foo.proto:6:3: field Baz.e: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly",
		},
		"failure_imported_synthetic_map_entry_reference": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/struct.proto";
					message Foo {
					  google.protobuf.Struct.FieldsEntry e = 1;
					}`,
			},
			expectedErr: "foo.proto:4:3: field Foo.e: google.protobuf.Struct.FieldsEntry is a synthetic map entry and may not be referenced explicitly",
		},
		"failure_proto3_extend_add_field": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Foo {
					  extensions 1 to 100;
					}`,
				"bar.proto": `
					syntax = "proto3";
					import "foo.proto";
					extend Foo {
					  string bar = 1;
					}`,
			},
			expectedErr: "bar.proto:3:8: extend blocks in proto3 can only be used to define custom options",
		},
		"failure_oneof_disallows_empty_statement": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  oneof bar {
					    string baz = 1;
					    uint64 buzz = 2;
					    ;
					  }
					}`,
			},
			expectedErr: "foo.proto:6:5: syntax error: unexpected ';'",
		},
		"failure_extend_disallows_empty_statement": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions {
					  string baz = 1001;
					  uint64 buzz = 1002;
					  ;
					}`,
			},
			expectedErr: "foo.proto:6:3: syntax error: unexpected ';'",
		},
		"failure_oneof_field_conflict": {
			input: map[string]string{
				"a.proto": `
					syntax = "proto3";
					message m{
					  oneof z{
						int64 z=1;
					  }
					}`,
			},
			expectedErr: `a.proto:4:15: symbol "m.z" already defined at a.proto:3:9`,
		},
		"failure_oneof_field_conflict2": {
			input: map[string]string{
				"a.proto": `
					syntax="proto3";
					message m{
					  string z = 1;
					  oneof z{int64 b=2;}
					}`,
			},
			expectedErr: `a.proto:4:9: symbol "m.z" already defined at a.proto:3:10`,
		},
		"failure_oneof_conflicts": {
			input: map[string]string{
				"a.proto": `
					syntax="proto3";
					message m{
					  oneof z{int64 a=1;}
					  oneof z{int64 b=2;}
					}`,
			},
			expectedErr: `a.proto:4:9: symbol "m.z" already defined at a.proto:3:9`,
		},
		"success_message_literals": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					enum Foo { option allow_alias = true; true = 0; false = 1; t = 2; f = 3; True = 0; False = 1; inf = 6; nan = 7; }
					extend google.protobuf.MessageOptions { repeated Foo foo = 10001; }
					message Baz {
					  option (foo) = true; option (foo) = false;
					  option (foo) = t; option (foo) = f;
					  option (foo) = True; option (foo) = False;
					  option (foo) = inf; option (foo) = nan;
					}`,
			},
		},
		"failure_option_boolean_names": {
			// in options, boolean values must be "true" or "false"
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions { repeated bool foo = 10001; }
					message Baz {
					  option (foo) = true; option (foo) = false;
					  option (foo) = t; option (foo) = f;
					  option (foo) = True; option (foo) = False;
					}`,
			},
			expectedErr: "foo.proto:6:18: message Baz: option (foo): expecting bool, got identifier",
		},
		"success_message_literals_boolean_names": {
			// but inside message literals, boolean values can be
			// "true", "false", "t", "f", "True", or "False"
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo { repeated bool b = 1; }
					extend google.protobuf.MessageOptions { Foo foo = 10001; }
					message Baz {
					  option (foo) = {
						b: t     b: f
						b: true  b: false
						b: True  b: False
					  };
					}`,
			},
		},
		"failure_message_literal_leading_dot": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					message Foo { extensions 1 to 10; }
					extend Foo { optional bool b = 10; }
					extend google.protobuf.MessageOptions { optional Foo foo = 10001; }
					message Baz {
					  option (foo) = {
					    [.b]: true
					  };
					}`,
			},
			expectedErr: "foo.proto:8:6: syntax error: unexpected '.'",
		},
		"success_extension_resolution_custom_options": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message b {
					  message c {
						extend a { repeated int32 i = 1; repeated float f = 2; }
					  }
					  option (msga) = {
						[foo.bar.b.c.i]: 123
						[bar.b.c.i]: 234
						[b.c.i]: 345
					  };
					  option (msga).(foo.bar.b.c.f) = 1.23;
					  option (msga).(bar.b.c.f) = 2.34;
					  option (msga).(b.c.f) = 3.45;
					}`,
			},
		},
		"failure_extension_resolution_custom_options": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					message b { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message c {
					  extend a { optional b b = 1; }
					  extend b { repeated int32 i = 1; repeated float f = 2; }
					  option (msga) = {
						[foo.bar.c.b] {
						  [foo.bar.c.i]: 123
						  [bar.c.i]: 234
						  [c.i]: 345
						}
					  };
					  option (msga).(foo.bar.c.b).(foo.bar.c.f) = 1.23;
					  option (msga).(foo.bar.c.b).(bar.c.f) = 2.34;
					  option (msga).(foo.bar.c.b).(c.f) = 3.45;
					}`,
			},
			expectedErr: "test.proto:9:10: extendee is invalid: foo.bar.c.b is an extension, not a message",
		},
		"failure_extension_resolution_unknown": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message b {
					  message c {
						extend a { repeated int32 i = 1; repeated float f = 2; }
					  }
					  option (msga) = {
					    [c.i]: 456
					  };
					}`,
			},
			expectedErr: "test.proto:11:6: message foo.bar.b: option (foo.bar.msga): unknown extension c.i",
		},
		"failure_extension_resolution_unknown2": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message b {
					  message c {
					    extend a { repeated int32 i = 1; repeated float f = 2; }
					  }
					  option (msga) = {
					    [i]: 567
					  };
					}`,
			},
			expectedErr: "test.proto:11:6: message foo.bar.b: option (foo.bar.msga): unknown extension i",
		},
		"failure_extension_resolution_unknown3": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message b {
					  message c {
					    extend a { repeated int32 i = 1; repeated float f = 2; }
					  }
					  option (msga).(c.f) = 4.56;
					}`,
			},
			expectedErr: "test.proto:10:17: message foo.bar.b: unknown extension c.f",
		},
		"failure_extension_resolution_unknown4": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message a { extensions 1 to 100; }
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message b {
					  message c {
					    extend a { repeated int32 i = 1; repeated float f = 2; }
					  }
					  option (msga).(f) = 5.67;
					}`,
			},
			expectedErr: "test.proto:10:17: message foo.bar.b: unknown extension f",
		},
		"success_nested_extension_resolution_custom_options": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message a {
					  extensions 1 to 100;
					  message b {
					    message c {
					      extend a { repeated int32 i = 1; repeated float f = 2; }
					    }
					    option (msga) = {
					      [foo.bar.a.b.c.i]: 123
					      [bar.a.b.c.i]: 234
					      [a.b.c.i]: 345
					      // can't use b.c.i here
					    };
					    option (msga).(foo.bar.a.b.c.f) = 1.23;
					    option (msga).(bar.a.b.c.f) = 2.34;
					    option (msga).(a.b.c.f) = 3.45;
					    option (msga).(b.c.f) = 4.56;
					  }
					}`,
			},
		},
		"failure_extension_resolution_unknown_nested": {
			input: map[string]string{
				"test.proto": `
					syntax="proto2";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MessageOptions { optional a msga = 10000; }
					message a {
					  extensions 1 to 100;
					  message b {
					    message c {
					      extend a { repeated int32 i = 1; }
					    }
					    option (msga) = {
					      [b.c.i]: 345
					    };
					  }
					}`,
			},
			expectedErr: "test.proto:12:8: message foo.bar.a.b: option (foo.bar.msga): unknown extension b.c.i",
		},
		"success_any_message_literal": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/any.proto";
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional google.protobuf.Any any = 10001; }
					message Baz {
					  option (any) = {
					    [type.googleapis.com/foo.bar.Foo] <
					      a: "abc"
					      b: 123
					    >
					  };
					}`,
			},
		},
		"failure_any_message_literal_not_any": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional Foo f = 10001; }
					message Baz {
					  option (f) = {
					    [type.googleapis.com/foo.bar.Foo] <
					      a: "abc"
					      b: 123
					    >
					  };
					}`,
			},
			expectedErr: "foo.proto:8:6: message foo.bar.Baz: option (foo.bar.f): type references are only allowed for google.protobuf.Any, but this type is foo.bar.Foo",
		},
		"failure_any_message_literal_unsupported_domain": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/any.proto";
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional google.protobuf.Any any = 10001; }
					message Baz {
					  option (any) = {
					    [types.custom.io/foo.bar.Foo] <
					      a: "abc"
					      b: 123
					    >
					  };
					}`,
			},
			expectedErr: "foo.proto:9:6: message foo.bar.Baz: option (foo.bar.any): could not resolve type reference types.custom.io/foo.bar.Foo",
		},
		"failure_any_message_literal_scalar": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/any.proto";
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional google.protobuf.Any any = 10001; }
					message Baz {
					  option (any) = {
					    [type.googleapis.com/foo.bar.Foo]: 123
					  };
					}`,
			},
			expectedErr: "foo.proto:9:40: message foo.bar.Baz: option (foo.bar.any): type references for google.protobuf.Any must have message literal value",
		},
		"failure_any_message_literal_incorrect_type": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/any.proto";
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional google.protobuf.Any any = 10001; }
					message Baz {
					  option (any) = {
					    [type.googleapis.com/Foo] <
					      a: "abc"
					      b: 123
					    >
					  };
					}`,
			},
			expectedErr: "foo.proto:9:6: message foo.bar.Baz: option (foo.bar.any): could not resolve type reference type.googleapis.com/Foo",
		},
		"failure_any_message_literal_duplicate": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					import "google/protobuf/any.proto";
					import "google/protobuf/descriptor.proto";
					message Foo { string a = 1; int32 b = 2; }
					extend google.protobuf.MessageOptions { optional google.protobuf.Any any = 10001; }
					message Baz {
					  option (any) = {
					    [type.googleapis.com/foo.bar.Foo] <
					      a: "abc"
					      b: 123
					    >
					    [type.googleapis.com/foo.bar.Foo] <
					      a: "abc"
					      b: 123
					    >
					  };
					}`,
			},
			expectedErr: "foo.proto:13:6: message foo.bar.Baz: option (foo.bar.any): multiple any type references are not allowed",
		},
		"failure_scope_type_name": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.foo;
					import "other.proto";
					service Foo { rpc Bar (Baz) returns (Baz); }
					message Baz {
					  foo.Foo.Bar f = 1;
					}`,
				"other.proto": `
					syntax = "proto3";
					package foo;
					message Foo {
					  enum Bar { ZED = 0; }
					}`,
			},
			expectedErr: "foo.proto:6:3: field foo.foo.Baz.f: invalid type: foo.foo.Foo.Bar is a method, not a message or enum",
		},
		"failure_scope_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					message Foo {
					  enum Bar { ZED = 0; }
					  message Foo {
					    extend google.protobuf.MessageOptions {
					      string Bar = 30000;
					    }
					    Foo.Bar f = 1;
					  }
					}`,
			},
			expectedErr: "foo.proto:9:5: field Foo.Foo.f: invalid type: Foo.Foo.Bar is an extension, not a message or enum",
		},
		"success_scope_extension": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.ServiceOptions {
					  string Bar = 30000;
					}
					message Empty {}
					service Foo {
					  option (Bar) = "blah";
					  rpc Bar (Empty) returns (Empty);
					}`,
			},
		},
		"failure_scope_extension2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					extend google.protobuf.MethodOptions {
					  string Bar = 30000;
					}
					message Empty {}
					service Foo {
					  rpc Bar (Empty) returns (Empty) { option (Bar) = "blah"; }
					}`,
			},
			expectedErr: "foo.proto:8:44: method Foo.Bar: invalid extension: Bar is a method, not an extension",
		},
		"success_scope_extension2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					enum Bar { ZED = 0; }
					message Foo {
					  extend google.protobuf.MessageOptions {
					    string Bar = 30000;
					  }
					  message Foo {
					    Bar f = 1;
					  }
					}`,
			},
		},
		"failure_json_name_conflict_default": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string foo = 1;
					  string bar = 2 [json_name="foo"];
					}`,
			},
			expectedErr: `foo.proto:4:3: field Foo.bar: custom JSON name "foo" conflicts with default JSON name of field foo, defined at foo.proto:3:3`,
		},
		"failure_json_name_conflict_nested": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Blah {
					  message Foo {
					    string foo = 1;
					    string bar = 2 [json_name="foo"];
					  }
					}`,
			},
			expectedErr: `foo.proto:5:5: field Foo.bar: custom JSON name "foo" conflicts with default JSON name of field foo, defined at foo.proto:4:5`,
		},
		"success_json_names_case_sensitive": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string foo = 1 [json_name="foo_bar"];
					  string bar = 2 [json_name="Foo_Bar"];
					}`,
			},
		},
		"failure_json_name_conflict_default_underscore": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string fooBar = 1;
					  string foo_bar = 2;
					}`,
			},
			expectedErr: `foo.proto:4:3: field Foo.foo_bar: default JSON name "fooBar" conflicts with default JSON name of field fooBar, defined at foo.proto:3:3`,
		},
		"failure_json_name_conflict_default_override": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string fooBar = 1;
					  string foo_bar = 2 [json_name="fuber"];
					}`,
			},
			expectedErr: `foo.proto:4:3: field Foo.foo_bar: default JSON name "fooBar" conflicts with default JSON name of field fooBar, defined at foo.proto:3:3`,
		},
		"success_json_name_differs_by_case": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string fooBar = 1;
					  string FOO_BAR = 2;
					}`,
			},
		},
		"failure_json_name_conflict_leading_underscores": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Foo {
					  string _fooBar = 1;
					  string __foo_bar = 2;
					}`,
			},
			expectedErr: `foo.proto:4:3: field Foo.__foo_bar: default JSON name "FooBar" conflicts with default JSON name of field _fooBar, defined at foo.proto:3:3`,
		},
		"failure_json_name_custom_and_default_proto2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Blah {
					  message Foo {
					    optional string foo = 1 [json_name="fooBar"];
					    optional string foo_bar = 2;
					  }
					}`,
			},
			expectedErr: `foo.proto:5:5: field Foo.foo_bar: default JSON name "fooBar" conflicts with custom JSON name of field foo, defined at foo.proto:4:5`,
		},
		"success_json_name_default_proto3_only": {
			// should succeed: only check default JSON names in proto3
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Foo {
					  optional string fooBar = 1;
					  optional string foo_bar = 2;
					}`,
			},
		},
		"failure_json_name_conflict_proto2": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Foo {
					  optional string fooBar = 1 [json_name="fooBar"];
					  optional string foo_bar = 2 [json_name="fooBar"];
					}`,
			},
			expectedErr: `foo.proto:4:3: field Foo.foo_bar: custom JSON name "fooBar" conflicts with custom JSON name of field fooBar, defined at foo.proto:3:3`,
		},
		"success_json_name_default_proto2": {
			// should succeed: only check default JSON names in proto3
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Foo {
					  optional string fooBar = 1;
					  optional string FOO_BAR = 2;
					}`,
			},
		},
		"success_json_name_default_proto2_underscore": {
			// should succeed: only check default JSON names in proto3
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					message Foo {
					  optional string fooBar = 1;
					  optional string __foo_bar = 2;
					}`,
			},
		},
		"failure_enum_name_conflict": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					enum Foo {
					  true = 0;
					  TRUE = 1;
					}`,
			},
			expectedErr: `foo.proto:4:3: enum value Foo.TRUE: camel-case name (with optional enum name prefix removed) "True" conflicts with camel-case name of enum value true, defined at foo.proto:3:3`,
		},
		"failure_nested_enum_name_conflict": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					message Blah {
					  enum Foo {
					    true = 0;
					    TRUE = 1;
					  }
					}`,
			},
			expectedErr: `foo.proto:5:5: enum value Foo.TRUE: camel-case name (with optional enum name prefix removed) "True" conflicts with camel-case name of enum value true, defined at foo.proto:4:5`,
		},
		"failure_nested_enum_scope_conflict": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					enum Foo {
					  BAR_BAZ = 0;
					  Foo_Bar_Baz = 1;
					}`,
			},
			expectedErr: `foo.proto:4:3: enum value Foo.Foo_Bar_Baz: camel-case name (with optional enum name prefix removed) "BarBaz" conflicts with camel-case name of enum value BAR_BAZ, defined at foo.proto:3:3`,
		},
		"success_enum_name_conflict_allow_alias": {
			// should succeed: not a conflict if both values have same number
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					enum Foo {
					  option allow_alias = true;
					  BAR_BAZ = 0;
					  FooBarBaz = 0;
					}`,
			},
		},
		"failure_symbol_conflicts_with_package": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					package foo.bar;
					enum baz { ZED = 0; }`,
				"bar.proto": `
					syntax = "proto3";
					package foo.bar.baz;
					message Empty { }`,
			},
			expectedErr: `foo.proto:3:6: symbol "foo.bar.baz" already defined as a package at bar.proto:2:9` +
				` || bar.proto:2:9: symbol "foo.bar.baz" already defined at foo.proto:3:6`,
		},
		"success_enum_in_msg_literal_using_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					enum Foo {
					  ZERO = 0;
					  ONE = 1;
					}
					message Bar {
						optional Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						optional Bar bar = 10101;
					}
					option (bar) = { foo: 1 };`,
			},
		},
		"success_enum_in_msg_literal_using_negative_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					enum Foo {
						ZERO = 0;
						ONE = 1;
						NEG_ONE = -1;
					}
					message Bar {
						optional Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						optional Bar bar = 10101;
					}
					option (bar) = { foo: -1 };`,
			},
		},
		"success_open_enum_in_msg_literal_using_unknown_number": {
			// enums in proto3 are "open", so unknown numbers are acceptable
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					enum Foo {
					  ZERO = 0;
					  ONE = 1;
					}
					message Bar {
						Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						Bar bar = 10101;
					}
					option (bar) = { foo: 5 };`,
			},
		},
		"failure_enum_option_using_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					enum Foo {
					  ZERO = 0;
					  ONE = 1;
					}
					extend google.protobuf.FileOptions {
						optional Foo foo = 10101;
					}
					option (foo) = 1;`,
			},
			expectedErr: `foo.proto:10:16: option (foo): expecting enum name, got integer`,
		},
		"failure_default_value_for_enum_using_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					enum Foo {
					  ZERO = 0;
					  ONE = 1;
					}
					message Bar {
						optional Foo foo = 1 [default=1];
					}`,
			},
			expectedErr: `foo.proto:7:39: field Bar.foo: option default: expecting enum name, got integer`,
		},
		"failure_closed_enum_in_msg_literal_using_unknown_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto2";
					import "google/protobuf/descriptor.proto";
					enum Foo {
					  ZERO = 0;
					  ONE = 1;
					}
					message Bar {
						optional Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						optional Bar bar = 10101;
					}
					option (bar) = { foo: 5 };`,
			},
			expectedErr: `foo.proto:13:23: option (bar): closed enum Foo has no value with number 5`,
		},
		"failure_enum_in_msg_literal_using_out_of_range_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					enum Foo {
						ZERO = 0;
						ONE = 1;
						NEG_ONE = -1;
					}
					message Bar {
						Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						Bar bar = 10101;
					}
					option (bar) = { foo: 2147483648 };`,
			},
			expectedErr: `foo.proto:14:23: option (bar): value 2147483648 is out of range for an enum`,
		},
		"failure_enum_in_msg_literal_using_out_of_range_negative_number": {
			input: map[string]string{
				"foo.proto": `
					syntax = "proto3";
					import "google/protobuf/descriptor.proto";
					enum Foo {
						ZERO = 0;
						ONE = 1;
						NEG_ONE = -1;
					}
					message Bar {
						Foo foo = 1;
					}
					extend google.protobuf.FileOptions {
						Bar bar = 10101;
					}
					option (bar) = { foo: -2147483649 };`,
			},
			expectedErr: `foo.proto:14:23: option (bar): value -2147483649 is out of range for an enum`,
		},
		"success_custom_field_option": {
			input: map[string]string{
				"google/protobuf/descriptor.proto": `
					syntax = "proto2";
					package google.protobuf;
					message FieldOptions {
						optional string some_new_option = 11;
					}`,
				"bar.proto": `
					syntax = "proto3";
					package foo.bar.baz;
					message Foo {
						string bar = 1 [some_new_option="abc"];
					}`,
			},
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
			for filename, data := range tc.input {
				tc.input[filename] = removePrefixIndent(data)
			}
			_, err := compile(t, tc.input)
			var panicErr protocompile.PanicError
			if errors.As(err, &panicErr) {
				t.Logf("panic! %v\n%s", panicErr.Value, panicErr.Stack)
			}
			switch {
			case tc.expectedErr == "":
				if err != nil {
					t.Errorf("expecting no error; instead got error %q", err)
				}
			case err == nil:
				t.Errorf("expecting validation error %q; instead got no error", tc.expectedErr)
			default:
				msgs := strings.Split(tc.expectedErr, " || ")
				found := false
				for _, errMsg := range msgs {
					if err.Error() == errMsg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expecting validation error %q; instead got: %q", tc.expectedErr, err)
				}
			}
		})
	}
}

func removePrefixIndent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 || strings.TrimSpace(lines[0]) != "" {
		return s
	}
	lines = lines[1:] // skip first blank line
	// determine whitespace prefix from first line (e.g. five tabstops)
	var prefix []rune //nolint:prealloc
	for _, r := range lines[1] {
		if !unicode.IsSpace(r) {
			break
		}
		prefix = append(prefix, r)
	}
	prefixStr := string(prefix)
	for i := range lines {
		lines[i] = strings.TrimPrefix(lines[i], prefixStr)
	}
	return strings.Join(lines, "\n")
}

func compile(t *testing.T, input map[string]string) (linker.Files, error) {
	t.Helper()
	acc := func(filename string) (io.ReadCloser, error) {
		f, ok := input[filename]
		if !ok {
			return nil, fmt.Errorf("file not found: %s", filename)
		}
		return io.NopCloser(strings.NewReader(f)), nil
	}
	names := make([]string, 0, len(input))
	for k := range input {
		names = append(names, k)
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: acc,
		}),
	}
	return compiler.Compile(context.Background(), names...)
}

func TestProto3Enums(t *testing.T) {
	t.Parallel()
	file1 := `syntax = "<SYNTAX>"; enum bar { A = 0; B = 1; }`
	file2 := `syntax = "<SYNTAX>"; import "f1.proto"; message foo { <LABEL> bar bar = 1; }`
	getFileContents := func(file, syntax string) string {
		contents := strings.Replace(file, "<SYNTAX>", syntax, 1)
		label := ""
		if syntax == "proto2" {
			label = "optional"
		}
		return strings.Replace(contents, "<LABEL>", label, 1)
	}

	syntaxOptions := []string{"proto2", "proto3"}
	for _, o1 := range syntaxOptions {
		fc1 := getFileContents(file1, o1)

		for _, o2 := range syntaxOptions {
			fc2 := getFileContents(file2, o2)

			// now parse the protos
			acc := func(filename string) (io.ReadCloser, error) {
				var data string
				switch filename {
				case "f1.proto":
					data = fc1
				case "f2.proto":
					data = fc2
				default:
					return nil, fmt.Errorf("file not found: %s", filename)
				}
				return io.NopCloser(strings.NewReader(data)), nil
			}
			compiler := protocompile.Compiler{
				Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
					Accessor: acc,
				}),
			}
			_, err := compiler.Compile(context.Background(), "f1.proto", "f2.proto")

			if o1 != o2 && o2 == "proto3" {
				expected := "f2.proto:1:54: field foo.bar: cannot use proto2 enum bar in a proto3 message"
				if err == nil {
					t.Errorf("expecting validation error; instead got no error")
				} else if err.Error() != expected {
					t.Errorf("expecting validation error %q; instead got: %q", expected, err)
				}
			} else {
				// other cases succeed (okay to for proto2 to use enum from proto3 file and
				// obviously okay for proto2 importing proto2 and proto3 importing proto3)
				assert.Nil(t, err)
			}
		}
	}
}

func TestLinkerSymbolCollisionNoSource(t *testing.T) {
	t.Parallel()
	fdProto := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("foo.proto"),
		Dependency: []string{"google/protobuf/descriptor.proto"},
		Package:    proto.String("google.protobuf"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("DescriptorProto"),
			},
		},
	}
	resolver := protocompile.WithStandardImports(protocompile.ResolverFunc(func(s string) (protocompile.SearchResult, error) {
		if s == "foo.proto" {
			return protocompile.SearchResult{Proto: fdProto}, nil
		}
		return protocompile.SearchResult{}, protoregistry.NotFound
	}))
	compiler := &protocompile.Compiler{
		Resolver: resolver,
	}
	_, err := compiler.Compile(context.Background(), "foo.proto")
	require.Error(t, err)
	assert.EqualError(t, err, `foo.proto: symbol "google.protobuf.DescriptorProto" already defined at google/protobuf/descriptor.proto`)
}

func TestSyntheticMapEntryUsageNoSource(t *testing.T) {
	t.Parallel()
	baseFileDescProto := &descriptorpb.FileDescriptorProto{
		Name: proto.String("foo.proto"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Foo"),
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("BarEntry"),
						Options: &descriptorpb.MessageOptions{
							MapEntry: proto.Bool(true),
						},
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("key"),
								Number:   proto.Int32(1),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								JsonName: proto.String("key"),
							},
							{
								Name:     proto.String("value"),
								Number:   proto.Int32(2),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								JsonName: proto.String("value"),
							},
						},
					},
				},
			},
		},
	}
	testCases := map[string]struct {
		fields      []*descriptorpb.FieldDescriptorProto
		others      []*descriptorpb.DescriptorProto
		expectedErr string
	}{
		"success_valid_map": {
			fields: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("bar"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".Foo.BarEntry"),
					JsonName: proto.String("bar"),
				},
			},
		},
		"failure_not_repeated": {
			fields: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("bar"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".Foo.BarEntry"),
					JsonName: proto.String("bar"),
				},
			},
			expectedErr: `foo.proto: field Foo.bar: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly`,
		},
		"failure_name_mismatch": {
			fields: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("baz"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".Foo.BarEntry"),
					JsonName: proto.String("baz"),
				},
			},
			expectedErr: `foo.proto: field Foo.baz: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly`,
		},
		"failure_multiple_refs": {
			fields: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("bar"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".Foo.BarEntry"),
					JsonName: proto.String("bar"),
				},
				{
					Name:     proto.String("Bar"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".Foo.BarEntry"),
					JsonName: proto.String("Bar"),
				},
			},
			expectedErr: `foo.proto: field Foo.Bar: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly`,
		},
		"failure_wrong_message": {
			others: []*descriptorpb.DescriptorProto{
				{
					Name: proto.String("Bar"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     proto.String("bar"),
							Number:   proto.Int32(1),
							Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
							Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
							TypeName: proto.String(".Foo.BarEntry"),
							JsonName: proto.String("bar"),
						},
					},
				},
			},
			expectedErr: `foo.proto: field Bar.bar: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly`,
		},
	}
	for name, tc := range testCases {
		expectedPrefix := "success_"
		if tc.expectedErr != "" {
			expectedPrefix = "failure_"
		}
		assert.Truef(t, strings.HasPrefix(name, expectedPrefix), "expected test name %q to have %q prefix", name, expectedPrefix)
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fdProto := proto.Clone(baseFileDescProto).(*descriptorpb.FileDescriptorProto) //nolint:errcheck
			fdProto.MessageType[0].Field = tc.fields
			fdProto.MessageType = append(fdProto.MessageType, tc.others...)

			resolver := protocompile.ResolverFunc(func(s string) (protocompile.SearchResult, error) {
				if s == "foo.proto" {
					return protocompile.SearchResult{Proto: fdProto}, nil
				}
				return protocompile.SearchResult{}, protoregistry.NotFound
			})
			compiler := &protocompile.Compiler{
				Resolver: resolver,
			}
			_, err := compiler.Compile(context.Background(), "foo.proto")
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSyntheticOneOfCollisions(t *testing.T) {
	t.Parallel()
	input := map[string]string{
		"foo1.proto": `
			syntax = "proto3";
			message Foo {
			  optional string bar = 1;
			}`,
		"foo2.proto": `
			syntax = "proto3";
			message Foo {
			  optional string bar = 1;
			}`,
	}

	var errs []error
	compiler := &protocompile.Compiler{
		Reporter: reporter.NewReporter(
			func(err reporter.ErrorWithPos) error {
				errs = append(errs, err)
				// need to return nil to accumulate all errors so we can report synthetic
				// oneof collision; otherwise, the link will fail after the first collision
				// and we'll never test the synthetic oneofs
				return nil
			},
			nil,
		),
		Resolver: protocompile.ResolverFunc(func(filename string) (protocompile.SearchResult, error) {
			f, ok := input[filename]
			if !ok {
				return protocompile.SearchResult{}, fmt.Errorf("file not found: %s", filename)
			}
			return protocompile.SearchResult{Source: strings.NewReader(removePrefixIndent(f))}, nil
		}),
	}
	_, err := compiler.Compile(context.Background(), "foo1.proto", "foo2.proto")

	assert.Equal(t, reporter.ErrInvalidSource, err)

	// since files are compiled concurrently, there are two possible outcomes
	expectedFoo1FirstErrors := []string{
		`foo2.proto:2:9: symbol "Foo" already defined at foo1.proto:2:9`,
		`foo2.proto:3:19: symbol "Foo.bar" already defined at foo1.proto:3:19`,
		`foo2.proto:3:19: symbol "Foo._bar" already defined at foo1.proto:3:19`,
	}
	expectedFoo2FirstErrors := []string{
		`foo1.proto:2:9: symbol "Foo" already defined at foo2.proto:2:9`,
		`foo1.proto:3:19: symbol "Foo.bar" already defined at foo2.proto:3:19`,
		`foo1.proto:3:19: symbol "Foo._bar" already defined at foo2.proto:3:19`,
	}
	var expected []string
	require.NotEmpty(t, errs)
	actual := make([]string, len(errs))
	for i, err := range errs {
		actual[i] = err.Error()
	}
	if strings.HasPrefix(actual[0], "foo2.proto") {
		expected = expectedFoo1FirstErrors
	} else {
		expected = expectedFoo2FirstErrors
	}
	assert.Equal(t, expected, actual)
}

func TestCustomJSONNameWarnings(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		source  string
		warning string
	}{
		{
			source: `
				syntax = "proto2";
				message Foo {
				  optional string foo_bar = 1;
				  optional string fooBar = 2;
				}`,
			warning: `test.proto:4:3: field Foo.fooBar: default JSON name "fooBar" conflicts with default JSON name of field foo_bar, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				message Foo {
				  optional string foo_bar = 1;
				  optional string fooBar = 2;
				}`,
			warning: `test.proto:4:3: field Foo.fooBar: default JSON name "fooBar" conflicts with default JSON name of field foo_bar, defined at test.proto:3:3`,
		},
		// in nested message
		{
			source: `
				syntax = "proto2";
				message Blah { message Foo {
				  optional string foo_bar = 1;
				  optional string fooBar = 2;
				} }`,
			warning: `test.proto:4:3: field Foo.fooBar: default JSON name "fooBar" conflicts with default JSON name of field foo_bar, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				message Blah { message Foo {
				  optional string foo_bar = 1;
				  optional string fooBar = 2;
				} }`,
			warning: `test.proto:4:3: field Foo.fooBar: default JSON name "fooBar" conflicts with default JSON name of field foo_bar, defined at test.proto:3:3`,
		},
		// enum values
		{
			source: `
				syntax = "proto2";
				enum Foo {
				  true = 0;
				  TRUE = 1;
				}`,
			warning: `test.proto:4:3: enum value Foo.TRUE: camel-case name (with optional enum name prefix removed) "True" conflicts with camel-case name of enum value true, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				enum Foo {
				  fooBar_Baz = 0;
				  _FOO__BAR_BAZ = 1;
				}`,
			warning: `test.proto:4:3: enum value Foo._FOO__BAR_BAZ: camel-case name (with optional enum name prefix removed) "BarBaz" conflicts with camel-case name of enum value fooBar_Baz, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				enum Foo {
				  fooBar_Baz = 0;
				  FOO__BAR__BAZ__ = 1;
				}`,
			warning: `test.proto:4:3: enum value Foo.FOO__BAR__BAZ__: camel-case name (with optional enum name prefix removed) "BarBaz" conflicts with camel-case name of enum value fooBar_Baz, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				enum Foo {
				  fooBarBaz = 0;
				  _FOO__BAR_BAZ = 1;
				}`,
			warning: "",
		},
		{
			source: `
				syntax = "proto2";
				enum Foo {
				  option allow_alias = true;
				  Bar_Baz = 0;
				  _BAR_BAZ_ = 0;
				  FOO_BAR_BAZ = 0;
				  foobar_baz = 0;
				}`,
			warning: "",
		},
		// in nested message
		{
			source: `
				syntax = "proto2";
				message Blah { enum Foo {
				  true = 0;
				  TRUE = 1;
				} }`,
			warning: `test.proto:4:3: enum value Foo.TRUE: camel-case name (with optional enum name prefix removed) "True" conflicts with camel-case name of enum value true, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				message Blah { enum Foo {
				  fooBar_Baz = 0;
				  _FOO__BAR_BAZ = 1;
				} }`,
			warning: `test.proto:4:3: enum value Foo._FOO__BAR_BAZ: camel-case name (with optional enum name prefix removed) "BarBaz" conflicts with camel-case name of enum value fooBar_Baz, defined at test.proto:3:3`,
		},
		{
			source: `
				syntax = "proto2";
				message Blah { enum Foo {
				  option allow_alias = true;
				  Bar_Baz = 0;
				  _BAR_BAZ_ = 0;
				  FOO_BAR_BAZ = 0;
				  foobar_baz = 0;
				} }`,
			warning: "",
		},
	}
	for i, tc := range testCases {
		resolver := protocompile.ResolverFunc(func(filename string) (protocompile.SearchResult, error) {
			if filename == "test.proto" {
				return protocompile.SearchResult{Source: strings.NewReader(removePrefixIndent(tc.source))}, nil
			}
			return protocompile.SearchResult{}, fmt.Errorf("file not found: %s", filename)
		})
		var warnings []string
		warnFunc := func(err reporter.ErrorWithPos) {
			warnings = append(warnings, err.Error())
		}
		compiler := protocompile.Compiler{
			Resolver: resolver,
			Reporter: reporter.NewReporter(nil, warnFunc),
		}
		_, err := compiler.Compile(context.Background(), "test.proto")
		if err != nil {
			t.Errorf("case %d: expecting no error; instead got error %q", i, err)
		}
		if tc.warning == "" && len(warnings) > 0 {
			t.Errorf("case %d: expecting no warnings; instead got: %v", i, warnings)
		} else if tc.warning != "" {
			found := false
			for _, w := range warnings {
				if w == tc.warning {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("case %d: expecting warning %q; instead got: %v", i, tc.warning, warnings)
			}
		}
	}
}
