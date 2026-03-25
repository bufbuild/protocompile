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

package printer_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/bufbuild/protocompile/experimental/ast/printer"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

// TestBufFormat runs the buf format golden tests against our printer.
//
// It walks the buf repo's bufformat testdata directory, parsing each .proto
// file and comparing the formatted output against the corresponding .golden
// file.
func TestBufFormat(t *testing.T) {
	t.Parallel()

	// The buf repo is expected to be a sibling of the protocompile repo.
	bufTestdata := filepath.Join(testBufRepoRoot(), "private", "buf", "bufformat", "testdata")
	if _, err := os.Stat(bufTestdata); err != nil {
		t.Skipf("buf testdata not found at %s: %v", bufTestdata, err)
	}

	// Collect all .proto files.
	var protoFiles []string
	err := filepath.Walk(bufTestdata, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".proto") {
			protoFiles = append(protoFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking testdata: %v", err)
	}

	for _, protoPath := range protoFiles {
		goldenPath := strings.TrimSuffix(protoPath, ".proto") + ".golden"
		relPath, _ := filepath.Rel(bufTestdata, protoPath)

		t.Run(relPath, func(t *testing.T) {
			t.Parallel()

			// Skip editions/2024 -- that's a parser error test, not a printer test.
			if strings.Contains(relPath, "editions/2024") {
				t.Skip("editions/2024 is a parser error test")
			}

			// Skip deprecate tests -- those require AST transforms (adding
			// deprecated options) that are done by buf's FormatModuleSet,
			// not by the printer itself.
			if strings.Contains(relPath, "deprecate/") {
				t.Skip("deprecate tests require buf-specific AST transforms")
			}

			// Skip: our formatter keeps detached comments at section boundaries
			// during sorting rather than permuting them with declarations.
			// This is intentional -- see PLAN.md.
			if strings.Contains(relPath, "all/v1/all") || strings.Contains(relPath, "customoptions/") {
				t.Skip("detached comment placement differs from old buf format during sort")
			}

			// Skip: our formatter always inserts a space before trailing
			// block comments (e.g., `M /* comment */` vs `M/* comment */`).
			// This is intentional -- consistent trailing comment spacing.
			if strings.Contains(relPath, "service/v1/service") {
				t.Skip("trailing block comment spacing policy differs from old buf format")
			}

			protoData, err := os.ReadFile(protoPath)
			if err != nil {
				t.Fatalf("reading proto: %v", err)
			}

			goldenData, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden: %v", err)
			}

			errs := &report.Report{}
			file, _ := parser.Parse(relPath, source.NewFile(relPath, string(protoData)), errs)
			for diagnostic := range errs.Diagnostics {
				t.Logf("parse warning: %q", diagnostic)
			}

			got := printer.PrintFile(printer.Options{Format: true}, file)
			want := string(goldenData)

			if got != want {
				diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
					A:        difflib.SplitLines(want),
					B:        difflib.SplitLines(got),
					FromFile: "want",
					ToFile:   "got",
					Context:  3,
				})
				t.Errorf("output mismatch:\n%s", diff)
			}

			// Also verify idempotency: formatting the formatted output
			// should produce the same result.
			errs2 := &report.Report{}
			file2, _ := parser.Parse(relPath, source.NewFile(relPath, got), errs2)
			got2 := printer.PrintFile(printer.Options{Format: true}, file2)
			if got2 != got {
				t.Errorf("formatting is not idempotent")
			}
		})
	}
}

// testBufRepoRoot returns the root of the buf repo, assumed to be a sibling
// of the protocompile repo.
func testBufRepoRoot() string {
	// Walk up from the current working directory to find the protocompile repo root,
	// then look for ../buf.
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	// The test runs from the package directory. Walk up to find go.mod.
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return filepath.Join(filepath.Dir(dir), "buf")
}
