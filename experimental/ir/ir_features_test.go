// Copyright 2020-2026 Buf Technologies, Inc.
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

package ir_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
)

func TestConcurrentFeatureLookup(t *testing.T) {
	t.Parallel()

	const (
		importers = 24
		enums     = 4
	)

	// base.proto declares the enums but never uses them, so nothing in its own
	// lowering looks up the enum_type feature. That leaves the memo cold for
	// the importers below to fill in concurrently.
	var base strings.Builder
	base.WriteString("edition = \"2023\";\npackage race;\n")
	for i := range enums {
		fmt.Fprintf(&base, "\nenum Shared%[1]d {\n  SHARED%[1]d_UNSPECIFIED = 0;\n  SHARED%[1]d_A = 1;\n}\n", i)
	}

	srcs := map[string]*source.File{
		"base.proto": source.NewFile("base.proto", base.String()),
	}
	paths := make([]string, importers)
	for i := range importers {
		var text strings.Builder
		fmt.Fprintf(&text, "edition = \"2023\";\npackage race;\n\nimport \"base.proto\";\n\nmessage M%d {\n", i)
		for j := range enums {
			fmt.Fprintf(&text, "  race.Shared%d f%d = %d;\n", j, j, j+1)
		}
		text.WriteString("}\n")

		paths[i] = fmt.Sprintf("user%d.proto", i)
		srcs[paths[i]] = source.NewFile(paths[i], text.String())
	}

	var files source.Opener = &source.Openers{source.NewMap(srcs), source.WKTs()}
	exec := incremental.New(incremental.WithParallelism(8))
	results, r, err := incremental.Run(t.Context(), exec, queries.Link{
		Opener:    files,
		Session:   new(ir.Session),
		Workspace: source.NewWorkspace(paths...),
	})
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Len(t, results, 1)
	require.NoError(t, results[0].Fatal)

	for _, d := range r.Diagnostics {
		t.Log(d.Message())
	}
	assert.Empty(t, r.Diagnostics)
	assert.Len(t, results[0].Value, importers)
}
