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

package parser

import (
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/internal/astx"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/bufbuild/protocompile/internal/prototest"
)

const (
	preserveSpans = `//pragma:preservespans`
)

func TestParse(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata/parser",
		Refresh:    "PROTOCOMPILE_REFRESH",
		Extensions: []string{"proto"},
		Outputs: []golden.Output{
			{Extension: "yaml"},
			{Extension: "stderr.txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		errs := &report.Report{Options: report.Options{Tracing: 10}}
		file, _ := Parse(path, source.NewFile(path, text), errs)

		errs.Canonicalize()
		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(errs)
		t.Log(stderr)
		outputs[1], _, _ = report.Renderer{}.RenderString(errs)

		// Make sure we catch panics that were converted to ICEs.
		if strings.Contains(stderr, "internal compiler error") {
			t.Fail()
		}

		proto := astx.ToProto(file, astx.ToProtoOptions{
			OmitSpans: !strings.Contains(text, preserveSpans),
			OmitFile:  true,
		})

		outputs[0] = prototest.ToYAML(proto, prototest.ToYAMLOptions{})
	})
}
