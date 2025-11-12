// Copyright 2020-2025 Buf Technologies, Inc.
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

package queries

import (
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/source"
)

// AST is an [incremental.Query] for the AST of a Protobuf file.
//
// AST queries with different Openers are considered distinct.
type AST struct {
	source.Opener // Must be comparable.
	Path          string
}

var _ incremental.Query[*ast.File] = AST{}

// Key implements [incremental.Query].
//
// The key for a Contents query is the query itself. This means that a single
// [incremental.Executor] can host Contents queries for multiple Openers. It
// also means that the Openers must all be comparable. As the [Opener]
// documentation states, implementations should take a pointer receiver so that
// comparison uses object identity.
func (a AST) Key() any {
	return a
}

// Execute implements [incremental.Query].
func (a AST) Execute(t *incremental.Task) (*ast.File, error) {
	t.Report().Options.Stage += stageAST

	r, err := incremental.Resolve(t, File{
		Opener:      a.Opener,
		Path:        a.Path,
		ReportError: true,
	})
	if err != nil {
		return nil, err
	}
	if r[0].Fatal != nil {
		return nil, r[0].Fatal
	}

	file, _ := parser.Parse(a.Path, r[0].Value, t.Report())
	return file, nil
}
