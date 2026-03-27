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

package benchmark

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/fdp"
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/incremental/queries"
	"github.com/bufbuild/protocompile/experimental/ir"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/testing/googleapis"
	"github.com/bufbuild/protocompile/internal/testing/memory"
)

func BenchmarkCompileGoogleapis(b *testing.B) {
	workspace, sources := googleapis.Get()
	if workspace == nil {
		b.Skip()
	}
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
					_, _, _ = incremental.Run(b.Context(), exec, queries.Link{
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
	workspace, sources := googleapis.Get()
	if workspace == nil {
		t.Skip()
	}
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
	require.NoError(t, err)

	runtime.GC()
	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)
	t.Logf("heap usage: %v", bitsx.ByteSize(m.Alloc))

	tape := new(memory.MeasuringTape)
	tape.Measure(results)
	t.Logf("reachable memory: %v", bitsx.ByteSize(tape.Usage()))
}
