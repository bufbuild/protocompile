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

package ast_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/tidwall/pretty"
	"google.golang.org/protobuf/encoding/protojson"
)

const jsonSpanSub = `{ "start": $1, "end": $2 }`

var jsonSpanPat = regexp.MustCompile(`\{\s*"start":\s*(\d+),\s*"end":\s*(\d+)\s*\}`)

func TestParser(t *testing.T) {
	renderer := report.Renderer{
		ShowRemarks: true,
	}

	corpus := golden.Corpus{
		Root:      "testdata/parser",
		Refresh:   "PROTOCOMPILE_REFRESH",
		Extension: "proto",
		Outputs: []golden.Output{
			{Extension: "lex.tsv"},
			{Extension: "ast.json"},
			{Extension: "stderr"},
		},
	}
	corpus.Run(t, func(t *testing.T, path, text string, results []string) {
		var r report.Report
		defer func() {
			// Dump out the report in a defer so it shows up even if we panic.
			text, _ := renderer.Render(&r)
			results[2] = text
		}()

		file := ast.Parse(report.File{Path: path, Text: text}, &r)
		proto := ast.FileToProto(file)

		var tokens strings.Builder
		file.Context().Tokens().Iter(func(i int, tok ast.Token) bool {
			start, end := tok.Span().Offsets()
			loc := tok.Span().Start()
			fmt.Fprintf(&tokens, "%4d:%#04x\t%v\t%d:%d\t%d:%d:%d", i, i, tok.Kind(), start, end, loc.Line, loc.Column, loc.UTF16)
			if v, ok := tok.AsInt(); ok {
				fmt.Fprintf(&tokens, "\t%d", v)
			} else if v, ok := tok.AsFloat(); ok {
				fmt.Fprintf(&tokens, "\t%f", v)
			} else if v, ok := tok.AsString(); ok {
				fmt.Fprintf(&tokens, "\t%q", v)
			}
			fmt.Fprintf(&tokens, "\t%q\n", tok.Text())

			return true
		})
		results[0] = tokens.String()

		jsonOptions := protojson.MarshalOptions{
			Multiline: true,
			Indent:    "  ",
		}

		json, err := jsonOptions.Marshal(proto)
		if err != nil {
			results[1] = fmt.Sprint("marshal error:", err)
		} else {
			json = pretty.PrettyOptions(json, &pretty.Options{
				Indent: "  ",
			})
			results[1] = string(json)
		}

		// Compact all of the Span objects into single lines.
		results[1] = jsonSpanPat.ReplaceAllString(results[1], jsonSpanSub)
	})
}
