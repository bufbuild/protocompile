package benchmark

import (
	"runtime"
	"testing"

	"github.com/bufbuild/protocompile/experimental/fdp"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/testing/memory"
	"github.com/stretchr/testify/assert"
)

func BenchmarkCompileGoogleapis(b *testing.B) {
	workspace, sources := GoogleapisProtos()
	sources = &source.Openers{sources, source.WKTs()}

	b.ResetTimer()
	for b.Loop() {
		exec := incremental.New()
		sess := new(ir.Session)
		result, _, _ := incremental.Run(b.Context(), exec, queries.Link{
			Opener:    sources,
			Session:   sess,
			Workspace: workspace,
		})

		_, _ = fdp.DescriptorSetBytes(result[0].Value, fdp.IncludeSourceCodeInfo(true))
	}
}

func BenchmarkCompileDescriptor(b *testing.B) {
	sources := &source.Openers{source.WKTs()}
	workspace := source.NewWorkspace("google/protobuf/descriptor.proto")

	b.ResetTimer()
	for b.Loop() {
		exec := incremental.New()
		sess := new(ir.Session)
		result, _, _ := incremental.Run(b.Context(), exec, queries.Link{
			Opener:    sources,
			Session:   sess,
			Workspace: workspace,
		})

		_, _ = fdp.DescriptorSetBytes(result[0].Value, fdp.IncludeSourceCodeInfo(true))
	}
}

func TestCompileGoogleapisMemory(t *testing.T) {
	workspace, sources := GoogleapisProtos()
	sources = &source.Openers{sources, source.WKTs()}

	exec := incremental.New()
	sess := new(ir.Session)
	results, _, err := incremental.Run(t.Context(), exec, queries.Link{
		Opener:    sources,
		Session:   sess,
		Workspace: workspace,
	})
	assert.NoError(t, err)

	runtime.GC()
	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)
	t.Logf("heap usage: %v", bitsx.ByteSize(m.Alloc))

	tape := new(memory.MeasuringTape)
	tape.Measure(results)
	t.Logf("reachable memory: %v", bitsx.ByteSize(tape.Usage()))
}
