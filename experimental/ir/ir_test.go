package ir_test

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/bufbuild/protocompile/internal/prototest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"gopkg.in/yaml.v3"
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
		outputs[1], _, _ = report.Renderer{}.RenderString(r)
		assert.NotContains(t, outputs[1], "internal compiler error")

		irs := slicesx.Transform(results, func(r incremental.Result[ir.File]) ir.File { return r.Value })
		irs = slices.DeleteFunc(irs, ir.File.IsZero)
		bytes, err := ir.DescriptorSetBytes(irs)
		require.NoError(t, err)

		fds := new(descriptorpb.FileDescriptorSet)
		require.NoError(t, proto.Unmarshal(bytes, fds))

		outputs[0] = prototest.ToYAML(fds, prototest.ToYAMLOptions{})
	})
}
