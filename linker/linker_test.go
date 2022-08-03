package linker_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile"
	_ "github.com/jhump/protocompile/internal/testprotos"
	"github.com/jhump/protocompile/linker"
)

func TestSimpleLink(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testprotos"},
		}),
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_complex.proto")
	if !assert.Nil(t, err) {
		return
	}

	res := fds[0].(linker.Result)
	fdset := loadDescriptorSet(t, "../internal/testprotos/desc_test_complex.protoset", linker.ResolverFromFile(fds[0]))
	checkFiles(t, res, (*fdsProtoSet)(fdset), map[string]struct{}{})
}

func loadDescriptorSet(t *testing.T, path string, res linker.Resolver) *descriptorpb.FileDescriptorSet {
	data, err := ioutil.ReadFile(path)
	if !assert.Nil(t, err) {
		t.Fail()
	}
	var fdset descriptorpb.FileDescriptorSet
	err = proto.UnmarshalOptions{Resolver: res}.Unmarshal(data, &fdset)
	if !assert.Nil(t, err) {
		t.Fail()
	}
	return &fdset
}

func TestMultiFileLink(t *testing.T) {
	for _, name := range []string{"desc_test_defaults.proto", "desc_test_field_types.proto", "desc_test_options.proto", "desc_test_wellknowntypes.proto"} {
		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				ImportPaths: []string{"../internal/testprotos"},
			}),
		}
		fds, err := compiler.Compile(context.Background(), name)
		if !assert.Nil(t, err) {
			continue
		}

		res := fds[0].(linker.Result)
		checkFiles(t, res, (*regProtoSet)(protoregistry.GlobalFiles), map[string]struct{}{})
	}
}

func TestProto3Optional(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{"../internal/testprotos"},
		}),
	}
	fds, err := compiler.Compile(context.Background(), "desc_test_proto3_optional.proto")
	if !assert.Nil(t, err) {
		return
	}

	fdset := loadDescriptorSet(t, "../internal/testprotos/desc_test_proto3_optional.protoset", fds.AsResolver())

	res := fds[0].(linker.Result)
	checkFiles(t, res, (*fdsProtoSet)(fdset), map[string]struct{}{})
}

func checkFiles(t *testing.T, act protoreflect.FileDescriptor, expSet fileProtoSet, checked map[string]struct{}) {
	if _, ok := checked[act.Path()]; ok {
		// already checked
		return
	}
	checked[act.Path()] = struct{}{}

	expProto := expSet.findFile(act.Path())
	actProto := toProto(act)
	checkFileDescriptor(t, actProto, expProto)

	for i := 0; i < act.Imports().Len(); i++ {
		checkFiles(t, act.Imports().Get(i), expSet, checked)
	}
}

func checkFileDescriptor(t *testing.T, act, exp *descriptorpb.FileDescriptorProto) {
	compareFiles(t, fmt.Sprintf("%q", act.GetName()), exp, act)
}

func toProto(f protoreflect.FileDescriptor) *descriptorpb.FileDescriptorProto {
	type canProto interface {
		Proto() *descriptorpb.FileDescriptorProto
	}
	if can, ok := f.(canProto); ok {
		return can.Proto()
	}
	return protodesc.ToFileDescriptorProto(f)
}

func toString(m proto.Message) string {
	mo := protojson.MarshalOptions{Indent: " ", Multiline: true}
	js, err := mo.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(js)
}

type fileProtoSet interface {
	findFile(name string) *descriptorpb.FileDescriptorProto
}

type fdsProtoSet descriptorpb.FileDescriptorSet

var _ fileProtoSet = &fdsProtoSet{}

func (fps *fdsProtoSet) findFile(name string) *descriptorpb.FileDescriptorProto {
	files := (*descriptorpb.FileDescriptorSet)(fps).File
	for _, fd := range files {
		if fd.GetName() == name {
			return fd
		}
	}
	return nil
}

type regProtoSet protoregistry.Files

var _ fileProtoSet = &regProtoSet{}

