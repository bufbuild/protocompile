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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/internal/protoscope/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
)

func TestParse(t *testing.T) {
	files, err := filepath.Glob("../testdata/*.protoscope")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no test files found in ../testdata")
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			src := source.NewFile(name, string(content))
			r := &report.Report{}
			parsed, ok := Parse(name, src, r)
			if !ok {
				t.Fatalf("Parse failed: %v", r.Diagnostics)
			}

			for decl := range seq.Values(parsed.Decls()) {
				span := decl.Span()
				t.Logf("decl span: %v", span)
			}

			if parsed.Decls().Len() == 0 {
				t.Errorf("expected at least one declaration, got 0")
				t.Logf("token stream: %v", parsed.Stream())
				c := parsed.Stream().Cursor()
				for !c.Done() {
					t.Logf("token: %v (%q)", c.Peek(), c.Peek().Text())
					_ = c.Next()
				}
			}
		})
	}
}

func TestSliceReallocation(t *testing.T) {
	var buf bytes.Buffer
	for i := 1; i <= 1000; i++ {
		buf.WriteString("1: 42\n")
	}

	src := source.NewFile("large.protoscope", buf.String())
	r := &report.Report{}
	file, ok := Parse("large.protoscope", src, r)
	if !ok {
		t.Fatalf("Parse failed: %v", r.Diagnostics)
	}

	count := 0
	for decl := range seq.Values(file.Decls()) {
		count++
		field := id.Wrap(file, id.ID[ast.Field](decl.ID().Value()))
		tag, _ := field.Tag().AsNumber().Int()
		if tag != 1 {
			t.Errorf("decl %d: expected tag 1, got %d", count, tag)
		}

		val := field.Value()
		lit := id.Wrap(file, id.ID[ast.Literal](val.ID().Value()))
		num, _ := lit.Token().AsNumber().Int()
		if num != 42 {
			t.Errorf("decl %d: expected value 42, got %d", count, num)
		}
	}
	if count != 1000 {
		t.Errorf("expected 1000 declarations, got %d", count)
	}
}

func TestParseBackticks(t *testing.T) {
	src := source.NewFile("test.protoscope", "1: {`01 02 03`}")
	r := &report.Report{}
	_, ok := Parse("test.protoscope", src, r)
	if !ok {
		t.Fatalf("Parse failed: %v", r.Diagnostics)
	}
}
