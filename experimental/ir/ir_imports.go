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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/intern"
)

// Import is an import in a [File].
type Import struct {
	*File                       // The file that is imported.
	Public, Weak bool           // The kind of import this is.
	Direct       bool           // Whether this is a direct or transitive import.
	Visible      bool           // Whether this import's symbols are visible in the current file.
	Used         bool           // Whether this import has been marked as used.
	Decl         ast.DeclImport // The import declaration.
}

// imports is a data structure for compactly classifying the transitive imports
// of a Protobuf file.
//
// When building the importable symbol table, we include the symbols from each
// direct import, as well as direct import's transitive public imports, NOT
// the *current* file's transitive public imports. The transitive public
// of a file are those which have a path from the importing file via public
// imports only.
//
// For example, where -> is a normal import and => a public import, a => b => c
// has that c's transitive public imports are {a, b}, but a => b -> d has d with
// no transitive public imports. However, both c and d's importable symbol
// tables will include all symbols from a and b, because b is a direct import,
// and a is a transitive public import of a direct import.
type imports struct {
	// All transitively-imported files and their AST definition. This slice is divided
	// into the following segments:
	//
	// 1. Public imports.
	// 2. Weak imports.
	// 3. Regular imports.
	// 4. Transitive public imports.
	// 5. Transitive imports.
	//
	// The fields after this one specify where each of these segments ends.
	//
	// The last element of this slice is always descriptor.proto, even if it
	// exists elsewhere as an ordinary import.
	files []imported

	// Maps the path of each file to its index in files. This is used for
	// mapping from one [Context]'s file IDs to another's.
	byPath intern.Map[uint32]

	// Map of path of each imported file to a direct import which causes it to
	// be imported. This is used for marking which imports are used.
	causes intern.Map[uint32]

	// NOTE: public imports always come first. This ensures that when
	// recursively determining public imports, we consider public imports'
	// recursive imports first. Consider the following sequence of files:
	//
	//  // a.proto
	//  message A {}
	//
	//  // b.proto
	//  import public "a.proto"
	//
	//  // c.proto
	//  import "d.proto"
	//
	//  // d.proto
	//  import public "b.proto"
	//  import "c.proto"
	//
	//  // e.proto
	//  import "d.proto"
	//
	//  message B { A foo = 1; }
	//
	// Because b imports a publicly, we need a to wind up as a transitive
	// public import so that when we search the transitive public imports of d
	// for symbols, we pick up "a.proto".
	//
	// There is a test in ir_imports_test.go that validates this behavior. So
	// much pain for a little-used feature...
	publicEnd, weakEnd, importEnd, transPublicEnd uint32
}

// imported wraps an imported [File] and the import statement declaration [ast.DeclImport].
type imported struct {
	file          *File
	decl          ast.DeclImport
	visible, used bool
}

// AddDirect appends a direct import to this imports table.
func (i *imports) AddDirect(imp Import) {
	switch {
	case imp.Public:
		i.Insert(imp, int(i.publicEnd), true)
		i.publicEnd++
		i.weakEnd++
	case imp.Weak:
		i.Insert(imp, int(i.weakEnd), true)
		i.weakEnd++
	default:
		i.Insert(imp, int(i.importEnd), true)
	}

	i.importEnd++
	i.transPublicEnd++
}

// Recurse updates the import table to incorporate the transitive imports of
// each import.
//
// Must only be called once, after all direct imports are added.
func (i *imports) Recurse(dedup intern.Map[ast.DeclImport]) {
	for _, file := range seq.All(i.Directs()) {
		for imp := range seq.Values(file.TransitiveImports()) {
			if !mapsx.AddZero(dedup, imp.InternedPath()) {
				continue
			}

			// Transitive imports are public to us if and only if they are
			// imported through a public import.
			if file.Public && imp.Public {
				i.Insert(imp, int(i.transPublicEnd), true)
				i.transPublicEnd++
				continue
			}

			// Public imports of direct imports are visible in the current file.
			i.Insert(imp, -1, imp.Public)
		}
	}

	// Now, build the path and causes maps.
	i.byPath = make(intern.Map[uint32])
	i.causes = make(intern.Map[uint32])

	for n, imp := range i.files {
		i.byPath[imp.file.InternedPath()] = uint32(n)
	}
	for k, file := range seq.All(i.Directs()) {
		// Direct imports take precedence over transitive imports.
		i.causes[file.InternedPath()] = uint32(k)
		for imp := range seq.Values(file.TransitiveImports()) {
			mapsx.Add(i.causes, imp.InternedPath(), uint32(k))
		}
	}
}

// Insert inserts a new import at the given position.
//
// If pos is < 0, appends at the end.
func (i *imports) Insert(imp Import, pos int, visible bool) {
	if pos < 0 {
		pos = len(i.files)
	}

	i.files = slices.Insert(i.files, pos, imported{
		file:    imp.File,
		decl:    imp.Decl,
		visible: visible,
	})
}

// MarkUsed records a file as used, which affects the values of [Import].Used.
func (i *imports) MarkUsed(file *File) {
	idx, ok := i.causes[file.InternedPath()]
	if ok {
		i.files[idx].used = true
	}
}

// DescriptorProto returns the file for descriptor.proto.
func (i *imports) DescriptorProto() *File {
	imported, _ := slicesx.Last(i.files)
	return imported.file
}

// Directs returns an indexer over the Directs imports.
func (i *imports) Directs() seq.Indexer[Import] {
	return seq.NewFixedSlice(
		i.files[:i.importEnd],
		func(j int, imported imported) Import {
			n := uint32(j)
			public := n < i.publicEnd
			return Import{
				File:    imported.file,
				Public:  public,
				Weak:    !public && n < i.weakEnd,
				Direct:  true,
				Visible: true,
				Decl:    imported.decl,

				// Public imports are implicitly always used.
				Used: imported.used || public,
			}
		},
	)
}

// Transitive returns an indexer over the Transitive imports.
//
// This function does not report whether those imports are weak or used.
func (i *imports) Transitive() seq.Indexer[Import] {
	return seq.NewFixedSlice(
		i.files[:max(0, len(i.files)-1)], // Exclude the implicit descriptor.proto.
		func(j int, imported imported) Import {
			n := uint32(j)
			return Import{
				File: imported.file,
				Public: n < i.publicEnd ||
					(n >= i.importEnd && n < i.transPublicEnd),
				Direct:  n < i.importEnd,
				Visible: imported.visible,
				Decl:    imported.decl,
			}
		},
	)
}
