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

// Package googleapis makes a checked-in download of
// https://github.com/googleapis/googleapis available for use by tests.
//
// The checked in data at googleapis-xxx.tar.gz is governed by
// https://github.com/googleapis/googleapis/blob/master/LICENSE.
package googleapis

//go:generate go run ./gen cb6fbe8784479b22af38c09a5039d8983e894566

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile/experimental/source"
)

var (
	opener    source.Opener
	workspace source.Workspace
	once      sync.Once
)

// Get returns a workspace and opener containing the entire googleapis project,
// for use in tests.
func Get() (source.Workspace, source.Opener) {
	once.Do(func() {
		protos, err := unpack(archive)
		if err != nil {
			panic(fmt.Errorf("googleapis: %w", err))
		}

		var paths []string
		for path := range protos.Get() {
			paths = append(paths, path)
		}
		slices.Sort(paths)

		opener = protos
		workspace = source.NewWorkspace(paths...)
	})

	return workspace, opener
}

// WriteTo writes the entire googleapis tree onto the given directory.
func WriteTo(dir string, perm os.FileMode) error {
	ws, op := Get()
	for _, path := range ws.Paths() {
		src, err := op.Open(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0777); err != nil {
			return err
		}

		f, err := os.OpenFile(filepath.Join(dir, path), os.O_CREATE|os.O_RDWR, perm)
		if err != nil {
			return err
		}

		_, err = f.WriteString(src.Text())
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func unpack(archive string) (opener source.Map, e error) {
	gz, err := gzip.NewReader(strings.NewReader(archive))
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

		buf := new(strings.Builder)
		if _, err := io.Copy(buf, ar); err != nil { //nolint:gosec
			return opener, err
		}

		opener.Add(hdr.Name, buf.String())
	}

	return opener, nil
}
