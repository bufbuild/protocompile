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

package parser

import (
	"math"

	"github.com/bufbuild/protocompile/experimental/report"
)

// MaxFileSize is the maximum file size Protocompile supports.
const MaxFileSize int = math.MaxInt32 // 2GB

// ErrFileTooBig diagnoses a file that is beyond Protocompile's implementation limits.
type ErrFileTooBig struct {
	Path string // The path of the offending file.
}

// Error implements [error].
func (e ErrFileTooBig) Error() string {
	return "files larger than 2GB are not supported"
}

// Diagnose implements [report.Diagnose].
func (e ErrFileTooBig) Diagnose(d *report.Diagnostic) {
	d.With(report.InFile(e.Path))
}

// ErrNotUTF8 diagnoses a file that contains non-UTF-8 bytes.
type ErrNotUTF8 struct {
	Path string // The path of the offending file.
	At   int    // The byte offset at which non-UTF-8 bytes occur.
	Byte byte   // The offending byte.
}

// Error implements [error].
func (e ErrNotUTF8) Error() string {
	return "files must be encoded as valid UTF-8"
}

// Diagnose implements [report.Diagnose].
func (e ErrNotUTF8) Diagnose(d *report.Diagnostic) {
	d.With(
		report.InFile(e.Path),
		report.Notef("unexpected 0x%02x byte at offset %d", e.Byte, e.At),
	)
}
