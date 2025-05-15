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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"gopkg.in/yaml.v3"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/ir/presence"
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
//
//nolint:tagliatelle
type Test struct {
	Files             []File `yaml:"files"`
	ExcludeWKTSources bool   `yaml:"exclude_wkt_sources"`
	OutputWKTs        bool   `yaml:"output_wkts"`
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

		var files source.Opener = source.NewMap(maps.Collect(iterx.Map1To2(
			slices.Values(test.Files),
			func(f File) (string, string) {
				return f.Path, f.Text
			},
		)))

		if !test.ExcludeWKTSources {
			files = &source.Openers{files, source.WKTs()}
		}

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
		assert.NotContains(t, outputs[1], "unexpected panic; this is a bug")

		irs := slicesx.Transform(results, func(r incremental.Result[ir.File]) ir.File { return r.Value })
		irs = slices.DeleteFunc(irs, ir.File.IsZero)
		bytes, err := ir.DescriptorSetBytes(irs)
		require.NoError(t, err)

		fds := new(descriptorpb.FileDescriptorSet)
		require.NoError(t, proto.Unmarshal(bytes, fds))

		if !test.OutputWKTs {
			fds.File = slices.DeleteFunc(fds.File, func(fdp *descriptorpb.FileDescriptorProto) bool {
				return strings.HasPrefix(*fdp.Name, "google/protobuf/")
			})
		}

		outputs[0] = prototest.ToYAML(fds, prototest.ToYAMLOptions{})
		outputs[1] = prototest.ToYAML(symtabProto(irs, &test), prototest.ToYAMLOptions{})
	})
}

func symtabProto(files []ir.File, t *Test) *compilerpb.SymbolSet {
	set := new(compilerpb.SymbolSet)
	set.Tables = make(map[string]*compilerpb.SymbolTable)

	for _, file := range files {
		// Don't bother if the file only has a single symbol for its
		// package, and no options.
		if file.Options().IsZero() {
			switch file.Symbols().Len() {
			case 0:
				continue
			case 1:
				if file.Symbols().At(0).Kind() == ir.SymbolKindPackage {
					continue
				}
			}
		}

		symtab := &compilerpb.SymbolTable{
			Options: messageProto(file.Options()),
		}

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
			if !t.OutputWKTs && strings.HasPrefix(sym.File().Path(), "google/protobuf/") {
				continue
			}

			var options ir.MessageValue
			switch sym.Kind() {
			case ir.SymbolKindMessage, ir.SymbolKindEnum:
				options = sym.AsType().Options()
			case ir.SymbolKindField, ir.SymbolKindExtension, ir.SymbolKindEnumValue:
				options = sym.AsMember().Options()
			case ir.SymbolKindOneof:
				options = sym.AsOneof().Options()
			}

			symtab.Symbols = append(symtab.Symbols, &compilerpb.Symbol{
				Fqn:     string(sym.FullName()),
				Kind:    compilerpb.Symbol_Kind(sym.Kind()),
				File:    sym.File().Path(),
				Index:   uint32(sym.RawData()),
				Visible: sym.Kind() != ir.SymbolKindPackage && sym.Visible(),
				Options: messageProto(options),
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

func messageProto(v ir.MessageValue) *compilerpb.Value {
	if v.IsZero() {
		return nil
	}

	m := new(compilerpb.Value_Message)
	for elem := range seq.Values(v.Fields()) {
		if elem.Field().IsExtension() {
			if m.Extns == nil {
				m.Extns = make(map[string]*compilerpb.Value)
			}
			m.Extns[string(elem.Field().FullName())] = valueProto(elem)
		} else {
			if m.Fields == nil {
				m.Fields = make(map[string]*compilerpb.Value)
			}
			m.Fields[elem.Field().Name()] = valueProto(elem)
		}
	}

	return &compilerpb.Value{Value: &compilerpb.Value_Message_{Message: m}}
}

func valueProto(v ir.Value) *compilerpb.Value {
	if v.IsZero() {
		return nil
	}

	element := func(v ir.Element) *compilerpb.Value {
		if x, ok := v.AsBool(); ok {
			return &compilerpb.Value{Value: &compilerpb.Value_Bool{Bool: x}}
		}

		if x, ok := v.AsInt(); ok {
			return &compilerpb.Value{Value: &compilerpb.Value_Int{Int: x}}
		}

		if x, ok := v.AsUInt(); ok {
			return &compilerpb.Value{Value: &compilerpb.Value_Uint{Uint: x}}
		}

		if x, ok := v.AsFloat(); ok {
			return &compilerpb.Value{Value: &compilerpb.Value_Float{Float: x}}
		}

		if x, ok := v.AsString(); ok {
			return &compilerpb.Value{Value: &compilerpb.Value_String_{String_: []byte(x)}}
		}

		return messageProto(v.AsMessage())
	}

	if v.Field().Presence() == presence.Repeated {
		r := new(compilerpb.Value_Repeated)
		for elem := range seq.Values(v.Elements()) {
			r.Values = append(r.Values, element(elem))
		}
		return &compilerpb.Value{Value: &compilerpb.Value_Repeated_{Repeated: r}}
	}

	return element(v.Elements().At(0))
}
