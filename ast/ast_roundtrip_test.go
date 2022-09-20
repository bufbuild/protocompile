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

package ast_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

func TestASTRoundTrips(t *testing.T) {
	t.Parallel()
	err := filepath.Walk("../internal/testdata", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".proto" {
			t.Run(path, func(t *testing.T) {
				t.Parallel()
				data, err := os.ReadFile(path)
				if !assert.Nil(t, err, "%v", err) {
					return
				}
				testASTRoundTrip(t, path, data)
			})
		}
		return nil
	})
	assert.Nil(t, err, "%v", err)
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		testASTRoundTrip(t, "empty", []byte(`
		// this file has no lexical elements, just this one comment
		`))
	})
}

func testASTRoundTrip(t *testing.T, path string, data []byte) {
	filename := filepath.Base(path)
	root, err := parser.Parse(filename, bytes.NewReader(data), reporter.NewHandler(nil))
	if !assert.Nil(t, err) {
		return
	}
	var buf bytes.Buffer
	err = printAST(&buf, root)
	if assert.Nil(t, err, "%v", err) {
		// see if file survived round trip!
		assert.Equal(t, string(data), buf.String())
	}
}

// printAST prints the given AST node to the given output. This operation
// basically walks the AST and, for each TerminalNode, prints the node's
// leading comments, leading whitespace, the node's raw text, and then
// any trailing comments. If the given node is a *FileNode, it will then
// also print the file's FinalComments and FinalWhitespace.
func printAST(w io.Writer, file *ast.FileNode) error {
	sw, ok := w.(stringWriter)
	if !ok {
		sw = &strWriter{w}
	}
	err := ast.Walk(file, &ast.SimpleVisitor{
		DoVisitTerminalNode: func(token ast.TerminalNode) error {
			info := file.NodeInfo(token)
			if err := printComments(sw, info.LeadingComments()); err != nil {
				return err
			}

			if _, err := sw.WriteString(info.LeadingWhitespace()); err != nil {
				return err
			}

			if _, err := sw.WriteString(info.RawText()); err != nil {
				return err
			}

			return printComments(sw, info.TrailingComments())
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func printComments(sw stringWriter, comments ast.Comments) error {
	for i := 0; i < comments.Len(); i++ {
		comment := comments.Index(i)
		if _, err := sw.WriteString(comment.LeadingWhitespace()); err != nil {
			return err
		}
		if _, err := sw.WriteString(comment.RawText()); err != nil {
			return err
		}
	}
	return nil
}

// many io.Writer impls also provide a string-based method.
type stringWriter interface {
	WriteString(s string) (n int, err error)
}

// adapter, in case the given writer does NOT provide a string-based method.
type strWriter struct {
	io.Writer
}

func (s *strWriter) WriteString(str string) (int, error) {
	if str == "" {
		return 0, nil
	}
	return s.Write([]byte(str))
}
