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

// Package golden provides a framework for writing file-based golden tests.
//
// The primary entry-point is [Corpus]. Define a new corpus in an ordinary Go
// test body and call [Corpus.Run] to execute it.
//
// Corpora can be "refreshed" automatically to update the golden test corpus
// with new data generated by the test instead of comparing it. To do this,
// run the test with the environment variable that [Corpus].Refresh names set
// to a file glob for all test files to regenerate expectations for.
package golden

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/pmezard/go-difflib/difflib"

	"github.com/bufbuild/protocompile/internal/prototest"
)

// A corpus describes a test data corpus. This is essentially a way for doing table-driven
// tests where the "table" is in your file system.
type Corpus struct {
	// The root of the test data directory. This path is relative to the directory of
	// the file that calls [Corpus.Run].
	Root string

	// An environment variable to check with regards to whether to run in "refresh"
	// mode or not.
	Refresh string

	// The file extensions (without a dot) of files which define a test case,
	// e.g. "proto".
	Extensions []string

	// Possible outputs of the test, which are found using Outputs.Extension.
	// If the file for a particular output is missing, it is implicitly treated
	// as being expected to be empty (i.e., if the file Output[n].Extension
	// specifies does not exist, then Output[n].Compare is passed the empty string
	// as the "want" value).
	Outputs []Output
}

// Run executes a golden test.
//
// The test function executes a single test case in the corpus, and writes the results to
// the entries of output, which will be the same length as Corpus.Outputs.
//
// test should write to outputs as early as possible to ensure that, if test panics, successfully
// created test output can still be shown to the user.
func (c Corpus) Run(t *testing.T, test func(t *testing.T, path, text string, outputs []string)) {
	testDir := prototest.CallerDirWithSkip(t, 1)
	root := filepath.Join(testDir, c.Root)
	t.Logf("corpora: searching for files in %q", root)

	// Enumerate the tests to run by walking the filesystem.
	var tests []string
	err := filepath.Walk(root, func(p string, fi fs.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}

		for _, extn := range c.Extensions {
			if strings.HasSuffix(p, "."+extn) {
				tests = append(tests, p)
				break
			}
		}

		return err
	})
	if err != nil {
		t.Fatal("corpora: error while stating testdata FS:", err)
	}

	// Check if a refresh has been requested.
	var refresh string
	if c.Refresh != "" {
		refresh = os.Getenv(c.Refresh)
		if !doublestar.ValidatePattern(refresh) {
			t.Fatalf("invalid glob: ")
		}
	}

	if refresh != "" {
		t.Logf("corpora: refreshing test data because %s=%s", c.Refresh, refresh)
		t.Fail()
	}

	// Execute the tests.
	for _, path := range tests {
		// Make sure the path is normalized regardless of platform. This
		// is necessary to avoid breakages on Windows.
		name, _ := filepath.Rel(testDir, path)
		name = filepath.ToSlash(name)
		testName, _ := filepath.Rel(root, path)
		testName = filepath.ToSlash(testName)
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("corpora: error while loading input file %q: %v", path, err)
			}

			input := string(bytes)
			results := make([]string, len(c.Outputs))

			//nolint:revive,predeclared // it's fine to use panic as a name here.
			panic, panicStack := catch(func() { test(t, name, input, results) })
			if panic != nil {
				t.Logf("test panicked: %v\n%s", panic, panicStack)
				t.Fail()
			}

			// If we panic, continue to run the tests. This helps with observability
			// by getting test results we managed to compute into a form the user can
			// inspect.

			refresh, _ := doublestar.Match(refresh, name)
			for i, output := range c.Outputs {
				if panic != nil && results[i] == "" {
					// If we panicked and the result is empty, this means there's a good
					// chance this result was not written to, so we skip doing anything
					// that would potentially be noisy.
					continue
				}

				path := fmt.Sprint(path, ".", output.Extension)

				if !refresh {
					bytes, err := os.ReadFile(path)

					if err != nil && !errors.Is(err, os.ErrNotExist) {
						t.Logf("corpora: error while loading output file %q: %v", path, err)
						t.Fail()
						continue
					}

					cmp := output.Compare
					if cmp == nil {
						cmp = CompareAndDiff
					}
					if err := cmp(results[i], string(bytes)); err != "" {
						t.Logf("output mismatch for %q:\n%s", path, err)
						t.Fail()
						continue
					}
				} else {
					if results[i] == "" {
						err := os.Remove(path)
						if err != nil && !errors.Is(err, os.ErrNotExist) {
							t.Logf("corpora: error while deleting output file %q: %v", path, err)
							t.Fail()
						}
					} else {
						err := os.WriteFile(path, []byte(results[i]), 0600)
						if err != nil {
							t.Logf("corpora: error while writing output file %q: %v", path, err)
							t.Fail()
						}
					}
				}
			}
		})
	}
}

// Output represents the output of a test case.
type Output struct {
	// The extension of the output. This is a suffix to the name of the
	// testcase's main file; so if Corpus.Extension is "proto", and this is
	// "stderr", for a test "foo.proto" the test runner will look for files
	// named "foo.proto.stderr".
	Extension string

	// The comparison function for this output. If nil, defaults to
	// [CompareAndDiff].
	Compare CompareFunc
}

// CompareFunc is a comparison function between strings, used in [Output].
//
// Returns empty string if the strings match, otherwise returns an error message.
type CompareFunc func(got, want string) string

// CompareAndDiff is a [CompareFunc] that returns a colorized diff of the two
// strings if they are not equal.
func CompareAndDiff(got, want string) string {
	if got == want {
		return ""
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(want),
		B:        difflib.SplitLines(got),
		FromFile: "want",
		ToFile:   "got",
		Context:  2,
	})
	if err != nil {
		return err.Error()
	}

	// Colorize the diff so it's easier to read. We're looking for lines that
	// start or end with a - or a +.
	lines := strings.Split(diff, "\n")
	for i := range lines {
		s := lines[i]
		if strings.HasPrefix(s, "+") {
			lines[i] = "\033[1;92m" + s + "\033[0m"
		} else if strings.HasPrefix(s, "-") {
			lines[i] = "\033[1;91m" + s + "\033[0m"
		}
	}

	return strings.Join(lines, "\n")
}

// catch runs cb and places any panic it results in panic.
//
//nolint:revive,predeclared // it's fine to use panic as a name here.
func catch(cb func()) (panic any, stack []byte) {
	defer func() {
		panic = recover()
		if panic != nil {
			stack = debug.Stack()
		}
	}()
	cb()
	return
}
