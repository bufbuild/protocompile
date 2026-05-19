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
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/experimental/ast/printer"
	"github.com/bufbuild/protocompile/experimental/parser"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/internal/golden"
)

// TestRoundTrip exercises round-tripping a protobuf source through [printer.PrintFile].
func TestRoundTrip(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata/roundtrip",
		Extensions: []string{"proto"},
	}

	corpus.Run(t, func(t *testing.T, path, text string, _ []string) {
		errs := &report.Report{}
		file, _ := parser.Parse(path, source.NewFile(path, text), errs)
		for _, d := range errs.Diagnostics {
			if d.Level() <= report.Warning {
				t.Logf("parse warning: %q", d)
			}
		}

		got, err := printer.PrintFile(printer.Options{}, file)
		if err != nil {
			t.Fatalf("PrintFile: %v", err)
		}
		if msg := golden.CompareAndDiff(got, text); msg != "" {
			t.Errorf("round-trip mismatch:\n%s", msg)
		}
	})
}

// TestPrint exercises [printer.Print] on each declaration in the round-trip
// corpus. The concatenated output of [printer.Print] on each AST decl is
// expected to be equivalent to the output of [printer.PrintFile], minus any
// file-level trailing trivia, since those will not be captured by AST decls.
func TestPrint(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata/roundtrip",
		Extensions: []string{"proto"},
	}

	corpus.Run(t, func(t *testing.T, path, text string, _ []string) {
		errs := &report.Report{}
		file, _ := parser.Parse(path, source.NewFile(path, text), errs)
		for _, d := range errs.Diagnostics {
			if d.Level() <= report.Warning {
				t.Logf("parse warning: %q", d)
			}
		}

		var actual strings.Builder
		for decl := range seq.Values(file.Decls()) {
			actual.WriteString(printer.Print(printer.Options{}, decl))
		}

		whole, err := printer.PrintFile(printer.Options{}, file)
		if err != nil {
			t.Fatalf("PrintFile: %v", err)
		}
		// The concatenated per-decl output must be a prefix of the
		// whole-file output. [printer.PrintFile] may emit trailing
		// detached trivia (e.g. EOF comments) that are not associated
		// with any decl, so it can have content after this prefix.
		if !strings.HasPrefix(whole, actual.String()) {
			if msg := golden.CompareAndDiff(actual.String(), whole); msg != "" {
				t.Errorf("Print over decls is not a prefix of PrintFile:\n%s", msg)
			}
		}
	})
}

// TestFormat exercises the printer's format mode against goldens in
// testdata/format. Each <name>.proto is formatted under two presets
// and compared against the corresponding golden:
//
//   - <name>.proto.legacy.txt: [printer.Legacy] — reproduces the
//     legacy protobuf formatter's behavior byte-for-byte.
//   - <name>.proto.default.txt: [printer.Default] — the recommended
//     modern preset (dynamic layout, width-aware breaking at 100
//     cols, no comment-text rewriting).
//
// Each preset's output must re-parse cleanly and be idempotent under
// a second format pass.
//
// To regenerate goldens:
//
//	PROTOCOMPILE_REFRESH=** go test ./experimental/ast/printer/... -run TestFormat
func TestFormat(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:       "testdata/format",
		Extensions: []string{"proto"},
		Refresh:    "PROTOCOMPILE_REFRESH",
		Outputs: []golden.Output{
			{Extension: "legacy.txt"},
			{Extension: "default.txt"},
		},
	}

	presets := []struct {
		label string
		opts  printer.Options
	}{
		{"legacy", printer.Options{Format: true, Formatting: printer.Legacy()}},
		{"default", printer.Options{Format: true, Formatting: printer.Default()}},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		errs := &report.Report{}
		file, _ := parser.Parse(path, source.NewFile(path, text), errs)
		hasParseErrors := false
		for _, d := range errs.Diagnostics {
			if d.Level() <= report.Error {
				hasParseErrors = true
			}
			if d.Level() <= report.Warning {
				t.Logf("parse warning: %q", d)
			}
		}

		for i, preset := range presets {
			got, err := printer.PrintFile(preset.opts, file)
			if err != nil {
				t.Errorf("[%s] PrintFile: %v", preset.label, err)
				continue
			}
			outputs[i] = got

			if hasParseErrors {
				continue
			}

			// Verify the output is valid protobuf by re-parsing it.
			errs2 := &report.Report{}
			file2, _ := parser.Parse(path, source.NewFile(path, got), errs2)
			for _, d := range errs2.Diagnostics {
				if d.Level() <= report.Error {
					t.Errorf("[%s] formatted output does not re-parse: %v", preset.label, d)
				}
			}

			// Verify idempotency.
			got2, err := printer.PrintFile(preset.opts, file2)
			if err != nil {
				t.Errorf("[%s] PrintFile (idempotency): %v", preset.label, err)
				continue
			}
			if msg := golden.CompareAndDiff(got2, got); msg != "" {
				t.Errorf("[%s] formatting is not idempotent:\n%s", preset.label, msg)
			}
		}
	})
}
