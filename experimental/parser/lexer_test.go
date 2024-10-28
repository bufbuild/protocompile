// Copyright 2020-2024 Buf Technologies, Inc.
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

package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/golden"
)

func TestRender(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:      "testdata/lexer",
		Refresh:   "PROTOCOMPILE_REFRESH",
		Extension: "proto",
		Outputs: []golden.Output{
			{Extension: "tokens.tsv"},
			{Extension: "stderr.txt"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		errs := &report.Report{Tracing: 10}
		ctx := ast.NewContext(report.File{Path: path, Text: text})
		parser.Lex(ctx, errs)

		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(errs)
		t.Log(stderr)
		outputs[1], _, _ = report.Renderer{}.RenderString(errs)

		var tsv strings.Builder
		tsv.WriteString("#\t\tkind\t\toffsets\t\tlinecol\t\ttext\n")
		ctx.Stream().All()(func(tok token.Token) bool {
			sp := tok.Span()
			start := ctx.Stream().IndexedFile.Search(sp.Start)
			fmt.Fprintf(
				&tsv, "%v\t\t%v\t\t%03d:%03d\t\t%03d:%03d\t\t%q",
				int32(tok.ID())-1, tok.Kind(),
				sp.Start, sp.End,
				start.Line, start.Column,
				tok.Text(),
			)

			if v, ok := tok.AsInt(); ok {
				fmt.Fprintf(&tsv, "\tint:%d", v)
			} else if v, ok := tok.AsFloat(); ok {
				fmt.Fprintf(&tsv, "\tfloat:%g", v)
			} else if v, ok := tok.AsString(); ok {
				fmt.Fprintf(&tsv, "\tstring:%q", v)
			}

			tsv.WriteByte('\n')
			return true
		})
		outputs[0] = tsv.String()
	})
}
