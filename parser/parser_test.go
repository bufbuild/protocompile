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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/reporter"
)

func TestEmptyParse(t *testing.T) {
	t.Parallel()
	errHandler := reporter.NewHandler(nil)
	ast, err := Parse("foo.proto", bytes.NewReader(nil), errHandler)
	assert.Nil(t, err)
	result, err := ResultFromAST(ast, true, errHandler)
	assert.Nil(t, err)
	fd := result.FileDescriptorProto()
	assert.Equal(t, "foo.proto", fd.GetName())
	assert.Equal(t, 0, len(fd.GetDependency()))
	assert.Equal(t, 0, len(fd.GetMessageType()))
	assert.Equal(t, 0, len(fd.GetEnumType()))
	assert.Equal(t, 0, len(fd.GetExtension()))
	assert.Equal(t, 0, len(fd.GetService()))
}

func TestJunkParse(t *testing.T) {
	t.Parallel()
	errHandler := reporter.NewHandler(nil)
	// inputs that have been found in the past to cause panics by oss-fuzz
	inputs := map[string]string{
		"case-34232": `'';`,
		"case-34238": `.`,
	}
	for name, input := range inputs {
		protoName := fmt.Sprintf("%s.proto", name)
		_, err := Parse(protoName, strings.NewReader(input), errHandler)
		// we expect this to error... but we don't want it to panic
		assert.NotNil(t, err, "junk input should have returned error")
		t.Logf("error from parse: %v", err)
	}
}

