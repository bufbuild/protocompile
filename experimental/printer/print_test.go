// Copyright 2020-2024 Buf Technologies, Inc.
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

package printer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/golden"
)

// TODO: make this better later.
func TestChunks(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:      "testdata",
		Refresh:   "PROTOCOMPILE_REFRESH",
		Extension: "proto",
		Outputs: []golden.Output{
			{Extension: "out"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		file, ok := parser.Parse(report.NewFile(path, text), &report.Report{Options: report.Options{Tracing: 10}})
		require.True(t, ok)
		p := printer{}
		p.printFile(file)
		t.Log(p.String())
		outputs[0] = p.String()
	})
}

// func TestASTRoundTrips(t *testing.T) {
// 	t.Parallel()
// 	err := filepath.Walk("../../internal/testdata", func(path string, _ os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if filepath.Ext(path) == ".proto" {
// 			data, err := os.ReadFile(path)
// 			if err != nil {
// 				return err
// 			}
// 			text := string(data)
// 			filename := strings.TrimPrefix(path, "../../internal/testdata/")
// 			t.Run(filename, func(t *testing.T) {
// 				t.Parallel()
// 				testASTRoundTrip(t, path, text)
// 			})
// 		}
// 		return nil
// 	})
// 	assert.NoError(t, err) //nolint:testifylint // we want to continue even if err!=nil
// 	t.Run("empty", func(t *testing.T) {
// 		t.Parallel()
// 		testASTRoundTrip(t, "empty", `
// 		// this file has no lexical elements, just this one comment
// 		`)
// 	})
// }
//
// func testASTRoundTrip(t *testing.T, path string, text string) {
// 	errs := &report.Report{Options: report.Options{Tracing: 10}}
// 	file, ok := parser.Parse(report.NewFile(path, text), errs)
// 	require.True(t, ok)
//
// 	p := printer{}
// 	p.File(file)
// 	// see if file survived round trip!
// 	assert.Equal(t, text, p.String())
// }
//
// func TestModifiedAST(t *testing.T) {
// 	t.Parallel()
//
// 	text := `syntax = "proto3";
// package foo;
// import "bar/baz.proto";
// message Bar {
//    string baz = 1;
// }`
//
// 	injectPos := strings.Index(text, ";") + 1
// 	expect := text[0:injectPos] + `
// import "synth/foo.proto";` + text[injectPos:]
// 	t.Log("expect:", expect)
//
// 	errs := &report.Report{Options: report.Options{Tracing: 10}}
// 	file, ok := parser.Parse(report.NewFile("test.proto", text), errs)
// 	require.True(t, ok)
// 	// Modify the AST
// 	ctx := file.Context()
// 	stream := ctx.Stream()
// 	nodes := ctx.Nodes()
// 	nodes.Root().Imports()(func(i int, imp ast.DeclImport) bool {
// 		fmt.Println("import", i, imp.What())
// 		return true
// 	})
//
// 	// Injects a import stmt.
// 	decImport := nodes.NewDeclImport(ast.DeclImportArgs{
// 		Keyword: stream.NewIdent("import"),
// 		ImportPath: ast.ExprLiteral{
// 			Token: stream.NewString("synth/foo.proto"),
// 		}.AsAny(),
// 		Semicolon: stream.NewPunct(";"),
// 	})
// 	decls := file.Decls()
// 	decls.Insert(1, decImport.AsAny())
//
// 	p := printer{}
// 	assert.NotPanics(t, func() {
// 		p.File(file)
// 	})
// 	// see if file survived round trip!
// 	assert.Equal(t, expect, p.String())
// }
//
// func TestMovedAST(t *testing.T) {
// 	t.Parallel()
//
// 	text := `syntax = "proto3";
// package foo;
// message Foo {
//    string bar = 1;
// }
// message Bar {
//    string baz = 1;
// }`
//
// 	errs := &report.Report{Options: report.Options{Tracing: 10}}
// 	file, ok := parser.Parse(report.NewFile("test.proto", text), errs)
// 	require.True(t, ok)
// 	// Modify the AST.
// 	decls := file.Decls()
// 	node := decls.At(3)
// 	decls.Delete(3)
// 	decls.Insert(2, node)
//
// 	p := printer{}
// 	assert.NotPanics(t, func() {
// 		p.File(file)
// 	})
// 	// see if file survived round trip!
// 	t.Log(p.String())
// 	assert.Equal(t, text, p.String())
// }
