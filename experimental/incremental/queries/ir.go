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
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// IR is an [incremental.Query] for the lowered IR of a Protobuf file.
//
// IR queries with different Openers are considered distinct.
type IR struct {
	source.Opener // Must be comparable.
	*ir.Session
	Path string

	// Used for tracking if this IR request was triggered by an import, for
	// constructing a cycle error. This is not part of the query's key.
	request ast.DeclImport
}

var _ incremental.Query[ir.File] = IR{}

// Key implements [incremental.Query].
func (i IR) Key() any {
	type key struct {
		o    source.Opener
		s    *ir.Session
		path string
	}
	return key{i.Opener, i.Session, i.Path}
}

// Execute implements [incremental.Query].
func (i IR) Execute(t *incremental.Task) (ir.File, error) {
	t.Report().Options.Stage += stageIR

	r, err := incremental.Resolve(t, AST{
		Opener: i.Opener,
		Path:   i.Path,
	})
	if err != nil {
		return ir.File{}, err
	}
	file := r[0].Value

	// Resolve all of the imports in the AST.
	queries := make([]incremental.Query[ir.File], iterx.Count(file.Imports()))
	errors := make([]error, len(queries))
	for j, decl := range iterx.Enumerate(file.Imports()) {
		path, ok := decl.ImportPath().AsLiteral().AsString()
		path = ir.CanonicalizeFilePath(path)

		if !ok { // Already legalized in parser.legalizeImport()
			continue
		}

		r, err := incremental.Resolve(t, File{
			Opener:      i.Opener,
			Path:        path,
			ReportError: false,
		})
		if err != nil {
			return ir.File{}, err
		}

		if err := r[0].Fatal; err != nil {
			queries[j] = incremental.ZeroQuery[ir.File]{}
			errors[j] = r[0].Fatal
			continue
		}

		queries[j] = IR{
			Opener:  i.Opener,
			Session: i.Session,
			Path:    path,
			request: decl,
		}
	}
	//nolint:makezero // False positive.
	queries = append(queries, IR{
		Opener:  i.Opener,
		Session: i.Session,
		Path:    ir.DescriptorProtoPath,
	})
	//nolint:makezero // False positive.
	errors = append(errors, nil)

	imports, err := incremental.Resolve(t, queries...)
	if err != nil {
		return ir.File{}, err
	}

	importer := func(n int, _ string, _ ast.DeclImport) (ir.File, error) {
		if n == -1 {
			// The lowering code will call the importer with n == -1 if it needs
			// descriptor.proto but it isn't imported (transitively).
			n = len(queries) - 1
		}

		result := imports[n]
		switch err := result.Fatal.(type) {
		case nil:
			return result.Value, errors[n]

		case *incremental.ErrCycle:
			// We need to walk the cycle and extract which imports are
			// responsible for the failure.
			cyc := new(ir.ErrCycle)
			for _, q := range err.Cycle {
				irq, ok := incremental.AsTyped[IR](q)
				if !ok {
					continue
				}
				if !irq.request.IsZero() {
					cyc.Cycle = append(cyc.Cycle, irq.request)
				}
			}

			return ir.File{}, cyc

		default:
			return ir.File{}, err
		}
	}

	ir, _ := i.Session.Lower(file, t.Report(), importer)
	return ir, nil
}