func TestSimpleParse(t *testing.T) {
	t.Parallel()
	protos := map[string]Result{}

	// Just verify that we can successfully parse the same files we use for
	// testing. We do a *very* shallow check of what was parsed because we know
	// it won't be fully correct until after linking. (So that will be tested
	// below, where we parse *and* link.)
	res, err := parseFileForTest("../internal/testdata/desc_test1.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test1.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasExtension(fd, "xtm"))
		assert.True(t, hasMessage(fd, "TestMessage"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test2.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test2.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasExtension(fd, "groupx"))
		assert.True(t, hasMessage(fd, "GroupX"))
		assert.True(t, hasMessage(fd, "Frobnitz"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test_defaults.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test_defaults.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasMessage(fd, "PrimitiveDefaults"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test_field_types.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test_field_types.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasEnum(fd, "TestEnum"))
		assert.True(t, hasMessage(fd, "UnaryFields"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test_options.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test_options.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasExtension(fd, "mfubar"))
		assert.True(t, hasEnum(fd, "ReallySimpleEnum"))
		assert.True(t, hasMessage(fd, "ReallySimpleMessage"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test_proto3.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test_proto3.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasEnum(fd, "Proto3Enum"))
		assert.True(t, hasService(fd, "TestService"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/desc_test_wellknowntypes.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/desc_test_wellknowntypes.proto", fd.GetName())
		assert.Equal(t, "testprotos", fd.GetPackage())
		assert.True(t, hasMessage(fd, "TestWellKnownTypes"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/nopkg/desc_test_nopkg.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/nopkg/desc_test_nopkg.proto", fd.GetName())
		assert.Equal(t, "", fd.GetPackage())
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/nopkg/desc_test_nopkg_new.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/nopkg/desc_test_nopkg_new.proto", fd.GetName())
		assert.Equal(t, "", fd.GetPackage())
		assert.True(t, hasMessage(fd, "TopLevel"))
		protos[fd.GetName()] = res
	}

	res, err = parseFileForTest("../internal/testdata/pkg/desc_test_pkg.proto")
	if assert.Nil(t, err, "%v", err) {
		fd := res.FileDescriptorProto()
		assert.Equal(t, "../internal/testdata/pkg/desc_test_pkg.proto", fd.GetName())
		assert.Equal(t, "bufbuild.protocompile.test", fd.GetPackage())
		assert.True(t, hasEnum(fd, "Foo"))
		assert.True(t, hasMessage(fd, "Bar"))
		protos[fd.GetName()] = res
	}
}

func parseFileForTest(filename string) (Result, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	errHandler := reporter.NewHandler(nil)
	res, err := Parse(filename, f, errHandler)
	if err != nil {
		return nil, err
	}
	return ResultFromAST(res, true, errHandler)
}

func hasExtension(fd *descriptorpb.FileDescriptorProto, name string) bool {
	for _, ext := range fd.Extension {
		if ext.GetName() == name {
			return true
		}
	}
	return false
}

func hasMessage(fd *descriptorpb.FileDescriptorProto, name string) bool {
	for _, md := range fd.MessageType {
		if md.GetName() == name {
			return true
		}
	}
	return false
}

func hasEnum(fd *descriptorpb.FileDescriptorProto, name string) bool {
	for _, ed := range fd.EnumType {
		if ed.GetName() == name {
			return true
		}
	}
	return false
}

func hasService(fd *descriptorpb.FileDescriptorProto, name string) bool {
	for _, sd := range fd.Service {
		if sd.GetName() == name {
			return true
		}
	}
	return false
}

func TestAggregateValueInUninterpretedOptions(t *testing.T) {
	t.Parallel()
	res, err := parseFileForTest("../internal/testdata/desc_test_complex.proto")
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	fd := res.FileDescriptorProto()

	// service TestTestService, method UserAuth; first option
	aggregateValue1 := *fd.Service[0].Method[0].Options.UninterpretedOption[0].AggregateValue
	assert.Equal(t, "authenticated : true permission : { action : LOGIN entity : \"client\" }", aggregateValue1)

	// service TestTestService, method Get; first option
	aggregateValue2 := *fd.Service[0].Method[1].Options.UninterpretedOption[0].AggregateValue
	assert.Equal(t, "authenticated : true permission : { action : READ entity : \"user\" }", aggregateValue2)

	// message Another; first option
	aggregateValue3 := *fd.MessageType[4].Options.UninterpretedOption[0].AggregateValue
	assert.Equal(t, "foo : \"abc\" s < name : \"foo\" , id : 123 > , array : [ 1 , 2 , 3 ] , r : [ < name : \"f\" > , { name : \"s\" } , { id : 456 } ] ,", aggregateValue3)

	// message Test.Nested._NestedNested; second option (rept)
	//  (Test.Nested is at index 1 instead of 0 because of implicit nested message from map field m)
	aggregateValue4 := *fd.MessageType[1].NestedType[1].NestedType[0].Options.UninterpretedOption[1].AggregateValue
	assert.Equal(t, "foo : \"goo\" [ foo . bar . Test . Nested . _NestedNested . _garblez ] : \"boo\"", aggregateValue4)
}

func TestBasicSuccess(t *testing.T) {
	t.Parallel()
	r := readerForTestdata(t, "largeproto.proto")
	handler := reporter.NewHandler(nil)

	fileNode, err := Parse("largeproto.proto", r, handler)
	require.NoError(t, err)

	result, err := ResultFromAST(fileNode, true, handler)
	require.NoError(t, err)
	require.NoError(t, handler.Error())

	assert.Equal(t, "proto3", result.AST().Syntax.Syntax.AsString())
}

func BenchmarkBasicSuccess(b *testing.B) {
	r := readerForTestdata(b, "largeproto.proto")
	bs, err := io.ReadAll(r)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.ReportAllocs()
		byteReader := bytes.NewReader(bs)
		handler := reporter.NewHandler(nil)

		fileNode, err := Parse("largeproto.proto", byteReader, handler)
		require.NoError(b, err)

		result, err := ResultFromAST(fileNode, true, handler)
		require.NoError(b, err)
		require.NoError(b, handler.Error())

		assert.Equal(b, "proto3", result.AST().Syntax.Syntax.AsString())
	}
}

func readerForTestdata(t testing.TB, filename string) io.Reader {
	file, err := os.Open(filepath.Join("testdata", filename))
	require.NoError(t, err)

	return file
}

func TestPathological(t *testing.T) {
	t.Parallel()

	// This test verifies that the test cases found in fuzz tests have
	// adequate performance.
	//   https://oss-fuzz.com/testcase-detail/4766256800858112
	//   https://oss-fuzz.com/testcase-detail/4952577018298368
	testCases := map[string]bool{
		"pathological.proto":  true,
		"pathological2.proto": false,
	}
	for fileName := range testCases {
		fileName, canParse := fileName, testCases[fileName] // don't want test func below to capture loop var
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			// Fuzz testing complains if this loop, with 100 iterations, takes longer
			// than 60 seconds. To prevent this test from being too slow, we limit to
			// 3 iterations and no longer than 1 second (which is a stricter deadline).
			allowedDuration := time.Second
			if internal.IsRace {
				// We increase that threshold to 10 seconds when the race detector is enabled.
				// The race detector has been observed to make it take ~8x as long. If coverage
				// is *also* enabled, the test can take 19x as long(!!). But 10s should still
				// be a reasonable limit in practice. (Unfortunately, there doesn't appear to
				// be a way to easily detect if coverage is enabled.)
				allowedDuration = 10 * time.Second
				t.Logf("allowing %v since race detector is enabled", allowedDuration)
			}
			timer := time.AfterFunc(allowedDuration, func() {
				require.Fail(t, "test took too long to execute (> %v)", allowedDuration)
				cancel()
			})
			defer timer.Stop()

			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					break
				}
				r := readerForTestdata(t, fileName)
				handler := reporter.NewHandler(nil)
				fileNode, err := Parse(fileName, r, handler)
				if canParse {
					require.NoError(t, err)
					_, err = ResultFromAST(fileNode, true, handler)
					require.NotNil(t, err)
				} else {
					require.NotNil(t, err)
				}
			}
		})
	}
}
