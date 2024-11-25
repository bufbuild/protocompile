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
}

var _ incremental.Query[*report.File] = File{}

// Key implements [incremental.Query].
//
// The key for a Contents query is the query itself. This means that a single
// [incremental.Executor] can host Contents queries for multiple Openers. It
// also means that the Openers must all be comparable. As the [Opener]
// documentation states, implementations should take a pointer receiver so that
// comparison uses object identity.
func (t File) Key() any {
	return t
}

// Execute implements [incremental.Query].
func (t File) Execute(incremental.Task) (*report.File, error) {
	text, err := t.Open(t.Path)
	if err != nil {
		r := newReport(stageFile)
		r.Report.Error(&report.ErrInFile{Err: err, Path: t.Path})
		return nil, r
	}

	return report.NewFile(t.Path, text), nil
}