func (fps *regProtoSet) findFile(name string) *descriptorpb.FileDescriptorProto {
	f, err := (*protoregistry.Files)(fps).FindFileByPath(name)
	if err != nil {
		return nil
	}
	return toProto(f)
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
			"foo.proto:1:85: field fu.baz.a: tag 2 is not in valid range for extended type fu.baz.foobar",
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
			"foo.proto:1:72: field bar: tag 536870912 is not in valid range for extended type Foo",
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
			"foo.proto:1:72: field bar: tag 536870912 is not in valid range for extended type Foo",
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
			"foo.proto:7:30: message Baz: option (foo): field bar not found (did you mean the group named Bar?)",
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
			"foo.proto:6:30: message Baz: option (foo): field Bar not found",
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
	}

	for i, tc := range testCases {
		t.Log("test case", i+1)
		acc := func(filename string) (io.ReadCloser, error) {
			f, ok := tc.input[filename]
			if !ok {
				return nil, fmt.Errorf("file not found: %s", filename)
			}
			return ioutil.NopCloser(strings.NewReader(f)), nil
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
				return ioutil.NopCloser(strings.NewReader(data)), nil
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

// adapted from implementation of proto.Equal, but records an error for each discrepancy
// found (does NOT exit early when a discrepancy is found)
func compareFiles(t *testing.T, path string, exp, act *descriptorpb.FileDescriptorProto) {
	if (exp == nil) != (act == nil) {
		if exp == nil {
			t.Errorf("%s: expected is nil; actual is not", path)
		} else {
			t.Errorf("%s: expected is not nil, but actual is", path)
		}
		return
	}
	mexp := exp.ProtoReflect()
	mact := act.ProtoReflect()
	if mexp.IsValid() != mact.IsValid() {
		if mexp.IsValid() {
			t.Errorf("%s: expected is valid; actual is not", path)
		} else {
			t.Errorf("%s: expected is not valid, but actual is", path)
		}
		return
	}
	compareMessages(t, path, mexp, mact)
}

func compareMessages(t *testing.T, path string, exp, act protoreflect.Message) {
	if exp.Descriptor() != act.Descriptor() {
		t.Errorf("%s: descriptors do not match: exp %#v, actual %#v", path, exp.Descriptor(), act.Descriptor())
		return
	}
	exp.Range(func(fd protoreflect.FieldDescriptor, expVal protoreflect.Value) bool {
		name := fieldDisplayName(fd)
		actVal := act.Get(fd)
		if !act.Has(fd) {
			t.Errorf("%s: expected has field %s but actual does not", path, name)
		} else {
			compareFields(t, path+"."+name, fd, expVal, actVal)
		}
		return true
	})
	act.Range(func(fd protoreflect.FieldDescriptor, actVal protoreflect.Value) bool {
		name := fieldDisplayName(fd)
		if !exp.Has(fd) {
			t.Errorf("%s: actual has field %s but expected does not", path, name)
		}
		return true
	})

	compareUnknown(t, path, exp.GetUnknown(), act.GetUnknown())
}

func fieldDisplayName(fd protoreflect.FieldDescriptor) string {
	if fd.IsExtension() {
		return "(" + string(fd.FullName()) + ")"
	}
	return string(fd.Name())
}

func compareFields(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Value) {
	switch {
	case fd.IsList():
		compareLists(t, path, fd, exp.List(), act.List())
	case fd.IsMap():
		compareMaps(t, path, fd, exp.Map(), act.Map())
	default:
		compareValues(t, path, fd, exp, act)
	}
}

func compareMaps(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Map) {
	exp.Range(func(k protoreflect.MapKey, expVal protoreflect.Value) bool {
		actVal := act.Get(k)
		if !act.Has(k) {
			t.Errorf("%s: expected map has key %s but actual does not", path, k.String())
		} else {
			compareValues(t, path+"["+k.String()+"]", fd.MapValue(), expVal, actVal)
		}
		return true
	})
	act.Range(func(k protoreflect.MapKey, actVal protoreflect.Value) bool {
		if !exp.Has(k) {
			t.Errorf("%s: actual map has key %s but expected does not", path, k.String())
		}
		return true
	})
}

func compareLists(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.List) {
	if exp.Len() != act.Len() {
		t.Errorf("%s: expected is list with %d items but actual has %d", path, exp.Len(), act.Len())
	}
	lim := exp.Len()
	if act.Len() < lim {
		lim = act.Len()
	}
	for i := 0; i < lim; i++ {
		compareValues(t, path+"["+strconv.Itoa(i)+"]", fd, exp.Get(i), act.Get(i))
	}
}

func compareValues(t *testing.T, path string, fd protoreflect.FieldDescriptor, exp, act protoreflect.Value) {
	if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
		compareMessages(t, path, exp.Message(), act.Message())
		return
	}

	var eq bool
	switch fd.Kind() {
	case protoreflect.BoolKind:
		eq = exp.Bool() == act.Bool()
	case protoreflect.EnumKind:
		eq = exp.Enum() == act.Enum()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		eq = exp.Int() == act.Int()
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
		eq = exp.Uint() == act.Uint()
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		fx := exp.Float()
		fy := act.Float()
		if math.IsNaN(fx) || math.IsNaN(fy) {
			eq = math.IsNaN(fx) && math.IsNaN(fy)
		} else {
			eq = fx == fy
		}
	case protoreflect.StringKind:
		eq = exp.String() == act.String()
	case protoreflect.BytesKind:
		eq = bytes.Equal(exp.Bytes(), act.Bytes())
	default:
		eq = exp.Interface() == act.Interface()
	}
	if !eq {
		t.Errorf("%s: expected is %v but actual is %v", path, exp, act)
	}
}

func compareUnknown(t *testing.T, path string, exp, act protoreflect.RawFields) {
	if bytes.Equal(exp, act) {
		return
	}

	mexp := make(map[protoreflect.FieldNumber]protoreflect.RawFields)
	mact := make(map[protoreflect.FieldNumber]protoreflect.RawFields)
	for len(exp) > 0 {
		fnum, _, n := protowire.ConsumeField(exp)
		mexp[fnum] = append(mexp[fnum], exp[:n]...)
		exp = exp[n:]
	}
	for len(act) > 0 {
		fnum, _, n := protowire.ConsumeField(act)
		bact := act[:n]
		mact[fnum] = append(mact[fnum], bact...)
		if bexp, ok := mexp[fnum]; !ok {
			t.Errorf("%s: expected has data for unknown field with tag %d but actual does not", path, fnum)
		} else if !bytes.Equal(bexp, bact) {
			t.Errorf("%s: expected has %v for unknown field with tag %d but actual has %v", path, bexp, fnum, bact)
		}
		act = act[n:]
	}
	for fnum := range mexp {
		_, ok := mact[fnum]
		if !ok {
			t.Errorf("%s: actual has data for unknown field with tag %d but expected does not", path, fnum)
		}
	}
}
