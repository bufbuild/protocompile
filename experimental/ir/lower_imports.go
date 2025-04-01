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
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Importer is a callback to resolve the imports of an [ast.File] being
// lowered.
//
// If a cycle is encountered, should return an *[incremental.ErrCycle][[ast.DeclImport]],
// starting from decl and ending when the currently lowered file is imported.
//
// [Session.Lower] may not call this function on all imports; only those for
// which it needs the caller to resolve a [File] for it.
type Importer func(n int, path string, decl ast.DeclImport) (File, error)

// buildImports builds the transitive imports table.
func buildImports(f File, r *report.Report, importer Importer) {
	c := f.Context()
	dedup := make(intern.Set, iterx.Count(f.AST().Imports()))

	for i, imp := range iterx.Enumerate(f.AST().Imports()) {
		path, ok := imp.ImportPath().AsLiteral().AsString()
		if !ok {
			continue // Already legalized in parser.legalizeImport()
		}
		path = caonicalizeImportPath(path, r, imp)
		if path == "" {
			continue
		}

		file, err := importer(i, path, imp)
		switch err := err.(type) {
		case nil:
		case *incremental.ErrCycle[ast.DeclImport]:
			diagnoseCycle(r, err)
			continue
		default:
			if errors.Is(err, fs.ErrNotExist) {
				r.Errorf("imported file does not exist").Apply(
					report.Snippetf(imp, "imported here"),
				)
			} else {

				r.Errorf("could not open imported file: %v", err).Apply(
					report.Snippetf(imp, "imported here"),
				)
			}
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

// diagnoseCycle generates a diagnostic for an import cycle, showing each
// import contributing to the cycle in turn.
func diagnoseCycle(r *report.Report, cycle *incremental.ErrCycle[ast.DeclImport]) {
	path, _ := cycle.Cycle[0].ImportPath().AsLiteral().AsString()
	err := r.Errorf("detected cyclic import while importing %q", path)

	for i, imp := range cycle.Cycle {
		var message string
		path, ok := imp.ImportPath().AsLiteral().AsString()
		if ok {
			switch i {
			case 0:
				message = "imported here"
			case len(cycle.Cycle) - 1:
				message = fmt.Sprintf("...which imports %q, completing the cycle", path)
			default:
				message = fmt.Sprintf("...which imports %q...", path)
			}
		}
		err.Apply(report.Snippetf(imp, "%v", message))
	}
}

// caonicalizeImportPath canonicalizes the path of an import declaration.
//
// This will generate diagnostics for invalid paths. Returns "" for paths that
// cannot be made canonical.
func caonicalizeImportPath(path string, r *report.Report, decl ast.DeclImport) string {
	if path == "" {
		r.Errorf("import path cannot be empty").Apply(
			report.Snippet(decl.ImportPath()),
		)
		return ""
	}

	orig := path
	// Not filepath.ToSlash, since this conversion is file-system independent.
	path = strings.ReplaceAll(path, `\`, `/`)
	if orig != path {
		r.Errorf("import path cannot use `\\` as a path separator").Apply(
			report.Snippetf(decl.ImportPath(), "this path begins with a `%c`", path[0]),
			report.SuggestEdits(decl.ImportPath(), "use `/` as the separator instead", report.Edit{
				Start: 0, End: decl.ImportPath().Span().Len(),
				Replace: strconv.Quote(path),
			}),
			report.Notef("this restriction also applies when compiling on a non-Windows system"),
		)
	}

	isLetter := func(b byte) bool {
		return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
	}

	if len(path) >= 2 && isLetter(path[0]) && path[1] == ':' {
		r.Warnf("import path appears to begin with the Windows drive prefix `%s`", path[:2]).Apply(
			report.Snippet(decl.ImportPath()),
			report.Notef("this is not an error, because `protoc` accepts it, but may result in unexpected behavior on Windows"),
		)
	}

	if strings.HasPrefix(path, "/") {
		r.Errorf("import path cannot be absolute").Apply(
			report.Snippetf(decl.ImportPath(), "this path begins with a `%c`", path[0]),
		)
		return ""
	}

	return path
}
