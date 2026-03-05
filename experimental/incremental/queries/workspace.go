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
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Workspace is an [incremental.Query] for the lowered IR files [ir.File] of the given
// Protobuf source workspace [source.Workspace].
//
// This allows us to check for duplicate symbols and extensions across the [source.Workspace],
// even if the [source.File]s may not import each other.
//
// Workspace queries with different Openers are considered distinct.
type Workspace struct {
	source.Opener // Must be comparable.
	*ir.Session
	source.Workspace // Must be comparable.
}

var _ incremental.Query[[]*ir.File] = Workspace{}

// Key implements [incremental.Query].
func (w Workspace) Key() any {
	return w
}

// Execute implements [incremental.Query].
func (w Workspace) Execute(t *incremental.Task) ([]*ir.File, error) {
	t.Report().Options.Stage += stageWorkspace

	queries := make([]incremental.Query[*ir.File], w.Workspace.Len())
	for i, path := range iterx.Enumerate(w.Workspace.Paths()) {
		queries[i] = IR{
			Opener:  w.Opener,
			Session: w.Session,
			Path:    path,
		}
	}

	results, err := incremental.Resolve(t, queries...)
	if err != nil {
		return nil, err
	}

	files := make([]*ir.File, len(queries))
	for i, result := range results {
		if result.Fatal != nil {
			return nil, result.Fatal
		}
		files[i] = result.Value
	}

	ir.DedupExportedSymbols(t.Report(), files...)
	ir.DedupExtensions(t.Report(), files...)
	return files, nil
}
