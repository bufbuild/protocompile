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
)

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

// Map implements [Opener] via lookup of a built-in map. This map is not
// directly accessible, to help avoid mistaken uses that cause different *Map
// pointer values (for the same built-in map value) to wind up in different
// queries, which breaks query caching.
//
// Missing entries result in [fs.ErrNotExist].
type Map struct {
	table map[string]string
}

// NewMap creates a new [Map] wrapping the given map.
//
// If passed nil, this will update the map to be an empty non-nil map.
func NewMap(m map[string]string) *Map {
	if m == nil {
		m = map[string]string{}
	}
	return &Map{m}
}

// Get returns the map this [Map] wraps. This can be used to modify the map.
//
// Never returns nil.
func (m *Map) Get() map[string]string {
	return m.table
}

// Open implements [Opener].
func (m *Map) Open(path string) (string, error) {
	text, ok := m.Get()[path]
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
