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

package options_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/options"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

type ident string
type aggregate string

func TestOptionsInUnlinkedFiles(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		contents         string
		uninterpreted    map[string]interface{}
		checkInterpreted func(*testing.T, *descriptorpb.FileDescriptorProto)
	}{
		{
			// file options
			contents: `option go_package = "foo.bar"; option (must.link) = "FOO";`,
			uninterpreted: map[string]interface{}{
				"test.proto:(must.link)": "FOO",
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.Equal(t, "foo.bar", fd.GetOptions().GetGoPackage())
			},
		},
		{
			// message options
			contents: `message Test { option (must.link) = 1.234; option deprecated = true; }`,
			uninterpreted: map[string]interface{}{
				"Test:(must.link)": 1.234,
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.True(t, fd.GetMessageType()[0].GetOptions().GetDeprecated())
			},
		},
		{
			// field options and pseudo-options
			contents: `message Test { optional string uid = 1 [(must.link) = 10101, (must.link) = 20202, default = "fubar", json_name = "UID", deprecated = true]; }`,
			uninterpreted: map[string]interface{}{
				"Test.uid:(must.link)":   10101,
				"Test.uid:(must.link)#1": 20202,
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.Equal(t, "fubar", fd.GetMessageType()[0].GetField()[0].GetDefaultValue())
				assert.Equal(t, "UID", fd.GetMessageType()[0].GetField()[0].GetJsonName())
				assert.True(t, fd.GetMessageType()[0].GetField()[0].GetOptions().GetDeprecated())
			},
		},
		{
			// field where default is uninterpretable
			contents: `enum TestEnum{ ZERO = 0; ONE = 1; } message Test { optional TestEnum uid = 1 [(must.link) = {foo: bar}, default = ONE, json_name = "UID", deprecated = true]; }`,
			uninterpreted: map[string]interface{}{
				"Test.uid:(must.link)": aggregate("foo : bar"),
				"Test.uid:default":     ident("ONE"),
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.Equal(t, "UID", fd.GetMessageType()[0].GetField()[0].GetJsonName())
				assert.True(t, fd.GetMessageType()[0].GetField()[0].GetOptions().GetDeprecated())
			},
		},
		{
			// one-of options
			contents: `message Test { oneof x { option (must.link) = true; option deprecated = true; string uid = 1; uint64 nnn = 2; } }`,
			uninterpreted: map[string]interface{}{
				"Test.x:(must.link)": ident("true"),
				"Test.x:deprecated":  ident("true"), // one-ofs do not have deprecated option :/
			},
		},
		{
			// extension range options
			contents: `message Test { extensions 100 to 200 [(must.link) = "foo", deprecated = true]; }`,
			uninterpreted: map[string]interface{}{
				"Test.100-200:(must.link)": "foo",
				"Test.100-200:deprecated":  ident("true"), // extension ranges do not have deprecated option :/
			},
		},
		{
			// enum options
			contents: `enum Test { option allow_alias = true; option deprecated = true; option (must.link) = 123.456; ZERO = 0; ZILCH = 0; }`,
			uninterpreted: map[string]interface{}{
				"Test:(must.link)": 123.456,
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.True(t, fd.GetEnumType()[0].GetOptions().GetDeprecated())
				assert.True(t, fd.GetEnumType()[0].GetOptions().GetAllowAlias())
			},
		},
		{
			// enum value options
			contents: `enum Test { ZERO = 0 [deprecated = true, (must.link) = -222]; }`,
			uninterpreted: map[string]interface{}{
				"Test.ZERO:(must.link)": -222,
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.True(t, fd.GetEnumType()[0].GetValue()[0].GetOptions().GetDeprecated())
			},
		},
		{
			// service options
			contents: `service Test { option deprecated = true; option (must.link) = {foo:1, foo:2, bar:3}; }`,
			uninterpreted: map[string]interface{}{
				"Test:(must.link)": aggregate("foo : 1 , foo : 2 , bar : 3"),
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.True(t, fd.GetService()[0].GetOptions().GetDeprecated())
			},
		},
		{
			// method options
			contents: `import "google/protobuf/empty.proto"; service Test { rpc Foo (google.protobuf.Empty) returns (google.protobuf.Empty) { option deprecated = true; option (must.link) = FOO; } }`,
			uninterpreted: map[string]interface{}{
				"Test.Foo:(must.link)": ident("FOO"),
			},
			checkInterpreted: func(t *testing.T, fd *descriptorpb.FileDescriptorProto) {
				assert.True(t, fd.GetService()[0].GetMethod()[0].GetOptions().GetDeprecated())
			},
		},
	}

	for i, tc := range testCases {
		h := reporter.NewHandler(nil)
		ast, err := parser.Parse("test.proto", strings.NewReader(tc.contents), h)
		if !assert.Nil(t, err, "case #%d failed to parse", i) {
			continue
		}
		res, err := parser.ResultFromAST(ast, true, h)
		if !assert.Nil(t, err, "case #%d failed to produce descriptor proto", i) {
			continue
		}
		_, err = options.InterpretUnlinkedOptions(res)
		if !assert.Nil(t, err, "case #%d failed to interpret options", i) {
			continue
		}
		actual := map[string]interface{}{}
		buildUninterpretedMapForFile(res.FileDescriptorProto(), actual)
		assert.Equal(t, tc.uninterpreted, actual, "case #%d resulted in wrong uninterpreted options", i)
		if tc.checkInterpreted != nil {
			tc.checkInterpreted(t, res.FileDescriptorProto())
		}
	}
}

