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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/cycle"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/intern"
)

const DescriptorProtoPath = "google/protobuf/descriptor.proto"

// Importer is a callback to resolve the imports of an [ast.File] being
// lowered.
//
// If a cycle is encountered, should return an *[incremental.ErrCycle],
// starting from decl and ending when the currently lowered file is imported.
//
// [Session.Lower] may not call this function on all imports; only those for
// which it needs the caller to resolve a [File] for it.
//
// This function will also be called with [DescriptorProtoPath] if it isn't
// transitively imported by the lowered file, with an index value of -1.
// Returning an error or a zero file will trigger an ICE.
type Importer func(n int, path string, decl ast.DeclImport) (*File, error)

// ErrCycle is returned by an [Importer] when encountering an import cycle.
type ErrCycle = cycle.Error[ast.DeclImport]

// buildImports builds the transitive imports table.
func buildImports(file *File, r *report.Report, importer Importer) {
	dedup := make(intern.Map[ast.DeclImport], iterx.Count(file.AST().Imports()))

	for i, imp := range iterx.Enumerate(file.AST().Imports()) {
		lit := imp.ImportPath().AsLiteral().AsString()
		if lit.IsZero() {
			continue // Already legalized in parser.legalizeImport()
		}
		path := canonicalizeImportPath(lit.Text(), r, imp)
		if path == "" {
			continue
		}

		imported, err := importer(i, path, imp)

		var cycle *ErrCycle
		switch {
		case err == nil:

		case errors.As(err, &cycle):
			diagnoseCycle(r, cycle)
			continue
		case errors.Is(err, fs.ErrNotExist):
			r.Errorf("imported file does not exist").Apply(
				report.Snippetf(imp, "imported here"),
			)
			continue
		default:
			r.Errorf("could not open imported file: %v", err).Apply(
				report.Snippetf(imp, "imported here"),
			)
			continue
		}

		if prev, ok := dedup.AddID(imported.InternedPath(), imp); !ok {
			d := r.Errorf("file imported multiple times").Apply(
				report.Snippet(imp),
				report.Snippetf(prev, "first imported here"),
			)
			if prev.ImportPath().AsLiteral().Text() != imp.ImportPath().AsLiteral().Text() {
				d.Apply(report.Helpf("both paths are equivalent to %q", path))
			}

			continue
		}

		file.imports.AddDirect(Import{
			File:   imported,
			Public: imp.IsPublic(),
			Weak:   imp.IsWeak(),
			Decl:   imp,
		})
	}

	// Having found all of the imports that are not cyclic, we now need to pull
	// in all of *their* transitive imports.
	file.imports.Recurse(dedup)

	// Check if descriptor.proto was transitively imported. If not, import it.
	if idx, ok := file.imports.byPath[file.session.builtins.DescriptorFile]; ok {
		// Copy it to the end so that it's easy to find.
		file.imports.files = append(file.imports.files, file.imports.files[idx])
		return
	}

	// If this is descriptor.proto itself, use it. This step is necessary to
	// avoid cycles.
	if file.IsDescriptorProto() {
		file.imports.Insert(Import{File: file}, -1, false)
		file.imports.byPath[file.session.builtins.DescriptorFile] = uint32(len(file.imports.files) - 1)
		file.imports.causes[file.session.builtins.DescriptorFile] = uint32(len(file.imports.files) - 1)
		return
	}

	// Otherwise, try to look it up.
	dproto, err := importer(-1, DescriptorProtoPath, ast.DeclImport{})

	if err != nil {
		panic(fmt.Errorf("could not import %q: %w", DescriptorProtoPath, err))
	}

	if dproto == nil {
		panic(fmt.Errorf("importing %q produced an invalid file", DescriptorProtoPath))
	}

	file.imports.Insert(Import{File: dproto, Decl: ast.DeclImport{}}, -1, false)
	file.imports.byPath[file.session.builtins.DescriptorFile] = uint32(len(file.imports.files) - 1)
	file.imports.causes[file.session.builtins.DescriptorFile] = uint32(len(file.imports.files) - 1)
}

