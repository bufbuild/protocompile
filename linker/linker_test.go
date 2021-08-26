package linker_test

import (
	"context"
	"fmt"
	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
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

	files := loadDescriptorSet(t, "../internal/testprotos/desc_test_complex.protoset", fds.AsResolver())
	res := fds[0].(linker.Result)
	exp := files.FindFileByPath(res.Path())
	checkFiles(t, res, exp, map[string]struct{}{})
}

func loadDescriptorSet(t *testing.T, path string, res linker.Resolver) linker.Files {
	data, err := ioutil.ReadFile(path)
	if !assert.Nil(t, err) {
		t.Fail()
	}
	var fdset descriptorpb.FileDescriptorSet
	err = proto.UnmarshalOptions{Resolver: res}.Unmarshal(data, &fdset)
	if !assert.Nil(t, err) {
		t.Fail()
	}

	var files linker.Files
	h := reporter.NewHandler(nil)
	for _, fd := range fdset.File {
		r, err := linker.Link(parser.ResultWithoutAST(fd), files, nil, h)
		if !assert.Nil(t, err) {
			t.Fail()
		}
		files = append(files, r)
	}

	return files
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

		exp, err := protoregistry.GlobalFiles.FindFileByPath(name)
		if !assert.Nil(t, err) {
			continue
		}

		res := fds[0].(linker.Result)
		checkFiles(t, res, exp, map[string]struct{}{})
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

	data, err := ioutil.ReadFile("../internal/testprotos/desc_test_proto3_optional.protoset")
	if !assert.Nil(t, err) {
		return
	}
	var fdset descriptorpb.FileDescriptorSet
	err = proto.UnmarshalOptions{Resolver: fds.AsResolver()}.Unmarshal(data, &fdset)
	if !assert.Nil(t, err) {
		return
	}

	var files linker.Files
	h := reporter.NewHandler(nil)
	for _, fd := range fdset.File {
		r, err := linker.Link(parser.ResultWithoutAST(fd), files, nil, h)
		if !assert.Nil(t, err) {
			return
		}
		files = append(files, r)
	}
	exp := files.FindFileByPath(fds[0].Path())
	if !assert.NotNil(t, exp) {
		return
	}

	res := fds[0].(linker.Result)
	checkFiles(t, res, exp, map[string]struct{}{})
}

func checkFiles(t *testing.T, act, exp protoreflect.FileDescriptor, checked map[string]struct{}) {
	if _, ok := checked[act.Path()]; ok {
		// already checked
		return
	}
	checked[act.Path()] = struct{}{}

	expProto := toProto(exp)
	actProto := toProto(act)
	checkFileDescriptor(t, actProto, expProto)

	for i := 0; i < act.Imports().Len(); i++ {
		checkFiles(t, act.Imports().Get(i), exp.Imports().Get(i), checked)
	}
}

func checkFileDescriptor(t *testing.T, act, exp *descriptorpb.FileDescriptorProto) {
	assert.True(t, proto.Equal(exp, act), "linked descriptor did not match output from protoc:\nwanted: %s\ngot: %s", toString(exp), toString(act))

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

func TestLinkerValidation(t *testing.T) {
	testCases := []struct {
		input  map[string]string
		errMsg string
	}{
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
			`foo.proto:1:8: cycle found in imports: "foo.proto" -> "foo2.proto" -> "foo.proto"`,
		},
		{
			map[string]string{
				"foo.proto": "message foo {} enum foo { V = 0; }",
			},
			"foo.proto:1:16: duplicate symbol foo: already defined as message",
		},
		{
			map[string]string{
				"foo.proto": "message foo { optional string a = 1; optional string a = 2; }",
			},
			"foo.proto:1:38: duplicate symbol foo.a: already defined as field",
		},
		{
			map[string]string{
				"foo.proto":  "message foo {}",
				"foo2.proto": "enum foo { V = 0; }",
			},
			"foo2.proto:1:1: duplicate symbol foo: already defined as message in \"foo.proto\"",
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
			"foo.proto:1:106: field b: duplicate extension: a and b are both using tag 1",
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
			"foo.proto:1:70: field fu.baz.foobar.a: unknown type baz",
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
					message Bar {
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
						rpc Bar(Bar) returns (Bar) {
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
			"foo.proto:6:8: option (f): non-repeated option field f already set",
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
		} else if err.Error() != tc.errMsg {
			t.Errorf("case %d: expecting validation error %q; instead got: %q", i, tc.errMsg, err)
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