func buildUninterpretedMapForFile(fd *descriptorpb.FileDescriptorProto, opts map[string]interface{}) {
	buildUninterpretedMap(fd.GetName(), fd.GetOptions().GetUninterpretedOption(), opts)
	for _, md := range fd.GetMessageType() {
		buildUninterpretedMapForMessage(fd.GetPackage(), md, opts)
	}
	for _, extd := range fd.GetExtension() {
		buildUninterpretedMap(qualify(fd.GetPackage(), extd.GetName()), extd.GetOptions().GetUninterpretedOption(), opts)
	}
	for _, ed := range fd.GetEnumType() {
		buildUninterpretedMapForEnum(fd.GetPackage(), ed, opts)
	}
	for _, sd := range fd.GetService() {
		svcFqn := qualify(fd.GetPackage(), sd.GetName())
		buildUninterpretedMap(svcFqn, sd.GetOptions().GetUninterpretedOption(), opts)
		for _, mtd := range sd.GetMethod() {
			buildUninterpretedMap(qualify(svcFqn, mtd.GetName()), mtd.GetOptions().GetUninterpretedOption(), opts)
		}
	}
}

func buildUninterpretedMapForMessage(qual string, md *descriptorpb.DescriptorProto, opts map[string]interface{}) {
	fqn := qualify(qual, md.GetName())
	buildUninterpretedMap(fqn, md.GetOptions().GetUninterpretedOption(), opts)
	for _, fld := range md.GetField() {
		buildUninterpretedMap(qualify(fqn, fld.GetName()), fld.GetOptions().GetUninterpretedOption(), opts)
	}
	for _, ood := range md.GetOneofDecl() {
		buildUninterpretedMap(qualify(fqn, ood.GetName()), ood.GetOptions().GetUninterpretedOption(), opts)
	}
	for _, extr := range md.GetExtensionRange() {
		buildUninterpretedMap(qualify(fqn, fmt.Sprintf("%d-%d", extr.GetStart(), extr.GetEnd()-1)), extr.GetOptions().GetUninterpretedOption(), opts)
	}
	for _, nmd := range md.GetNestedType() {
		buildUninterpretedMapForMessage(fqn, nmd, opts)
	}
	for _, extd := range md.GetExtension() {
		buildUninterpretedMap(qualify(fqn, extd.GetName()), extd.GetOptions().GetUninterpretedOption(), opts)
	}
	for _, ed := range md.GetEnumType() {
		buildUninterpretedMapForEnum(fqn, ed, opts)
	}
}

func buildUninterpretedMapForEnum(qual string, ed *descriptorpb.EnumDescriptorProto, opts map[string]interface{}) {
	fqn := qualify(qual, ed.GetName())
	buildUninterpretedMap(fqn, ed.GetOptions().GetUninterpretedOption(), opts)
	for _, evd := range ed.GetValue() {
		buildUninterpretedMap(qualify(fqn, evd.GetName()), evd.GetOptions().GetUninterpretedOption(), opts)
	}
}

