package ast_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
)

func TestASTRoundTrips(t *testing.T) {
	err := filepath.Walk("../internal/testprotos", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".proto" {
			t.Run(path, func(t *testing.T) {
				data, err := ioutil.ReadFile(path)
				if !assert.Nil(t, err, "%v", err) {
					return
				}
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
			})
		}
		return nil
	})
	assert.Nil(t, err, "%v", err)
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

	//err = printComments(sw, file.FinalComments)
	//if err != nil {
	//	return err
	//}
	//_, err = sw.WriteString(file.FinalWhitespace)
	//return err

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

// many io.Writer impls also provide a string-based method
type stringWriter interface {
	WriteString(s string) (n int, err error)
}

// adapter, in case the given writer does NOT provide a string-based method
type strWriter struct {
	io.Writer
}

func (s *strWriter) WriteString(str string) (int, error) {
	if str == "" {
		return 0, nil
	}
	return s.Write([]byte(str))
}
