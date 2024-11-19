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

// package source provides standard queries and interfaces for accessing the
// contents of source files.
package source

import (
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/bufbuild/protocompile/experimental/incremental"
	"github.com/bufbuild/protocompile/experimental/report"
)

// Stage is the value this package uses for [report.Report].Stage.
const Stage int = 0

// Opener is a mechanism for opening files.
//
// Opener implementations are assumed by Protocompile to be comparable. It is
// sufficient to always ensure that the implementation uses a pointer receiver.
type Opener interface {
	// Open opens a file, potentially returning an error.
	//
	// The result should be a string, because the syntactic analysis framework
	// wants strings as inputs so that providing the contents of a file as a Go
	// string minimizes copies down the line.
	//
	// A return value of [fs.ErrNotExist] is given special treatment by some
	// Opener adapters, such as the [Openers] type.
	Open(path string) (string, error)
}

// Map implements [Opener] via map lookup.
//
// Missing entries result in [fs.ErrNotExist].
type Map map[string]string

// Open implements [Opener].
func (m *Map) Open(path string) (string, error) {
	text, ok := (*m)[path]
	if !ok {
		return "", fs.ErrNotExist
	}
	return text, nil
}

// FS wraps an [fs.FS] to give it an [Opener] interface.
type FS struct {
	fs.FS
}

// Open implements [Opener].
func (fs *FS) Open(path string) (string, error) {
	file, err := fs.FS.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf strings.Builder
	_, err = io.Copy(&buf, file)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Openers wraps a sequence of [Opener]s.
//
// When calling Open, it calls each Opener in sequence until one does not return
// [fs.ErrNotExist].
type Openers []Opener

// Open implements [Opener].
func (o *Openers) Open(path string) (string, error) {
	for _, opener := range *o {
		text, err := opener.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return text, err
	}
	return "", fs.ErrNotExist
}

// Contents is an [incremental.Query] for the contents of a file.
type Contents struct {
	Opener // Must be comparable.

	Path string
}

var _ incremental.Query[string] = Contents{}

// Key implements [incremental.Query].
//
// The key for a Contents query is the query itself. This means that a single
// [incremental.Executor] can host Contents queries for multiple Openers. It
// also means that the Openers must all be comparable. As the [Opener]
// documentation states, implementations should take a pointer receiver so that
// comparison uses object identity.
func (t Contents) Key() any {
	return t
}

// Execute implements [incremental.Query].
func (t Contents) Execute(incremental.Task) (value string, fatal error) {
	text, err := t.Open(t.Path)
	if err != nil {
		r := new(report.AsError)
		r.Report.Error(&report.ErrInFile{Err: err, Path: t.Path})
		return "", r
	}
	return text, nil
}
