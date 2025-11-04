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
	"github.com/bufbuild/protocompile/experimental/report"
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

var _ incremental.Query[*ir.File] = IR{}

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
func (i IR) Execute(t *incremental.Task) (*ir.File, error) {
	t.Report().Options.Stage += stageIR

	r, err := incremental.Resolve(t, AST{
		Opener: i.Opener,
		Path:   i.Path,
	})
	if err != nil {
		return nil, err
	}
	file := r[0].Value

	// Check for descriptor.proto in the opener. If it's not present, that's
	// going to produce a mess of weird errors, and we want this to be an ICE.
	dp, err := incremental.Resolve(t, File{
		Opener:      i.Opener,
		Path:        ir.DescriptorProtoPath,
		ReportError: false,
	})
	if err != nil {
		return nil, err
	}
	if dp[0].Fatal != nil {
		t.Report().Fatalf("could not import `%s`", ir.DescriptorProtoPath).Apply(
			report.Notef("protocompile is not configured correctly"),
			report.Helpf("`descriptor.proto` must always be available, since it is "+
				"required for correctly implementing Protobuf's semantics. "+
				"If you are using protocompile as a library, you may be missing a "+
				"source.WKTs() in your source.Opener setup."),
		)
		return nil, dp[0].Fatal
	}

	// Resolve all of the imports in the AST.
	queries := make([]incremental.Query[*ir.File],
		// Preallocate for one extra query here, corresponding to the
		// descriptor.proto query.
		iterx.Count(file.Imports())+1)
	errors := make([]error, len(queries))
	for j, decl := range iterx.Enumerate(file.Imports()) {
		lit := decl.ImportPath().AsLiteral().AsString()
		path := lit.Text()
		path = ir.CanonicalizeFilePath(path)

		if lit.IsZero() {
			// The import path is already legalized in [parser.legalizeImport()], if it is not
			// a valid path, we just set a [incremental.ZeroQuery] so that we don't get a nil
			// query for index j.
			queries[j] = incremental.ZeroQuery[*ir.File]{}
			continue
		}

		r, err := incremental.Resolve(t, File{
			Opener:      i.Opener,
			Path:        path,
			ReportError: false,
		})
		if err != nil {
			return nil, err
		}

		if err := r[0].Fatal; err != nil {
			queries[j] = incremental.ZeroQuery[*ir.File]{}
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

	queries[len(queries)-1] = IR{
		Opener:  i.Opener,
		Session: i.Session,
		Path:    ir.DescriptorProtoPath,
	}

	imports, err := incremental.Resolve(t, queries...)
	if err != nil {
		return nil, err
	}

	importer := func(n int, _ string, _ ast.DeclImport) (*ir.File, error) {
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

			return nil, cyc

		default:
			return nil, err
		}
	}

	ir, _ := i.Session.Lower(file, t.Report(), importer)
	return ir, nil
}
