package parser

import (
	"io"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

//go:generate goyacc -o proto.y.go -p proto proto.y

func Parse(filename string, r io.Reader, handler *reporter.Handler) (*ast.FileNode, error) {

}
