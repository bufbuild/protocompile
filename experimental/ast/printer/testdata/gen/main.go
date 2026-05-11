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

// Binary gen downloads the bufformat testdata from bufbuild/buf at a
// given commit and writes it under ../bufformat (i.e. the bufformat
// subdirectory of the testdata directory that contains this gen tool).
//
// Usage: go run ./testdata/gen <commit-sha>
//
// Invoked via the //go:generate directive in bufformat_test.go.
//
//nolint:gosec
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bufbuild/protocompile/internal/ext/bitsx"
	"github.com/bufbuild/protocompile/internal/ext/flagx"
)

// The prefix inside the GitHub-archive tarball that identifies the
// bufformat testdata directory. The leading `buf-<sha>/` segment is
// trimmed before this prefix matches.
const archivePrefix = "private/buf/bufformat/testdata/"

func main() {
	flagx.Main(func() (e error) {
		if flag.NArg() != 1 {
			return errors.New("usage: gen <commit-sha>")
		}
		commit := flag.Arg(0)

		// The gen binary is invoked via `go run ./testdata/gen` from
		// the printer package directory, so cwd is the printer
		// package. Write data into ./testdata/bufformat.
		outDir := filepath.Join("testdata", "bufformat")

		url := fmt.Sprintf("https://github.com/bufbuild/buf/archive/%s.tar.gz", commit)

		start := time.Now()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if resp.Body != nil {
			defer func() {
				if err := resp.Body.Close(); err != nil && e == nil {
					e = err
				}
			}()
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("downloading %s resulted in status code %s", url, resp.Status)
		}

		// Wipe the existing output directory so a re-generate from a
		// different SHA doesn't leave stale files behind.
		if err := os.RemoveAll(outDir); err != nil {
			return fmt.Errorf("removing %s: %w", outDir, err)
		}
		if err := os.MkdirAll(outDir, 0o777); err != nil {
			return fmt.Errorf("creating %s: %w", outDir, err)
		}

		archiveRoot := "buf-" + commit + "/"
		total, count, err := extract(resp.Body, outDir, archiveRoot)
		if err != nil {
			return err
		}

		fmt.Printf("bufformat testdata: extracted %d files (%v) from commit %v in %v\n",
			count, bitsx.ByteSize(total), commit, time.Since(start))
		return nil
	})
}

// extract reads a gzipped tarball from src, keeping `.proto` and
// `.golden` files under the bufformat testdata path, and writes each
// to outDir using a path relative to that testdata root.
//
// archiveRoot is the leading directory inside the tarball produced by
// GitHub's archive endpoint (e.g. "buf-<sha>/"). Entries outside that
// root or outside [archivePrefix] are skipped.
func extract(src io.Reader, outDir, archiveRoot string) (total int64, count int, err error) {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return 0, 0, err
	}
	ar := tar.NewReader(gz)

	for {
		hdr, err := ar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return total, count, err
		}
		if hdr == nil || hdr.Typeflag != tar.TypeReg {
			continue
		}

		name, ok := strings.CutPrefix(hdr.Name, archiveRoot)
		if !ok {
			continue
		}
		rel, ok := strings.CutPrefix(name, archivePrefix)
		if !ok {
			continue
		}
		switch filepath.Ext(rel) {
		case ".proto", ".golden":
		default:
			continue
		}

		dst := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o777); err != nil {
			return total, count, err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
		if err != nil {
			return total, count, err
		}
		n, copyErr := io.Copy(f, ar)
		closeErr := f.Close()
		if copyErr != nil {
			return total, count, copyErr
		}
		if closeErr != nil {
			return total, count, closeErr
		}
		total += n
		count++
	}
	return total, count, nil
}