// diagnoseCycle generates a diagnostic for an import cycle, showing each
// import contributing to the cycle in turn.
func diagnoseCycle(r *report.Report, cycle *ErrCycle) {
	path := cycle.Cycle[0].ImportPath().AsLiteral().AsString().Text()
	err := r.Errorf("detected cyclic import while importing %q", path)

	for i, imp := range cycle.Cycle {
		var message string
		if path := imp.ImportPath().AsLiteral().AsString(); !path.IsZero() {
			switch i {
			case 0:
				message = "imported here"
			case len(cycle.Cycle) - 1:
				message = fmt.Sprintf("...which imports %q, completing the cycle", path.Text())
			default:
				message = fmt.Sprintf("...which imports %q...", path.Text())
			}
		}
		err.Apply(
			report.PageBreak,
			report.Snippetf(imp, "%v", message),
		)
	}
}

// canonicalizeImportPath canonicalizes the path of an import declaration.
//
// This will generate diagnostics for invalid paths. Returns "" for paths that
// cannot be made canonical.
//
// If r is nil, no diagnostics are emitted. This behavior exists to avoid
// duplicating code with [CanonicalizeFilePath].
func canonicalizeImportPath(path string, r *report.Report, decl ast.DeclImport) string {
	if path == "" {
		if r != nil {
			r.Errorf("import path cannot be empty").Apply(
				report.Snippet(decl.ImportPath()),
			)
		}
		return ""
	}

	orig := path
	// Not filepath.ToSlash, since this conversion is file-system independent.
	path = strings.ReplaceAll(path, `\`, `/`)
	hasBackslash := orig != path
	if r != nil && hasBackslash {
		r.Errorf("import path cannot use `\\` as a path separator").Apply(
			report.Snippetf(decl.ImportPath(), "this path begins with a `%c`", path[0]),
			report.SuggestEdits(decl.ImportPath(), "use `/` as the separator instead", report.Edit{
				Start: 0, End: decl.ImportPath().Span().Len(),
				Replace: strconv.Quote(path),
			}),
			report.Notef("this restriction also applies when compiling on a non-Windows system"),
		)
	}

	path = filepath.ToSlash(filepath.Clean(path))
	isClean := !hasBackslash && orig == path
	if r != nil && !isClean {
		r.Errorf("import path must not contain `.`, `..`, or repeated separators").Apply(
			report.Snippetf(decl.ImportPath(), "imported here"),
			report.SuggestEdits(decl.ImportPath(), "canonicalize this path", report.Edit{
				Start: 0, End: decl.ImportPath().Span().Len(),
				Replace: strconv.Quote(path),
			}),
		)
	}

	if r != nil && isClean && strings.HasPrefix(path, "../") {
		r.Errorf("import path must not refer to parent directory").Apply(
			report.Snippetf(decl.ImportPath(), "imported here"),
		)

		return "" // Refuse to escape to a parent directory.
	}

	if r != nil {
		isLetter := func(b byte) bool {
			return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
		}
		if len(path) >= 2 && isLetter(path[0]) && path[1] == ':' {
			// TODO: error on windows?
			r.Warnf("import path appears to begin with the Windows drive prefix `%s`", path[:2]).Apply(
				report.Snippet(decl.ImportPath()),
				report.Notef("this is not an error, because `protoc` accepts it, but may result in unexpected behavior on Windows"),
			)
		}
	}

	if r != nil && strings.HasPrefix(path, "/") {
		r.Errorf("import path must be relative").Apply(
			report.Snippetf(decl.ImportPath(), "this path begins with a `%c`", path[0]),
		)
		return ""
	}

	return path
}

// CanonicalizeFilePath puts a file path into canonical form.
//
// This function is exported so that all code depending on this module can make
// sure paths are consistently canonicalized.
func CanonicalizeFilePath(path string) string {
	return canonicalizeImportPath(path, nil, ast.DeclImport{})
}