func buildUninterpretedMap(prefix string, uos []*descriptorpb.UninterpretedOption, opts map[string]interface{}) {
	for _, uo := range uos {
		parts := make([]string, len(uo.GetName()))
		for i, np := range uo.GetName() {
			if np.GetIsExtension() {
				parts[i] = fmt.Sprintf("(%s)", np.GetNamePart())
			} else {
				parts[i] = np.GetNamePart()
			}
		}
		uoName := fmt.Sprintf("%s:%s", prefix, strings.Join(parts, "."))
		key := uoName
		i := 0
		for {
			if _, ok := opts[key]; !ok {
				break
			}
			i++
			key = fmt.Sprintf("%s#%d", uoName, i)
		}
		var val interface{}
		switch {
		case uo.AggregateValue != nil:
			val = aggregate(uo.GetAggregateValue())
		case uo.IdentifierValue != nil:
			val = ident(uo.GetIdentifierValue())
		case uo.DoubleValue != nil:
			val = uo.GetDoubleValue()
		case uo.PositiveIntValue != nil:
			val = int(uo.GetPositiveIntValue())
		case uo.NegativeIntValue != nil:
			val = int(uo.GetNegativeIntValue())
		default:
			val = string(uo.GetStringValue())
		}
		opts[key] = val
	}
}

func qualify(qualifier, name string) string {
	if qualifier == "" {
		return name
	}
	return qualifier + "." + name
}

func TestOptionsEncoding(t *testing.T) {
	t.Parallel()
	testCases := map[string]string{
		"proto2":   "options/test.proto",
		"proto3":   "options/test_proto3.proto",
		"defaults": "desc_test_defaults.proto",
	}
	for syntax, file := range testCases {
		file := file // must not capture loop variable below, for thread safety
		t.Run(syntax, func(t *testing.T) {
			t.Parallel()
			fileToCompile := strings.TrimPrefix(file, "options/")
			importPath := "../internal/testdata"
			if fileToCompile != file {
				importPath = "../internal/testdata/options"
			}
			compiler := protocompile.Compiler{
				Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
					ImportPaths: []string{importPath},
				}),
			}
			fds, err := compiler.Compile(context.Background(), fileToCompile)
			var panicErr protocompile.PanicError
			if errors.As(err, &panicErr) {
				t.Logf("panic! %v\n%s", panicErr.Value, panicErr.Stack)
			}
			require.NoError(t, err)

			res, ok := fds[0].(linker.Result)
			require.True(t, ok)
			descriptorSetFile := fmt.Sprintf("../internal/testdata/%sset", file)
			fdset := prototest.LoadDescriptorSet(t, descriptorSetFile, linker.ResolverFromFile(fds[0]))
			prototest.CheckFiles(t, res, fdset, false)

			canonicalProto := res.CanonicalProto()
			actualFdset := &descriptorpb.FileDescriptorSet{
				File: []*descriptorpb.FileDescriptorProto{canonicalProto},
			}
			actualData, err := proto.Marshal(actualFdset)
			require.NoError(t, err)

			// semantic check that unmarshalling the "canonical bytes" results
			// in the same proto as when not using "canonical bytes"
			protoData, err := proto.Marshal(canonicalProto)
			require.NoError(t, err)
			proto.Reset(canonicalProto)
			uOpts := proto.UnmarshalOptions{Resolver: linker.ResolverFromFile(fds[0])}
			err = uOpts.Unmarshal(protoData, canonicalProto)
			require.NoError(t, err)
			if !proto.Equal(res.FileDescriptorProto(), canonicalProto) {
				t.Fatal("canonical proto != proto")
			}

			// drum roll... make sure the bytes match the protoc output
			expectedData, err := os.ReadFile(descriptorSetFile)
			require.NoError(t, err)
			if !bytes.Equal(actualData, expectedData) {
				outputDescriptorSetFile := strings.ReplaceAll(descriptorSetFile, ".proto", ".actual.proto")
				err = os.WriteFile(outputDescriptorSetFile, actualData, 0644)
				if err != nil {
					t.Log("failed to write actual to file")
				}

				t.Fatalf("descriptor set bytes not equal (created file %q with actual bytes)", outputDescriptorSetFile)
			}
		})
	}
}
