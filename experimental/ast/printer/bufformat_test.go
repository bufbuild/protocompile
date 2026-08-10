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

// TestBufFormat is the legacy-formatter conformance suite for the
// printer's [printer.Legacy] preset.
//
// It walks the vendored bufformat testdata under testdata/bufformat,
// parsing each .proto file and comparing the formatted output against
// the corresponding .golden file. The corpus was originally seeded
// from buf's upstream bufformat testdata (de176125bc1a22a3d9a3a17b9b84dc502c7dd6c9)
// and is now maintained directly in this repo; updates should land here,
// not be re-imported from upstream.
func TestBufFormat(t *testing.T) {
	t.Parallel()

	bufTestdata := filepath.Join("testdata", "bufformat")

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
		relPath = filepath.ToSlash(relPath)

		t.Run(relPath, func(t *testing.T) {
			t.Parallel()

			// deprecate/* requires adding `deprecated` options to
			// existing declarations as an AST transform. The legacy
			// formatter does this in buf's FormatModuleSet wrapper,
			// not in the formatter itself; the companion ast/edit
			// package doesn't yet support modifying compact-options
			// brackets on fields or enum values either. The printer
			// only formats what the AST already contains.
			if strings.Contains(relPath, "deprecate/") {
				t.Skip("deprecate tests require buf-specific AST transforms")
			}

			// Detached comments at section boundaries stay at the
			// boundary during declaration sorting rather than
			// permuting with the surrounding declarations.
			//
			// When file declarations are reordered (imports sorted,
			// options grouped, etc.), comments that sat between two
			// declarations stay at the section boundary in our
			// output, while the legacy formatter attaches them to
			// the next declaration so they travel along with the
			// sort. Our behavior prevents comments from being
			// silently re-associated with the wrong declaration.
			//
			// Example from all/v1/all.proto:
			//
			//	package all.v1;
			//	// between-package-and-import comment  <-- our output
			//	                                           keeps the
			//	                                           comment here
			//	import ".../a.proto";
			//	// (legacy moves it onto a.proto so it sorts with that
			//	//  import; we leave it at the section boundary)
			//	import ".../b.proto";
			//
			// TestFormat/ordering_section_comments.proto exercises
			// the same divergence on a fixture we own.
			if strings.Contains(relPath, "all/v1/all") || strings.Contains(relPath, "customoptions/") {
				t.Skip("detached comment placement differs from the legacy formatter during sort")
			}

			// We always insert a space before trailing block
			// comments (`M /* comment */`), whereas the legacy
			// formatter elides the space (`M/* comment */`).
			// Consistent spacing before trailing comments is more
			// readable and matches the convention used everywhere
			// else in our output.
			//
			// Example from service/v1/service.proto:
			//
			//	// legacy golden:
			//	rpc Ping(/* Before */Message/* After */) returns ...
			//	// our output:
			//	rpc Ping(/* Before */Message /* After */) returns ...
			if strings.Contains(relPath, "service/v1/service") {
				t.Skip("trailing block comment spacing policy differs from the legacy formatter")
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

			opts := printer.Options{Format: true, Formatting: printer.Legacy()}
			got, err := printer.PrintFile(opts, file)
			if err != nil {
				t.Fatalf("PrintFile: %v", err)
			}
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

			// Verify the formatted output is valid protobuf and that
			// formatting is idempotent.
			errs2 := &report.Report{}
			file2, _ := parser.Parse(relPath, source.NewFile(relPath, got), errs2)
			for _, diagnostic := range errs2.Diagnostics {
				if diagnostic.Level() <= report.Error {
					t.Errorf("formatted output does not re-parse: %v", diagnostic)
				}
			}
			got2, err := printer.PrintFile(opts, file2)
			if err != nil {
				t.Fatalf("PrintFile (idempotency): %v", err)
			}
			if got2 != got {
				diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
					A:        difflib.SplitLines(got),
					B:        difflib.SplitLines(got2),
					FromFile: "format(source)",
					ToFile:   "format(format(source))",
					Context:  3,
				})
				t.Errorf("formatting is not idempotent:\n%s", diff)
			}
		})
	}
}
