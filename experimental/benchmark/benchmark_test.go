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
	benchmark(b, sources, workspace)
}

func BenchmarkCompileDescriptor(b *testing.B) {
	sources := &source.Openers{source.WKTs()}
	workspace := source.NewWorkspace("google/protobuf/descriptor.proto")
	benchmark(b, sources, workspace)
}

func benchmark(b *testing.B, sources source.Opener, workspace source.Workspace) {
	for _, what := range []string{"hot", "cold"} {
		hot := what == "hot"
		b.Run(what, func(b *testing.B) {
			b.Run("link", func(b *testing.B) {
				exec := incremental.New()
				sess := new(ir.Session)
				for b.Loop() {
					if !hot {
						exec = incremental.New()
						sess = new(ir.Session)
					}
					incremental.Run(b.Context(), exec, queries.Link{
						Opener:    sources,
						Session:   sess,
						Workspace: workspace,
					})
				}
			})

			b.Run("desc", func(b *testing.B) {
				exec := incremental.New()
				sess := new(ir.Session)
				for b.Loop() {
					if !hot {
						exec = incremental.New()
						sess = new(ir.Session)
					}
					result, _, _ := incremental.Run(b.Context(), exec, queries.Link{
						Opener:    sources,
						Session:   sess,
						Workspace: workspace,
					})
					_, _ = fdp.DescriptorSetBytes(result[0].Value)
				}
			})

			b.Run("sci", func(b *testing.B) {
				exec := incremental.New()
				sess := new(ir.Session)
				for b.Loop() {
					if !hot {
						exec = incremental.New()
						sess = new(ir.Session)
					}
					result, _, _ := incremental.Run(b.Context(), exec, queries.Link{
						Opener:    sources,
						Session:   sess,
						Workspace: workspace,
					})
					_, _ = fdp.DescriptorSetBytes(result[0].Value, fdp.IncludeSourceCodeInfo(true))
				}
			})
		})
	}
}

func TestCompileGoogleapisMemory(t *testing.T) {
	workspace, sources := GoogleapisProtos()
	sources = &source.Openers{sources, source.WKTs()}
	testMemory(t, sources, workspace)
}

func TestCompileDescriptorMemory(t *testing.T) {
	sources := &source.Openers{source.WKTs()}
	workspace := source.NewWorkspace("google/protobuf/descriptor.proto")
	testMemory(t, sources, workspace)
}

func testMemory(t *testing.T, sources source.Opener, workspace source.Workspace) {
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
