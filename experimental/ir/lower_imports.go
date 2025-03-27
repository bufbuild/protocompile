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

package ir

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Importer is a function that resolves the nth import of an [ast.File] being
// lowered.
//
// If a cycle is encountered, returns the cycle of import statements that caused
// it, starting from decl and ending when the currently lowered file is
// imported.
//
// [Session.Lower] may not call this function on all imports; only those for
// which it needs the caller to resolve a [File] for it.
type Importer func(n int, path string, decl ast.DeclImport) (File, ErrCycle[ast.DeclImport])

// buildImports builds the transitive imports table.
func buildImports(f File, r *report.Report, importer Importer) {
	c := f.Context()
	dedup := make(intern.Set, iterx.Count2(f.AST().Imports()))

	for i, imp := range f.AST().Imports() {
		path, ok := imp.ImportPath().AsLiteral().AsString()
		if !ok {
			continue // Already legalized in parser.legalizeImport()
		}

		file, cycle := importer(i, path, imp)
		if cycle != nil {
			diagnoseCycle(r, path, cycle)
			continue
		}

		if !dedup.AddID(file.InternedPath()) {
			// Duplicates are diagnosed in the legalizer.
			continue
		}

		c.imports.AddDirect(Import{
			File:   file,
			Public: imp.IsPublic(),
			Weak:   imp.IsWeak(),
		})
	}

	// Having found all of the imports that are not cyclic, we now need to pull
	// in all of *their* transitive imports.
	c.imports.Recurse(dedup)
}

// ErrCycle is an error indicating that a cycle has occurred during processing.
//
// The first and last elements of this slice should be equal.
type ErrCycle[T any] []T

// diagnoseCycle generates a diagnostic for an import cycle, showing each
// import contributing to the cycle in turn.
func diagnoseCycle(r *report.Report, path string, cycle ErrCycle[ast.DeclImport]) {
	err := r.Errorf("encountered cycle while importing %q", path)

	for i, imp := range cycle {
		var message string
		path, ok := imp.ImportPath().AsLiteral().AsString()
		if ok {
			switch i {
			case 0:
				message = "imported here"
			case len(cycle) - 1:
				message = fmt.Sprintf("which imports %q, completing the cycle", path)
			default:
				message = fmt.Sprintf("which imports %q", path)
			}
		}
		err.Apply(report.Snippetf(imp, "%v", message))
	}
}
