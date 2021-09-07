package ast_test

import (
	"bytes"
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
				err = ast.Print(&buf, root)
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
