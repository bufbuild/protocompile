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
	"github.com/bufbuild/protocompile/internal/intern"
)

// Import is an import in a [File].
type Import struct {
	File              // The file that is imported.
	Public, Weak bool // The kind of import this is.
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
	// All transitively-imported files. This slice is divided into the
	// following segments:
	//
	// 1. Public imports.
	// 2. Weak imports.
	// 3. Regular imports.
	// 4. Transitive public imports.
	// 4. Transitive imports.
	//
	// The fields after this one specify where each of these segments ends.
	files []File

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
	publicEnd, weakEnd, importEnd, transPublicEnd int
}

// Append appends a direct import to this imports table.
func (i *imports) AddDirect(imp Import) {
	switch {
	case imp.Public:
		i.files = slices.Insert(i.files, i.publicEnd, imp.File)
		i.publicEnd++
		i.weakEnd++
	case imp.Weak:
		i.files = slices.Insert(i.files, i.weakEnd, imp.File)
		i.weakEnd++
	default:
		i.files = slices.Insert(i.files, i.importEnd, imp.File)
	}

	i.importEnd++
	i.transPublicEnd++
}

// Recurse updates the import table to incorporate the transitive imports of
// each import.
func (i *imports) Recurse(dedup intern.Map[ast.DeclImport]) {
	for file := range seq.Values(i.Directs()) {
		for imp := range seq.Values(file.TransitiveImports()) {
			if !mapsx.AddZero(dedup, imp.InternedPath()) {
				continue
			}

			// Transitive imports are public to us if and only if they are
			// imported through a public import.
			if file.Public && imp.Public {
				i.files = slices.Insert(i.files, i.transPublicEnd, imp.File)
				i.transPublicEnd++
				continue
			}

			i.files = append(i.files, imp.File)
		}
	}
}

// Directs returns an indexer over the Directs imports.
func (i *imports) Directs() seq.Indexer[Import] {
	return seq.NewFixedSlice(
		i.files[:i.importEnd],
		func(n int, f File) Import {
			return Import{
				File:   f,
				Public: n < i.publicEnd,
				Weak:   n >= i.publicEnd && n < i.weakEnd,
			}
		},
	)
}

// Transitive returns an indexer over the Transitive imports.
//
// This function does not report whether those imports are weak or not.
func (i *imports) Transitive() seq.Indexer[Import] {
	return seq.NewFixedSlice(
		i.files,
		func(n int, f File) Import {
			return Import{
				File: f,
				Public: n < i.publicEnd ||
					(n >= i.importEnd && n < i.transPublicEnd),
			}
		},
	)
}
