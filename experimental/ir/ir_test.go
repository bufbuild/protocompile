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

package ir_test

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/cmpx"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	compilerpb "github.com/bufbuild/protocompile/internal/gen/buf/compiler/v1alpha1"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/bufbuild/protocompile/internal/prototest"
)

// Test is the type that a test case for the compiler is deserialized from.
type Test struct {
	Files []File `yaml:"files"`
}

type File struct {
	Path   string `yaml:"path"`
	Text   string `yaml:"text"`
	Import bool   `yaml:"import"`
}

func TestIR(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata",
		Refresh:    "PROTOCOMPILE_REFRESH",
		Extensions: []string{"proto", "proto.yaml"},
		Outputs: []golden.Output{
			{Extension: "fds.yaml"},
			{Extension: "symtab.yaml"},
			{Extension: "stderr.txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		var test Test
		switch filepath.Ext(path) {
		case ".proto":
			test.Files = []File{{Path: path, Text: text}}
		case ".yaml":
			require.NoError(t, yaml.Unmarshal([]byte(text), &test))
		}

		files := source.NewMap(maps.Collect(iterx.Map1To2(
			slices.Values(test.Files),
			func(f File) (string, string) {
				return f.Path, f.Text
			},
		)))

		exec := incremental.New(
			incremental.WithParallelism(1),
			incremental.WithReportOptions(report.Options{Tracing: 10}),
		)

		session := new(ir.Session)
		queries := slices.Collect(iterx.FilterMap(
			slices.Values(test.Files),
			func(f File) (incremental.Query[ir.File], bool) {
				if f.Import {
					return nil, false
				}
				return queries.IR{
					Opener:  files,
					Session: session,
					Path:    f.Path,
				}, true
			},
		))

		results, r, err := incremental.Run(context.Background(), exec, queries...)
		require.NoError(t, err)

		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(r)
		t.Log(stderr)
		outputs[2], _, _ = report.Renderer{}.RenderString(r)
		assert.NotContains(t, outputs[1], "internal compiler error")

		irs := slicesx.Transform(results, func(r incremental.Result[ir.File]) ir.File { return r.Value })
		irs = slices.DeleteFunc(irs, ir.File.IsZero)
		bytes, err := ir.DescriptorSetBytes(irs)
		require.NoError(t, err)

		fds := new(descriptorpb.FileDescriptorSet)
		require.NoError(t, proto.Unmarshal(bytes, fds))

		outputs[0] = prototest.ToYAML(fds, prototest.ToYAMLOptions{})
		outputs[1] = prototest.ToYAML(symtabProto(irs), prototest.ToYAMLOptions{})
	})
}

func symtabProto(files []ir.File) *compilerpb.SymbolSet {
	set := new(compilerpb.SymbolSet)
	set.Tables = make(map[string]*compilerpb.SymbolTable)

	for _, file := range files {
		if file.Symbols().Len() <= 1 {
			// Don't bother if the file only has a single symbol for its
			// package.
			continue
		}

		symtab := new(compilerpb.SymbolTable)

		for imp := range seq.Values(file.TransitiveImports()) {
			symtab.Imports = append(symtab.Imports, &compilerpb.Import{
				Path:       imp.Path(),
				Public:     imp.Public,
				Weak:       imp.Weak,
				Transitive: !imp.Direct,
				Visible:    imp.Visible,
			})
		}
		slices.SortFunc(symtab.Imports, cmpx.Key(func(x *compilerpb.Import) string { return x.Path }))

		for sym := range seq.Values(file.Symbols()) {
			symtab.Symbols = append(symtab.Symbols, &compilerpb.Symbol{
				Fqn:     string(sym.FullName()),
				Kind:    compilerpb.Symbol_Kind(sym.Kind()),
				File:    sym.File().Path(),
				Index:   uint32(sym.RawData()),
				Visible: sym.Kind() != ir.SymbolKindPackage && sym.Visible(),
			})
		}
		slices.SortFunc(symtab.Symbols,
			cmpx.Join(
				cmpx.Key(func(x *compilerpb.Symbol) string { return x.File }),
				cmpx.Key(func(x *compilerpb.Symbol) compilerpb.Symbol_Kind { return x.Kind }),
				cmpx.Key(func(x *compilerpb.Symbol) uint32 { return x.Index }),
			),
		)

		set.Tables[file.Path()] = symtab
	}

	return set
}
