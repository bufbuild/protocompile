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
	"slices"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/mapsx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// Link is an [incremental.Query] for the lowered IR files [*ir.File] of the given
// Protobuf source workspace [source.Workspace]. This query links the compilation of the
// given sources together and allows us to additional checks across the sources,
// e.g. duplicate symbols and extensions across the given [source.Workspace].
//
// Link queries with different [source.Opener]s and/or [source.Workspace]s are
// considered distinct.
type Link struct {
	source.Opener // Must be comparable.
	*ir.Session
	source.Workspace // Must be comparable.
}

var _ incremental.Query[[]*ir.File] = Link{}

// Key implements [incremental.Query].
func (l Link) Key() any {
	return l
}

// Execute implements [incremental.Query].
func (l Link) Execute(t *incremental.Task) ([]*ir.File, error) {
	t.Report().Options.Stage += stageLink

	queries := slicesx.Transform(
		l.Workspace.Paths(),
		func(path string) incremental.Query[*ir.File] {
			return IR{
				Opener:  l.Opener,
				Session: l.Session,
				Path:    path,
			}
		},
	)

	results, err := incremental.Resolve(t, queries...)
	if err != nil {
		return nil, err
	}

	files, err := results.Slice()
	if err != nil {
		return nil, err
	}

	// Symbols are already deduplicated among imported files during the IR queries.
	ir.DedupExportedSymbols(t.Report(), files...)
	// Extension numbers are not deduped among imports during the IR queries, so all imported
	// files are added to this check. We avoid adding duplicated imported files.
	seen := make(map[string]*ir.File)
	var requiredImports []*ir.File
	for _, file := range files {
		// We will already include all linked files
		mapsx.Add(seen, file.Path(), file)
		if exists := seen[file.Path()]; exists == nil {
			seen[file.Path()] = file
		}
		for imp := range seq.Values(file.Imports()) {
			file, inserted := mapsx.Add(seen, imp.Path(), imp.File)
			if inserted {
				requiredImports = append(requiredImports, file)
			}
		}
	}
	// Make a copy of the files slice
	ir.DedupExtensions(t.Report(), slices.Concat(files, requiredImports)...)
	return files, nil
}
