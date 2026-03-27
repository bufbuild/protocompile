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

package benchmark

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile/experimental/source"
)

var (
	googleapisCommit = flag.String("googleapis-commit", "cb6fbe8784479b22af38c09a5039d8983e894566", "the googleapis commit to use")

	googleapisOpener    source.Opener
	googleapisWorkspace source.Workspace
	googleapisOnce      sync.Once

	googleapisSkip = os.Getenv("SKIP_DOWNLOAD_GOOGLEAPIS") != ""
)

func GoogleapisProtos() (source.Workspace, source.Opener) {
	if googleapisSkip {
		return nil, nil
	}
	googleapisOnce.Do(func() {
		url := fmt.Sprintf("https://github.com/googleapis/googleapis/archive/%s.tar.gz", *googleapisCommit)
		dir := "googleapis-" + *googleapisCommit
		opener, err := download(context.Background(), url, func(path string) string {
			rel, err := filepath.Rel(dir, path)
			if err != nil || filepath.Ext(path) != ".proto" {
				return ""
			}
			return rel
		})
		if err != nil {
			panic(err)
		}

		var paths []string
		for path := range opener.Get() {
			paths = append(paths, path)
		}
		slices.Sort(paths)

		googleapisOpener = opener
		googleapisWorkspace = source.NewWorkspace(paths...)
	})
	return googleapisWorkspace, googleapisOpener
}

func download(ctx context.Context, url string, filter func(string) string) (opener source.Map, e error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return opener, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return opener, err
	}

	if resp.Body != nil {
		defer func() {
			if err = resp.Body.Close(); err != nil && e == nil {
				e = err
			}
		}()
	}

	if resp.StatusCode != http.StatusOK {
		return opener, fmt.Errorf("downloading %s resulted in status code %s", url, resp.Status)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return opener, err
	}

	ar := tar.NewReader(gz)
	opener = source.NewMap(nil)
	for {
		hdr, err := ar.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return opener, err
		}

		if hdr == nil || hdr.Typeflag != tar.TypeReg {
			continue
		}

		target := filter(hdr.Name)
		if target == "" {
			continue
		}

		buf := new(strings.Builder)
		if _, err := io.Copy(buf, ar); err != nil {
			return opener, err
		}

		opener.Add(target, buf.String())
	}

	return opener, nil
}
