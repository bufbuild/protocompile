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

package report2

import (
	"slices"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/bufbuild/protocompile/internal/width"
)

// The size we render all tabstops as.
const TabstopWidth int = 4

// Span is any type that can be used to generate source code information for a diagnostic.
type Span interface {
	File() File
	Start() Location
	End() Location
}

// File is a source code file involved in a diagnostic.
type File struct {
	// The filesystem path for this string. It doesn't need to be a real path, but
	// it will be used to deduplicate spans according to their file.
	Path string

	// The complete text of the file.
	Text string
}

// Location is a user-displayable location within a source code file.
type Location struct {
	// The byte offset for this location.
	Offset int

	// The line and column for this location, 1-indexed.
	//
	// Note that Column is not Offset with the length of all
	// previous lines subtracted off; it takes into account the
	// Unicode width. The rune A is one column wide, the rune
	// 貓 is two columns wide, and the multi-rune emoji presentation
	// sequence 🐈‍⬛ is also two columns wide.
	//
	// Because these are 1-indexed, a zero Line can be used as a sentinel.
	Line, Column int

	// The ostensible UTF-16 codepoint offset from the start of the line
	// for this location. This exists for the benefit of LSP
	// implementations.
	UTF16 int
}

// IndexedFile is an index of line information from a [File], which permits
// O(log n) calculation of [Location]s from offsets.
type IndexedFile struct {
	file File

	once sync.Once
	// A prefix sum of the line lengths of text. Given a byte offset, it is possible
	// to recover which line that offset is on by performing a binary search on this
	// list.
	//
	// Alternatively, this slice can be interpreted as the index after each \n in the
	// original file.
	lines []int
	// Similar to the above, but instead using the length of each line in code units
	// if it was transcoded to UTF-16. This is required for compatibility with LSP.
	utf16Lines []int
}

// NewIndexedFile constructs a line index for the given text. This is O(n) in the size
// of the text.
func NewIndexedFile(file File) *IndexedFile {
	return &IndexedFile{file: file}
}

// File returns the file that this index indexes.
func (i *IndexedFile) File() File {
	return i.file
}

// Span generates a span using this index.
//
// This is mostly intended for convenience; generally speaking, users of package report
// will want to implement their own [Span] types that use a compressed representation,
// and delay computation of line and column information as late as possible.
func (i *IndexedFile) NewSpan(start, end int) Span {
	return naiveSpan{
		file:  i.File(),
		start: i.Search(start),
		end:   i.Search(end),
	}
}

// Search searches this index to build full Location information for the given byte
// offset.
func (i *IndexedFile) Search(offset int) Location {
	// Compute the prefix sum on-demand.
	i.once.Do(func() {
		var next, next16 int

		// We add 1 to the return value of IndexByte because we want to work
		// with the index immediately *after* the newline byte.
		text := i.file.Text
		for {
			newline := strings.IndexByte(text, '\n') + 1
			if newline == 0 {
				break
			}

			line := text[:newline]
			text = text[newline:]

			i.lines = append(i.lines, next)
			next += newline

			// Calculate the length of `line` in UTF-16 code units.
			var utf16Len int
			for _, r := range line {
				utf16Len += utf16.RuneLen(r)
			}

			i.utf16Lines = append(i.utf16Lines, next16)
			next16 += utf16Len
		}

		i.lines = append(i.lines, next)
		i.utf16Lines = append(i.utf16Lines, next16)
	})

	// Find the smallest index in c.liznes such that lines[line] <= offset.
	line, exact := slices.BinarySearch(i.lines, offset)
	if !exact {
		line--
	}

	// Calculate the column.
	chunk := i.file.Text[i.lines[line]:offset]
	column := width.Width(chunk, TabstopWidth)

	// Calculate the UTF-16 offset of of the offset within its line.
	var utf16Col int
	for _, r := range chunk {
		utf16Col += utf16.RuneLen(r)
	}

	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
		UTF16:  utf16Col,
	}
}

type naiveSpan struct {
	file       File
	start, end Location
}

func (s naiveSpan) File() File      { return s.file }
func (s naiveSpan) Start() Location { return s.start }
func (s naiveSpan) End() Location   { return s.end }
func (s naiveSpan) Span() Span      { return s }
