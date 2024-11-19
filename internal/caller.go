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

package internal

import (
	"path/filepath"
	"runtime"
)

// CallerDir returns the directory of the file in which this function is called.
//
// skip is the number of callers to skip, like in [runtime.Caller]. A value of
// zero represents the caller of CallerDir.
//
// This function is intended for tests to find their test data only. Panics
// if called within a stripped binary.
func CallerDir(skip int) string {
	_, file, _, ok := runtime.Caller(skip + 1)
	if !ok {
		panic("protocompile/internal: could not determine test file's directory; the binary may have been stripped")
	}
	return filepath.Dir(file)
}
