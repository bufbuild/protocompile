// Copyright 2020-2023 Buf Technologies, Inc.
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
	"fmt"
	"math"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/bufbuild/protocompile/protoutil"
)

func TestFields(t *testing.T) {
	t.Parallel()
	fds := prototest.LoadDescriptorSet(t, "../internal/testdata/descriptor_impl_tests.protoset", nil)
	files, err := protodesc.NewFiles(fds)
	require.NoError(t, err)

	testFileNames := []string{
		"desc_test2.proto",
		"desc_test_defaults.proto",
		"desc_test_proto3.proto",
		"desc_test_proto3_optional.proto",
	}
	for _, testFileName := range testFileNames {
		testFileName := testFileName // must not capture loop variable below, for thread safety
		t.Run(testFileName, func(t *testing.T) {
			t.Parallel()
			protocFd, err := files.FindFileByPath(testFileName)
			require.NoError(t, err)

			compiler := protocompile.Compiler{
				Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
					ImportPaths: []string{"../internal/testdata"},
				}),
			}
			results, err := compiler.Compile(context.Background(), testFileName)
			require.NoError(t, err)
			fd := results[0]

			checkAttributes(t, protocFd, fd, fmt.Sprintf("%q", testFileName))
		})
	}
}

func TestUnescape(t *testing.T) {
	t.Parallel()
	fileProto := &descriptorpb.FileDescriptorProto{
		Name:   proto.String("foo.proto"),
		Syntax: proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Foo"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:         proto.String("escaped_bytes"),
						DefaultValue: proto.String(`\p\0\001\02\ab\b\f\n\r\t\v\\\'\"\?\xfe\Xab\Xc\xf\u2192\U0001F389`),
						Type:         (*descriptorpb.FieldDescriptorProto_Type)(proto.Int32(int32(descriptorpb.FieldDescriptorProto_TYPE_BYTES))),
					},
				},
			},
		},
	}
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(protocompile.ResolverFunc(func(path string) (protocompile.SearchResult, error) {
			return protocompile.SearchResult{Proto: fileProto}, nil
		})),
	}
	result, err := compiler.Compile(context.Background(), "foo.proto")
	require.NoError(t, err)
	require.NotNil(t, result)
	field := result.FindFileByPath("foo.proto").Messages().Get(0).Fields().Get(0)
	expected := []byte{'\\', 'p', 0, 1, 2, '\a', 'b', '\b', '\f', '\n', '\r', '\t', '\v', '\\', '\'', '"', '?', 0xfe, 0xab, 0xc, 0xf}
	expected = utf8.AppendRune(expected, 0x2192)
	expected = utf8.AppendRune(expected, 0x0001f389)
	assert.Equal(t, expected, field.Default().Bytes())
}

type container interface {
	Extensions() protoreflect.ExtensionDescriptors
	Messages() protoreflect.MessageDescriptors
}

func checkAttributes(t *testing.T, exp, actual container, path string) {
	checkAttributesInFields(t, exp.Extensions(), actual.Extensions(), fmt.Sprintf("extensions in %s", path))
	if assert.Equal(t, exp.Messages().Len(), actual.Messages().Len()) {
		for i := 0; i < exp.Messages().Len(); i++ {
			expMsg := exp.Messages().Get(i)
			actMsg := actual.Messages().Get(i)
			if !assert.Equal(t, expMsg.Name(), actMsg.Name(), "%s: message name at index %d", path, i) {
				continue
			}
			checkAttributes(t, expMsg, actMsg, fmt.Sprintf("%s.%s", path, expMsg.Name()))
		}
	}

	if expMsg, ok := exp.(protoreflect.MessageDescriptor); ok {
		actMsg, ok := actual.(protoreflect.MessageDescriptor)
		require.True(t, ok)
		checkAttributesInFields(t, expMsg.Fields(), actMsg.Fields(), fmt.Sprintf("fields in %s", path))
		checkAttributesInOneofs(t, expMsg.Oneofs(), actMsg.Oneofs(), fmt.Sprintf("oneofs in %s", path))
	}
}

