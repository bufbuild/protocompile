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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/reporter"
)

func TestSymbolsPackages(t *testing.T) {
	t.Parallel()

	var s linker.Symbols
	// default/nameless package is the root
	assert.Equal(t, s.Packages(), s.GetPackage(""))

	h := reporter.NewHandler(nil)
	pos := ast.UnknownPos("foo.proto")
	pkg, err := s.ImportPackages(pos, "build.buf.foo.bar.baz", h)
	require.NoError(t, err)
	// new package has nothing in it
	assert.Empty(t, pkg.Children())
	assert.Empty(t, pkg.Files())
	assert.Empty(t, pkg.Symbols())
	assert.Empty(t, pkg.Extensions())

	assert.Equal(t, pkg, s.GetPackage("build.buf.foo.bar.baz"))

	// verify that trie was created correctly:
	//   each package has just one entry, which is its immediate sub-package
	cur := s.Packages()
	pkgNames := []protoreflect.FullName{"build", "build.buf", "build.buf.foo", "build.buf.foo.bar", "build.buf.foo.bar.baz"}
	for _, pkgName := range pkgNames {
		assert.Equal(t, 1, len(cur.Children()))
		assert.Empty(t, cur.Files())
		assert.Equal(t, 1, len(cur.Symbols()))
		assert.Empty(t, cur.Extensions())

		entry, ok := cur.Symbols()[pkgName]
		require.True(t, ok)
		assert.Equal(t, pos, entry.Pos())
		assert.False(t, entry.IsEnumValue())
		assert.True(t, entry.IsPackage())

		next, ok := cur.Children()[pkgName]
		require.True(t, ok)
		require.NotNil(t, next)

		cur = next
	}
	assert.Equal(t, pkg, cur)
}

func TestSymbolsImport(t *testing.T) {
	t.Parallel()

	testProto := `
		syntax = "proto2";
		import "google/protobuf/descriptor.proto";
		package foo.bar;
		message Foo {
			optional string bar = 1;
			repeated int32 baz = 2;
			extensions 10 to 20;
		}
		extend Foo {
			optional float f = 10;
			optional string s = 11;
		}
		extend google.protobuf.FieldOptions {
			optional bytes xtra = 20000;
		}
		`
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"test.proto": testProto,
			}),
		}),
	}
	files, err := compiler.Compile(context.Background(), "test.proto")
	require.NoError(t, err)

	fileAsResult := files[0].(linker.Result)
	fileAsNonResult, err := protodesc.NewFile(fileAsResult.FileDescriptorProto(), protoregistry.GlobalFiles)
	require.NoError(t, err)

	h := reporter.NewHandler(nil)
	testCases := map[string]protoreflect.FileDescriptor{
		"linker.Result":               fileAsResult,
		"protoreflect.FileDescriptor": fileAsNonResult,
	}

	for name, fd := range testCases {
		fd := fd
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var s linker.Symbols
			err := s.Import(fd, h)
			require.NoError(t, err)

			// verify contents of s

			pkg := s.GetPackage("foo.bar")
			syms := pkg.Symbols()
			assert.Equal(t, 6, len(syms))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.bar"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.Foo.baz"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.f"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.s"))
			assert.Contains(t, syms, protoreflect.FullName("foo.bar.xtra"))
			exts := pkg.Extensions()
			assert.Equal(t, 1, len(exts))
			extNums := exts["foo.bar.Foo"]
			assert.Equal(t, 2, len(extNums))
			assert.Contains(t, extNums, protoreflect.FieldNumber(10))
			assert.Contains(t, extNums, protoreflect.FieldNumber(11))

			pkg = s.GetPackage("google.protobuf")
			exts = pkg.Extensions()
			assert.Equal(t, 1, len(exts))
			extNums = exts["google.protobuf.FieldOptions"]
			assert.Equal(t, 1, len(extNums))
			assert.Contains(t, extNums, protoreflect.FieldNumber(20000))
		})
	}
}

func TestSymbolExtensions(t *testing.T) {
	t.Parallel()

	var s linker.Symbols

	_, err := s.ImportPackages(ast.UnknownPos("foo.proto"), "foo.bar", reporter.NewHandler(nil))
	require.NoError(t, err)
	_, err = s.ImportPackages(ast.UnknownPos("google/protobuf/descriptor.proto"), "google.protobuf", reporter.NewHandler(nil))
	require.NoError(t, err)

	addExt := func(pkg, extendee protoreflect.FullName, num protoreflect.FieldNumber) error {
		return s.AddExtension(pkg, extendee, num, ast.UnknownPos("foo.proto"), reporter.NewHandler(nil))
	}

	t.Run("mismatch", func(t *testing.T) {
		t.Parallel()
		err := addExt("bar.baz", "foo.bar.Foo", 11)
		require.ErrorContains(t, err, "does not match package")
	})
	t.Run("missing package", func(t *testing.T) {
		t.Parallel()
		err := addExt("bar.baz", "bar.baz.Bar", 11)
		require.ErrorContains(t, err, "missing package symbols")
	})

	err = addExt("foo.bar", "foo.bar.Foo", 11)
	require.NoError(t, err)
	err = addExt("foo.bar", "foo.bar.Foo", 12)
	require.NoError(t, err)

	err = addExt("foo.bar", "foo.bar.Foo", 11)
	require.ErrorContains(t, err, "already defined")

	err = addExt("google.protobuf", "google.protobuf.FileOptions", 10101)
	require.NoError(t, err)
	err = addExt("google.protobuf", "google.protobuf.FieldOptions", 10101)
	require.NoError(t, err)
	err = addExt("google.protobuf", "google.protobuf.MessageOptions", 10101)
	require.NoError(t, err)

	// verify contents of s

	pkg := s.GetPackage("foo.bar")
	exts := pkg.Extensions()
	assert.Equal(t, 1, len(exts))
	extNums := exts["foo.bar.Foo"]
	assert.Equal(t, 2, len(extNums))
	assert.Contains(t, extNums, protoreflect.FieldNumber(11))
	assert.Contains(t, extNums, protoreflect.FieldNumber(12))

	pkg = s.GetPackage("google.protobuf")
	exts = pkg.Extensions()
	assert.Equal(t, 3, len(exts))
	assert.Contains(t, exts, protoreflect.FullName("google.protobuf.FileOptions"))
	assert.Contains(t, exts, protoreflect.FullName("google.protobuf.FieldOptions"))
	assert.Contains(t, exts, protoreflect.FullName("google.protobuf.MessageOptions"))
	extNums = exts["google.protobuf.FileOptions"]
	assert.Equal(t, 1, len(extNums))
	assert.Contains(t, extNums, protoreflect.FieldNumber(10101))
	extNums = exts["google.protobuf.FieldOptions"]
	assert.Equal(t, 1, len(extNums))
	assert.Contains(t, extNums, protoreflect.FieldNumber(10101))
	extNums = exts["google.protobuf.MessageOptions"]
	assert.Equal(t, 1, len(extNums))
	assert.Contains(t, extNums, protoreflect.FieldNumber(10101))
}
