// Copyright 2020-2026 Buf Technologies, Inc.
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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/ast"
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

func TestExprNestingDepth(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		expr  func(depth int) string
		start string
	}{
		{
			name:  "dict",
			expr:  func(depth int) string { return strings.Repeat("{a: ", depth-1) + "{}" + strings.Repeat("}", depth-1) },
			start: "{",
		},
		{
			name:  "angles",
			expr:  func(depth int) string { return strings.Repeat("<a: ", depth-1) + "<>" + strings.Repeat(">", depth-1) },
			start: "<",
		},
		{
			name:  "array",
			expr:  func(depth int) string { return strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth) },
			start: "[",
		},
		{
			name:  "prefix",
			expr:  func(depth int) string { return strings.Repeat("-", depth) + "1" },
			start: "-",
		},
		{
			name:  "range",
			expr:  func(depth int) string { return "1" + strings.Repeat(" to 1", depth) },
			start: "to",
		},
	} {
		for _, depth := range []int{100, 101, 200000} {
			t.Run(fmt.Sprintf("%s/%d", test.name, depth), func(t *testing.T) {
				t.Parallel()
				// The second option checks that the depth is reset.
				text := "syntax = \"proto3\"; option x = " + test.expr(depth) + "; option y = " + test.expr(100) + ";"
				errs := &report.Report{}
				Parse("test.proto", source.NewFile("test.proto", text), errs)

				var depthErrors int
				for _, diagnostic := range errs.Diagnostics {
					if diagnostic.Message() != "expression nesting depth must be at most 100" {
						continue
					}
					depthErrors++
					assert.Equal(t, test.start, diagnostic.Primary().Text())
				}
				if depth > 100 {
					assert.Equal(t, 1, depthErrors)
				} else {
					assert.Zero(t, depthErrors)
				}
			})
		}
	}
}

func TestBodyNestingDepth(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		decl func(depth int) string
	}{
		{
			name: "message",
			decl: func(depth int) string { return strings.Repeat("message M {", depth) + strings.Repeat("}", depth) },
		},
		{
			name: "body",
			decl: func(depth int) string { return strings.Repeat("{", depth) + strings.Repeat("}", depth) },
		},
	} {
		for _, depth := range []int{100, 101, 200000} {
			t.Run(fmt.Sprintf("%s/%d", test.name, depth), func(t *testing.T) {
				t.Parallel()
				text := "syntax = \"proto3\"; " + test.decl(depth)
				errs := &report.Report{}
				file, _ := Parse("test.proto", source.NewFile("test.proto", text), errs)

				// The first body past the limit is kept, but left empty.
				var got int
				decls := file.Decls()
				for decls.Len() > 0 {
					var body ast.DeclBody
					switch decl := decls.At(decls.Len() - 1); decl.Kind() {
					case ast.DeclKindBody:
						body = decl.AsBody()
					case ast.DeclKindDef:
						body = decl.AsDef().Body()
					}
					if body.IsZero() {
						break
					}
					got++
					decls = body.Decls()
				}
				assert.Equal(t, min(depth, 101), got)
			})
		}
	}
}

func TestTypeNestingDepth(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		ty    func(depth int) string
		start string
	}{
		{
			name:  "map",
			ty:    func(depth int) string { return strings.Repeat("map<", depth) + "int32" + strings.Repeat(">", depth) },
			start: "<",
		},
		{
			name:  "modifier",
			ty:    func(depth int) string { return strings.Repeat("optional ", depth) + "int32" },
			start: "optional",
		},
	} {
		for _, depth := range []int{100, 101, 200000} {
			t.Run(fmt.Sprintf("%s/%d", test.name, depth), func(t *testing.T) {
				t.Parallel()
				// The second field checks that the depth is reset.
				text := "syntax = \"proto3\"; message M { " + test.ty(depth) + " x = 1; " + test.ty(100) + " y = 2; }"
				errs := &report.Report{}
				Parse("test.proto", source.NewFile("test.proto", text), errs)

				var depthErrors int
				for _, diagnostic := range errs.Diagnostics {
					if diagnostic.Message() != "type nesting depth must be at most 100" {
						continue
					}
					depthErrors++
					assert.Equal(t, test.start, diagnostic.Primary().Text())
				}
				if depth > 100 {
					assert.Equal(t, 1, depthErrors)
				} else {
					assert.Zero(t, depthErrors)
				}
			})
		}
	}
}