func checkAttributesInFields(t *testing.T, exp, actual protoreflect.ExtensionDescriptors, where string) {
	if !assert.Equal(t, exp.Len(), actual.Len(), "%s: number of fields", where) {
		return
	}
	for i := 0; i < exp.Len(); i++ {
		expFld := exp.Get(i)
		actFld := actual.Get(i)
		if !assert.Equal(t, expFld.Name(), actFld.Name(), "%s: field name at index %d", where, i) {
			continue
		}

		// default values

		assert.Equal(t, expFld.HasDefault(), actFld.HasDefault(), "%s: field has default at index %d (%s)", where, i, expFld.Name())

		expVal := expFld.Default().Interface()
		actVal := actFld.Default().Interface()
		if fl, ok := expVal.(float32); ok && math.IsNaN(float64(fl)) {
			actFl, actOk := actVal.(float32)
			assert.True(t, actOk && math.IsNaN(float64(actFl)), "%s: field default value should be float32 NaN at index %d (%s): %v", where, i, expFld.Name(), actVal)
		} else if fl, ok := expVal.(float64); ok && math.IsNaN(fl) {
			actFl, actOk := actVal.(float64)
			assert.True(t, actOk && math.IsNaN(actFl), "%s: field default value should be float64 NaN at index %d (%s): %v", where, i, expFld.Name(), actVal)
		} else {
			assert.Equal(t, expFld.Default().Interface(), actFld.Default().Interface(), "%s: field default value at index %d (%s)", where, i, expFld.Name())
		}

		expEnumVal := expFld.DefaultEnumValue()
		actEnumVal := actFld.DefaultEnumValue()
		if expEnumVal == nil {
			assert.Nil(t, actEnumVal, "%s: field default enum value should be nil at index %d (%s)", where, i, expFld.Name())
		} else if assert.NotNil(t, actEnumVal, "%s: field default enum value should not be nil at index %d (%s)", where, i, expFld.Name()) {
			assert.Equal(t, expEnumVal.Name(), actEnumVal.Name(), "%s: field default enum value at index %d (%s)", where, i, expFld.Name())
			assert.Equal(t, expEnumVal.Number(), actEnumVal.Number(), "%s: field default enum value at index %d (%s)", where, i, expFld.Name())
		}

		expFldProto := protoutil.ProtoFromFieldDescriptor(expFld)
		actFldProto := protoutil.ProtoFromFieldDescriptor(actFld)
		if expFldProto.DefaultValue == nil {
			assert.Nil(t, actFldProto.DefaultValue, "%s: field default value should be nil at index %d (%s)", where, i, expFld.Name())
		} else {
			assert.Equal(t, expFldProto.DefaultValue, actFldProto.DefaultValue, "%s: field default value at index %d (%s)", where, i, expFld.Name())
		}

		// proto3 optionals

		assert.Equal(t, expFld.HasOptionalKeyword(), actFld.HasOptionalKeyword(), "%s: field has optional keyword at index %d (%s)", where, i, expFld.Name())
		assert.Equal(t, expFld.HasPresence(), actFld.HasPresence(), "%s: field has presence at index %d (%s)", where, i, expFld.Name())

		if actFld.IsExtension() && actFldProto.GetProto3Optional() {
			// protoc sets proto3_optional to true for extensions w/ explicit optional
			// keyword, so we do, too. BUT the Go runtime ignores it, so its descriptor
			// implementation (as well as the logic to convert descriptor -> proto)
			// is missing it. So we don't bother with this check in this case since we
			// know it would fail. This is a case of the conversion of the standard Go
			// runtime descriptor to proto being lossy :/
			continue
		}
		if expFldProto.Proto3Optional == nil {
			assert.Nil(t, actFldProto.Proto3Optional, "%s: field proto3 optional should be nil at index %d (%s)", where, i, expFld.Name())
		} else {
			assert.Equal(t, expFldProto.Proto3Optional, actFldProto.Proto3Optional, "%s: field proto3 optional at index %d (%s)", where, i, expFld.Name())
		}
	}
}

func checkAttributesInOneofs(t *testing.T, exp, actual protoreflect.OneofDescriptors, where string) {
	if !assert.Equal(t, exp.Len(), actual.Len(), "%s: number of fields", where) {
		return
	}
	for i := 0; i < exp.Len(); i++ {
		expOo := exp.Get(i)
		actOo := actual.Get(i)
		if !assert.Equal(t, expOo.Name(), actOo.Name(), "%s: oneof name at index %d", where, i) {
			continue
		}
	}
}
