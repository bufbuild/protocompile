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

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
)

func TestSimpleLink(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testdata"},
		}),
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_complex.proto")
	if !assert.Nil(t, err) {
		return
	}

	res := fds[0].(linker.Result)
	fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/desc_test_complex.protoset", linker.ResolverFromFile(fds[0]))
	prototest.CheckFiles(t, res, fdset, true)
}

func TestMultiFileLink(t *testing.T) {
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

		res := fds[0].(linker.Result)
		fdset := prototest.LoadDescriptorSet(t, "../internal/testdata/all.protoset", linker.ResolverFromFile(fds[0]))
		prototest.CheckFiles(t, res, fdset, true)
	}
}

func TestProto3Optional(t *testing.T) {
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

	res := fds[0].(linker.Result)
	prototest.CheckFiles(t, res, fdset, true)
}

func TestLinkerValidation(t *testing.T) {
	testCases := []struct {
		input  map[string]string
		errMsg string
	}{
		{
			map[string]string{
				"foo.proto":  `syntax = "proto3"; package namespace.a; import "foo2.proto"; import "foo3.proto"; import "foo4.proto"; message Foo{ b.Bar a = 1; b.Baz b = 2; b.Buzz c = 3; }`,
				"foo2.proto": `syntax = "proto3"; package namespace.b; message Bar{}`,
				"foo3.proto": `syntax = "proto3"; package namespace.b; message Baz{}`,
				"foo4.proto": `syntax = "proto3"; package namespace.b; message Buzz{}`,
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "import \"foo2.proto\"; message fubar{}",
			},
			`foo.proto:1:8: file not found: foo2.proto`,
		},
		{
			map[string]string{
				"foo.proto":  "import \"foo2.proto\"; message fubar{}",
				"foo2.proto": "import \"foo.proto\"; message baz{}",
			},
			// since files are compiled concurrently, there are two possible outcomes
			`foo.proto:1:8: cycle found in imports: "foo.proto" -> "foo2.proto" -> "foo.proto"` +
				` || foo2.proto:1:8: cycle found in imports: "foo2.proto" -> "foo.proto" -> "foo2.proto"`,
		},
		{
			map[string]string{
				"foo.proto": "enum foo { bar = 1; baz = 2; } enum fu { bar = 1; baz = 2; }",
			},
			`foo.proto:1:42: symbol "bar" already defined at foo.proto:1:12; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum`,
		},
		{
			map[string]string{
				"foo.proto": "message foo {} enum foo { V = 0; }",
			},
			`foo.proto:1:21: symbol "foo" already defined at foo.proto:1:9`,
		},
		{
			map[string]string{
				"foo.proto": "message foo { optional string a = 1; optional string a = 2; }",
			},
			`foo.proto:1:54: symbol "foo.a" already defined at foo.proto:1:31`,
		},
		{
			map[string]string{
				"foo.proto":  "message foo {}",
				"foo2.proto": "enum foo { V = 0; }",
			},
			// since files are compiled concurrently, there are two possible outcomes
			"foo.proto:1:9: symbol \"foo\" already defined at foo2.proto:1:6" +
				" || foo2.proto:1:6: symbol \"foo\" already defined at foo.proto:1:9",
		},
		{
			map[string]string{
				"foo.proto": "message foo { optional blah a = 1; }",
			},
			"foo.proto:1:24: field foo.a: unknown type blah",
		},
		{
			map[string]string{
				"foo.proto": "message foo { optional bar.baz a = 1; } service bar { rpc baz (foo) returns (foo); }",
			},
			"foo.proto:1:24: field foo.a: invalid type: bar.baz is a method, not a message or enum",
		},
		{
			map[string]string{
				"foo.proto": "message foo { extensions 1 to 2; } extend foo { optional string a = 1; } extend foo { optional int32 b = 1; }",
			},
			"foo.proto:1:106: extension with tag 1 for message foo already defined at foo.proto:1:69",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; extend foobar { optional string a = 1; }",
			},
			"foo.proto:1:24: unknown extendee type foobar",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; service foobar{} extend foobar { optional string a = 1; }",
			},
			"foo.proto:1:41: extendee is invalid: fu.baz.foobar is a service, not a message",
		},
		{
			map[string]string{
				"foo.proto": "message foo{} message bar{} service foobar{ rpc foo(foo) returns (bar); }",
			},
			"foo.proto:1:53: method foobar.foo: invalid request type: foobar.foo is a method, not a message",
		},
		{
			map[string]string{
				"foo.proto": "message foo{} message bar{} service foobar{ rpc foo(bar) returns (foo); }",
			},
			"foo.proto:1:67: method foobar.foo: invalid response type: foobar.foo is a method, not a message",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; message foobar{ extensions 1; } extend foobar { optional string a = 2; }",
			},
			"foo.proto:1:85: extension fu.baz.a: tag 2 is not in valid range for extended type fu.baz.foobar",
		},
		{
			map[string]string{
				"foo.proto":  "package fu.baz; import public \"foo2.proto\"; message foobar{ optional baz a = 1; }",
				"foo2.proto": "package fu.baz; import \"foo3.proto\"; message fizzle{ }",
				"foo3.proto": "package fu.baz; message baz{ }",
			},
			"foo.proto:1:70: field fu.baz.foobar.a: unknown type baz; resolved to fu.baz which is not defined; consider using a leading dot",
		},
		{
			map[string]string{
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
					}
					`,
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; message foobar{ repeated string a = 1 [default = \"abc\"]; }",
			},
			"foo.proto:1:56: field fu.baz.foobar.a: default value cannot be set because field is repeated",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; message foobar{ optional foobar a = 1 [default = { a: {} }]; }",
			},
			"foo.proto:1:56: field fu.baz.foobar.a: default value cannot be set because field is a message",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; message foobar{ optional string a = 1 [default = { a: \"abc\" }]; }",
			},
			"foo.proto:1:66: field fu.baz.foobar.a: default value cannot be a message",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; message foobar{ optional string a = 1 [default = 1.234]; }",
			},
			"foo.proto:1:66: field fu.baz.foobar.a: option default: expecting string, got double",
		},
		{
			map[string]string{
				"foo.proto": "package fu.baz; enum abc { OK=0; NOK=1; } message foobar{ optional abc a = 1 [default = NACK]; }",
			},
			"foo.proto:1:89: field fu.baz.foobar.a: option default: enum fu.baz.abc has no value named NACK",
		},
		{
			map[string]string{
				"foo.proto": "option b = 123;",
			},
			"foo.proto:1:8: option b: field b of google.protobuf.FileOptions does not exist",
		},
		{
			map[string]string{
				"foo.proto": "option (foo.bar) = 123;",
			},
			"foo.proto:1:8: unknown extension foo.bar",
		},
		{
			map[string]string{
				"foo.proto": "option uninterpreted_option = { };",
			},
			"foo.proto:1:8: invalid option 'uninterpreted_option'",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f).b = 123;",
			},
			"foo.proto:5:12: option (f).b: field b of foo does not exist",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f).a = 123;",
			},
			"foo.proto:5:16: option (f).a: expecting string, got integer",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (b) = 123;",
			},
			"foo.proto:5:8: option (b): extension b should extend google.protobuf.FileOptions but instead extends foo",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (foo) = 123;",
			},
			"foo.proto:5:8: invalid extension: foo is a message, not an extension",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (foo.a) = 123;",
			},
			"foo.proto:5:8: invalid extension: foo.a is a field but not an extension",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: [ 123 ] };",
			},
			"foo.proto:5:19: option (f): value is an array but field is not repeated",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { repeated string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: [ \"a\", \"b\", 123 ] };",
			},
			"foo.proto:5:31: option (f): expecting string, got integer",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: \"a\" };\n" +
					"option (f) = { a: \"b\" };",
			},
			"foo.proto:6:8: option (f): non-repeated option field (f) already set",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: \"a\" };\n" +
					"option (f).a = \"b\";",
			},
			"foo.proto:6:12: option (f).a: non-repeated option field a already set",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { optional string a = 1; extensions 10 to 20; }\n" +
					"extend foo { optional int32 b = 10; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: \"a\" };\n" +
					"option (f).(b) = \"b\";",
			},
			"foo.proto:6:18: option (f).(b): expecting int32, got string",
		},
		{
			map[string]string{
				"foo.proto": "import \"google/protobuf/descriptor.proto\";\n" +
					"message foo { required string a = 1; required string b = 2; }\n" +
					"extend google.protobuf.FileOptions { optional foo f = 20000; }\n" +
					"option (f) = { a: \"a\" };\n",
			},
			"foo.proto:1:1: error in file options: some required fields missing: (f).b",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional int32 bar = 1; }",
			},
			"foo.proto:1:99: messages with message-set wire format cannot contain scalar extensions, only messages",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional Foo bar = 1; }",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "message Foo { extensions 1 to max; } extend Foo { optional int32 bar = 536870912; }",
			},
			"foo.proto:1:72: extension bar: tag 536870912 is not in valid range for extended type Foo",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to max; } extend Foo { optional Foo bar = 536870912; }",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional int32 bar = 1; }",
			},
			"foo.proto:1:99: messages with message-set wire format cannot contain scalar extensions, only messages",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { optional Foo bar = 1; }",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to 100; } extend Foo { repeated Foo bar = 1; }",
			},
			"foo.proto:1:90: messages with message-set wire format cannot contain repeated extensions, only optional",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { extensions 1 to max; } extend Foo { optional int32 bar = 536870912; }",
			},
			"foo.proto:1:72: extension bar: tag 536870912 is not in valid range for extended type Foo",
		},
		{
			map[string]string{
				"foo.proto": "message Foo { option message_set_wire_format = true; extensions 1 to max; } extend Foo { optional Foo bar = 536870912; }",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3"; package com.google; import "google/protobuf/wrappers.proto"; message Foo { google.protobuf.StringValue str = 1; }`,
			},
			"foo.proto:1:95: field com.google.Foo.str: unknown type google.protobuf.StringValue; resolved to com.google.protobuf.StringValue which is not defined; consider using a leading dot",
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo {\n" +
					"  optional group Bar = 1 { optional string name = 1; }\n" +
					"}\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo).bar.name = \"abc\"; }\n",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo {\n" +
					"  optional group Bar = 1 { optional string name = 1; }\n" +
					"}\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo).Bar.name = \"abc\"; }\n",
			},
			"foo.proto:7:28: message Baz: option (foo).Bar.name: field Bar of Foo does not exist",
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"extend google.protobuf.MessageOptions {\n" +
					"  optional group Foo = 10001 { optional string name = 1; }\n" +
					"}\n" +
					"message Bar { option (foo).name = \"abc\"; }\n",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"extend google.protobuf.MessageOptions {\n" +
					"  optional group Foo = 10001 { optional string name = 1; }\n" +
					"}\n" +
					"message Bar { option (Foo).name = \"abc\"; }\n",
			},
			"foo.proto:6:22: message Bar: invalid extension: Foo is a message, not an extension",
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo {\n" +
					"  optional group Bar = 1 { optional string name = 1; }\n" +
					"}\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo) = { Bar< name: \"abc\" > }; }\n",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo {\n" +
					"  optional group Bar = 1 { optional string name = 1; }\n" +
					"}\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo) = { bar< name: \"abc\" > }; }\n",
			},
			"foo.proto:7:32: message Baz: option (foo): field bar not found (did you mean the group named Bar?)",
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { extensions 1 to 10; }\n" +
					"extend Foo { optional group Bar = 10 { optional string name = 1; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo) = { [bar]< name: \"abc\" > }; }\n",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto2\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { extensions 1 to 10; }\n" +
					"extend Foo { optional group Bar = 10 { optional string name = 1; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo) = { [Bar]< name: \"abc\" > }; }\n",
			},
			"foo.proto:6:32: message Baz: option (foo): field Bar not found",
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { oneof bar { string baz = 1; string buzz = 2; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz { option (foo) = { baz: \"abc\" buzz: \"xyz\" }; }\n",
			},
			`foo.proto:5:43: message Baz: option (foo): oneof "bar" already has field "baz" set`,
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { oneof bar { string baz = 1; string buzz = 2; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz {\n" +
					"  option (foo).baz = \"abc\";\n" +
					"  option (foo).buzz = \"xyz\";\n" +
					"}",
			},
			`foo.proto:7:16: message Baz: option (foo).buzz: oneof "bar" already has field "baz" set`,
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { oneof bar { google.protobuf.DescriptorProto baz = 1; google.protobuf.DescriptorProto buzz = 2; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz {\n" +
					"  option (foo).baz.name = \"abc\";\n" +
					"  option (foo).buzz.name = \"xyz\";\n" +
					"}",
			},
			`foo.proto:7:16: message Baz: option (foo).buzz.name: oneof "bar" already has field "baz" set`,
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { oneof bar { google.protobuf.DescriptorProto baz = 1; google.protobuf.DescriptorProto buzz = 2; } }\n" +
					"extend google.protobuf.MessageOptions { optional Foo foo = 10001; }\n" +
					"message Baz {\n" +
					"  option (foo).baz.options.(foo).baz.name = \"abc\";\n" +
					"  option (foo).baz.options.(foo).buzz.name = \"xyz\";\n" +
					"}",
			},
			`foo.proto:7:34: message Baz: option (foo).baz.options.(foo).buzz.name: oneof "bar" already has field "baz" set`,
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { repeated string strs = 1; repeated Foo foos = 2; }\n" +
					"extend google.protobuf.FileOptions { optional Foo foo = 10001; }\n" +
					"option (foo) = {\n" +
					"  strs: []\n" +
					"  foos []\n" +
					"};",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { repeated string strs = 1; repeated Foo foos = 2; }\n" +
					"extend google.protobuf.FileOptions { optional Foo foo = 10001; }\n" +
					"option (foo) = {\n" +
					"  strs []\n" +
					"  foos []\n" +
					"};",
			},
			`foo.proto:6:8: syntax error: unexpected value, expecting ':'`,
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { repeated string strs = 1; repeated Foo foos = 2; }\n" +
					"extend google.protobuf.FileOptions { optional Foo foo = 10001; }\n" +
					"option (foo) = {\n" +
					"  strs: ['abc', 'def']\n" +
					"  foos [<strs:'foo'>, <strs:'bar'>]\n" +
					"};",
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": "syntax = \"proto3\";\n" +
					"import \"google/protobuf/descriptor.proto\";\n" +
					"message Foo { repeated string strs = 1; repeated Foo foos = 2; }\n" +
					"extend google.protobuf.FileOptions { optional Foo foo = 10001; }\n" +
					"option (foo) = {\n" +
					"  strs ['abc', 'def']\n" +
					"  foos [<strs:'foo'>, <strs:'bar'>]\n" +
					"};",
			},
			`foo.proto:6:9: syntax error: unexpected string literal, expecting '{' or '<' or ']'`,
		},
		{
			map[string]string{
				"foo.proto": "package foo.bar;\n" +
					"message M { \n" +
					"  enum E { M = 0; }\n" +
					"  optional M F1 = 1;\n" +
					"  extensions 2 to 2;\n" +
					"  extend M { optional string F2 = 2; }\n" +
					"}",
			},
			`foo.proto:6:10: extendee is invalid: foo.bar.M.M is a enum value, not a message`,
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions {
  string foobar = 10001 [json_name="FooBar"];
}`,
			},
			"foo.proto:4:26: field foobar: option json_name is not allowed on extensions",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
message Foo {
  map<string,string> bar = 1;
}
message Baz {
  Foo.BarEntry e = 1;
}`,
			},
			"foo.proto:6:3: field Baz.e: Foo.BarEntry is a synthetic map entry and may not be referenced explicitly",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
import "google/protobuf/struct.proto";
message Foo {
  google.protobuf.Struct.FieldsEntry e = 1;
}`,
			},
			"foo.proto:4:3: field Foo.e: google.protobuf.Struct.FieldsEntry is a synthetic map entry and may not be referenced explicitly",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto2";
message Foo {
  extensions 1 to 100;
}`,
				"bar.proto": `syntax = "proto3";
import "foo.proto";
extend Foo {
  string bar = 1;
}`,
			},
			"bar.proto:3:8: extend blocks in proto3 can only be used to define custom options",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
message Foo {
  oneof bar {
    string baz = 1;
    uint64 buzz = 2;
    ;
  }
}`,
			},
			"foo.proto:6:5: syntax error: unexpected ';'",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions {
  string baz = 1001;
  uint64 buzz = 1002;
  ;
}`,
			},
			"foo.proto:6:3: syntax error: unexpected ';'",
		},
		{
			map[string]string{
				"a.proto": `syntax = "proto3";
message m{
  oneof z{
	int64 z=1;
  }
}`,
			},
			`a.proto:4:15: symbol "m.z" already defined at a.proto:3:9`,
		},
		{
			map[string]string{
				"a.proto": `syntax="proto3";
message m{
  string z = 1;
  oneof z{int64 b=2;}
}`,
			},
			`a.proto:4:9: symbol "m.z" already defined at a.proto:3:10`,
		},
		{
			map[string]string{
				"a.proto": `syntax="proto3";
message m{
  oneof z{int64 a=1;}
  oneof z{int64 b=2;}
}`,
			},
			`a.proto:4:9: symbol "m.z" already defined at a.proto:3:9`,
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
import "google/protobuf/descriptor.proto";
enum Foo { true = 0; false = 1; t = 2; f = 3; True = 4; False = 5; inf = 6; nan = 7; }
extend google.protobuf.MessageOptions { repeated Foo foo = 10001; }
message Baz {
  option (foo) = true; option (foo) = false;
  option (foo) = t; option (foo) = f;
  option (foo) = True; option (foo) = False;
  option (foo) = inf; option (foo) = nan;
}`,
			},
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions { repeated bool foo = 10001; }
message Baz {
  option (foo) = true; option (foo) = false;
  option (foo) = t; option (foo) = f;
  option (foo) = True; option (foo) = False;
}`,
			},
			"foo.proto:6:18: message Baz: option (foo): expecting bool, got identifier",
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto3";
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
			"", // should succeed
		},
		{
			map[string]string{
				"foo.proto": `syntax = "proto2";
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
			"foo.proto:8:6: syntax error: unexpected '.'",
		},
	}

	for i, tc := range testCases {
		t.Log("test case", i)
		acc := func(filename string) (io.ReadCloser, error) {
			f, ok := tc.input[filename]
			if !ok {
				return nil, fmt.Errorf("file not found: %s", filename)
			}
			return io.NopCloser(strings.NewReader(f)), nil
		}
		names := make([]string, 0, len(tc.input))
		for k := range tc.input {
			names = append(names, k)
		}

		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				Accessor: acc,
			}),
		}
		_, err := compiler.Compile(context.Background(), names...)
		var panicErr protocompile.PanicError
		if errors.As(err, &panicErr) {
			t.Logf("case %d: panic! %v\n%s", i, panicErr.Value, panicErr.Stack)
		}
		if tc.errMsg == "" {
			if err != nil {
				t.Errorf("case %d: expecting no error; instead got error %q", i, err)
			}
		} else if err == nil {
			t.Errorf("case %d: expecting validation error %q; instead got no error", i, tc.errMsg)
		} else {
			msgs := strings.Split(tc.errMsg, " || ")
			found := false
			for _, errMsg := range msgs {
				if err.Error() == errMsg {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("case %d: expecting validation error %q; instead got: %q", i, tc.errMsg, err)
			}
		}
	}
}

func TestProto3Enums(t *testing.T) {
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
