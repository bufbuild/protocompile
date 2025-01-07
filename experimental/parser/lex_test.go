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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/golden"
)

func TestLexer(t *testing.T) {
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
		text = unescapeTestCase(text)

		errs := &report.Report{Tracing: 10}
		ctx := ast.NewContext(report.NewFile(path, text))
		lex(ctx, errs)

		stderr, _, _ := report.Renderer{
			Colorize:  true,
			ShowDebug: true,
		}.RenderString(errs)
		t.Log(stderr)
		outputs[1], _, _ = report.Renderer{}.RenderString(errs)

		var tsv strings.Builder
		var count int
		tsv.WriteString("#\t\tkind\t\toffsets\t\tlinecol\t\ttext\n")
		ctx.Stream().All()(func(tok token.Token) bool {
			count++

			sp := tok.Span()
			start := ctx.Stream().Location(sp.Start)
			fmt.Fprintf(
				&tsv, "%v\t\t%v\t\t%03d:%03d\t\t%03d:%03d\t\t%q",
				int32(tok.ID())-1, tok.Kind(),
				sp.Start, sp.End,
				start.Line, start.Column,
				tok.Text(),
			)

			if v := tok.AsBigInt(); v != nil {
				fmt.Fprintf(&tsv, "\t\tint:%d", v)
			} else if v, ok := tok.AsFloat(); ok {
				fmt.Fprintf(&tsv, "\t\tfloat:%g", v)
			} else if v, ok := tok.AsString(); ok {
				fmt.Fprintf(&tsv, "\t\tstring:%q", v)
			}

			if a, b := tok.StartEnd(); a != b {
				if tok == a {
					fmt.Fprintf(&tsv, "\t\tclose:%v", b.ID())
				} else {
					fmt.Fprintf(&tsv, "\t\topen:%v", a.ID())
				}
			}

			tsv.WriteByte('\n')
			return true
		})
		if count > 0 {
			outputs[0] = tsv.String()
		}
	})
}

// Our lexer tests support Unicode escapes in the form $u{nnnn} and byte escapes
// in the form $x{nn}. This is so that the checked-in files are human-readable
// while potentially containing unprintable characters or invalid bytes.
var escapePat = regexp.MustCompile(`\$([ux])\{(\w+)\}`)

func unescapeTestCase(s string) string {
	return escapePat.ReplaceAllStringFunc(s, func(needle string) string {
		groups := escapePat.FindStringSubmatch(needle)
		value, err := strconv.ParseInt(groups[2], 16, 32)
		if err != nil {
			panic(err)
		}

		if groups[1] == "x" {
			return string([]byte{byte(value)})
		}
		return string(rune(value))
	})
}
