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

// package corpora provides a mechanism for managing test corpora, i.e.,
// a collection of files that define some kind of compiler test.
package corpora

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/pmezard/go-difflib/difflib"
)

// A corpus describes a test data corpus. This is essentially a way for doing table-driven
// tests where the "table" is in your file system.
type Corpus struct {
	// The root of the test data directory. This path is relative to the file that
	// calls [Corpus.Run].
	Root string

	// An environment variable to check with regards to whether to run in "refresh"
	// mode or not.
	Refresh string

	// The file extension (without a dot) of files which define a test case,
	// e.g. "proto".
	Extension string
	// Possible outputs of the test, which are found using Outputs.Extension.
	// If the file for a particular output is missing, it is implicitly treated
	// as being expected to be empty (i.e., if the file Output[n].Extension
	// specifies does not exist, then Output[n].Compare is passed the empty string
	// as the "want" value).
	Outputs []Output

	// Run executes the test on one test case from the corpus. Returns a slice
	// of strings corresponding to the elements of Outputs.
	Test func(t *testing.T, path, text string) []string
}

func (c Corpus) Run(t *testing.T) {
	testDir := callerDir(0)
	root := filepath.Join(testDir, c.Root)
	t.Logf("corpora: searching for files in %q", root)

	// Enumerate the tests to run by walking the filesystem.
	var tests []string
	err := filepath.Walk(root, func(p string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && strings.TrimPrefix(path.Ext(p), ".") == c.Extension {
			tests = append(tests, p)
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
		name, _ := filepath.Rel(testDir, path)
		t.Run(name, func(t *testing.T) {
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("corpora: error while loading input file %q: %v", path, err)
			}

			input := string(bytes)
			results := c.Test(t, name, input)

			refresh, _ := doublestar.Match(refresh, name)
			for i, output := range c.Outputs {
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
						cmp = defaultCompare
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
						os.WriteFile(path, []byte(results[i]), 0770)
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

	// The comparison function for this output. May be nil, in which case the
	// values will be compared byte-for-byte.
	Compare Compare
}

// Compare is a comparison function between strings, used in [Output].
//
// Returns empty string if the strings match, otherwise returns an error message.
type Compare func(got, want string) string

func defaultCompare(got, want string) string {
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

func callerDir(skip int) string {
	_, file, _, ok := runtime.Caller(skip + 2)
	if !ok {
		panic("corpora: could not determine test file's directory")
	}
	return filepath.Dir(file)
}
