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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
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

// ErrCycle is an error indicating that a cycle has occurred during processing.
type ErrCycle[T any] []T

// buildImports builds the transitive imports table.
func buildImports(f File, r *report.Report, importer Importer) {
	c := f.Context()
	dedup := make(map[intern.ID]struct{}, iterx.Count2(f.AST().Imports()))

	for i, imp := range f.AST().Imports() {
		path, ok := imp.ImportPath().AsLiteral().AsString()
		if !ok {
			continue // Already legalized in parser.legalizeImport()
		}

		file, cycle := importer(i, path, imp)
		if cycle != nil {
			err := r.Errorf("encountered cycle while importing %q", path).Apply(
				report.Snippet(imp),
			)
			for _, imp := range cycle[1 : len(cycle)-1] {
				path, ok := imp.ImportPath().AsLiteral().AsString()
				if !ok {
					err.Apply(report.Snippet(imp))
					continue
				}
				err.Apply(report.Snippetf(imp, "which imports %q", path))
			}
			last, _ := slicesx.Last(cycle)
			err.Apply(report.Snippetf(last, "which imports %q, completing the cycle", path))
		}

		if !mapsx.Add(dedup, file.InternedPath()) {
			continue
		}

		// Figure out where to insert the file into the imports array.
		switch {
		case imp.IsPublic():
			c.file.imports = slices.Insert(c.file.imports, c.file.publicEnd, file)
			c.file.publicEnd++
			c.file.weakEnd++
		case imp.IsWeak():
			c.file.imports = slices.Insert(c.file.imports, c.file.weakEnd, file)
			c.file.weakEnd++
		default:
			c.file.imports = append(c.file.imports, file)
		}
	}

	c.file.importEnd = len(c.file.imports)
	c.file.transPublicEnd = len(c.file.imports)

	// Having found all of the imports that are not cyclic, we now need to pull
	// in all of *their* transitive imports.
	for file := range seq.Values(f.Imports()) {
		for imp := range seq.Values(file.TransitiveImports()) {
			if !mapsx.Add(dedup, imp.File.InternedPath()) {
				continue
			}

			// Transitive imports are public to us if and only if they are
			// imported through a public import.
			if file.Public && imp.Public {
				c.file.imports = slices.Insert(c.file.imports, c.file.transPublicEnd, imp.File)
				c.file.transPublicEnd++
				continue
			}

			c.file.imports = append(c.file.imports, imp.File)
		}
	}
}
