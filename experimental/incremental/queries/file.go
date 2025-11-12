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

package queries

import (
	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
)

// File is an [incremental.Query] for the contents of a file as provided
// by a [source.Opener].
//
// File queries with different Openers are considered distinct.
type File struct {
	source.Opener // Must be comparable.
	Path          string

	// If set, any errors generated from opening the file are logged as
	// diagnostics. Setting this to false is useful for cases where the
	// caller wants to emit a more general diagnostic.
	ReportError bool
}

var _ incremental.Query[*source.File] = File{}

// Key implements [incremental.Query].
//
// The key for a File query is the query itself. This means that a single
// [incremental.Executor] can host File queries for multiple Openers. It
// also means that the Openers must all be comparable. As the [Opener]
// documentation states, implementations should take a pointer receiver so that
// comparison uses object identity.
func (f File) Key() any {
	return f
}

// Execute implements [incremental.Query].
func (f File) Execute(t *incremental.Task) (*source.File, error) {
	if !f.ReportError {
		file, err := f.Open(f.Path)

		if err != nil {
			return nil, err
		}
		return file, nil
	}

	f.ReportError = false
	r, err := incremental.Resolve(t, f)
	if err != nil {
		return nil, err
	}

	err = r[0].Fatal
	if err != nil {
		t.Report().Errorf("%v", err).Apply(
			report.InFile(f.Path),
		)
		return nil, err
	}

	return r[0].Value, nil
}
