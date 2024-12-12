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

package report

// AsError wraps a [Report] as an [error].
type AsError struct {
	Report Report
}

// Error implements [error].
func (e *AsError) Error() string {
	text, _, _ := Renderer{Compact: true}.RenderString(&e.Report)
	return text
}

// ErrInFile wraps an [error] into a diagnostic on the given file.
type ErrInFile struct {
	Err  error
	Path string
}

var _ Diagnose = &ErrInFile{}

// Diagnose implements [Diagnose].
func (e *ErrInFile) Diagnose(d *Diagnostic) {
	d.Apply(
		Message("%v", e.Err),
		InFile(e.Path),
	)
}
