// Copyright 2020-2023 Buf Technologies, Inc.
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

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// GetProtocPath returns the path to an appropriate protoc executable. This
// path is created by the Makefile, so run `make test` instead of `go test ./...`
// to make sure the path is populated.
//
// The protoc executable is used by some tests to verify that the output of
// this repo matches the output of the reference compiler.
func GetProtocPath(rootDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, ".protoc_version"))
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	protocPath := filepath.Join(rootDir, fmt.Sprintf("internal/testdata/protoc/%s/bin/protoc", version))
	if runtime.GOOS == "windows" {
		protocPath += ".exe"
	}
	if info, err := os.Stat(protocPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s does not exist; run 'make test' from the top-level of this repo", protocPath)
		}
		return "", err
	} else if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, but should be an executable file", protocPath)
	}
	return protocPath, nil
}
