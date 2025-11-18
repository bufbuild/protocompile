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

package source

import (
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	_ "unsafe" // For go:linkname.

	"github.com/bufbuild/protocompile/experimental/source/length"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

// File is a source code file involved in a diagnostic.
//
// It contains additional book-keeping information for resolving span locations.
// Files are immutable once created.
//
// A nil *File behaves like an empty file with the path name "".
type File struct {
	path, text string

	once sync.Once
	// A prefix sum of the line lengths of text. Given a byte offset, it is possible
	// to recover which line that offset is on by performing a binary search on this
	// list.
	//
	// Alternatively, this slice can be interpreted as the index after each \n in the
	// original file.
	lineIndex []int
}

// NewFile constructs a new source file.
func NewFile(path, text string) *File {
	return &File{path: path, text: text}
}

// Path returns this file's filesystem path.
//
// It doesn't need to be a real path, but it will be used to deduplicate spans
// according to their file.
func (f *File) Path() string {
	if f == nil {
		return ""
	}

	return f.path
}

// Text returns this file's textual contents.
func (f *File) Text() string {
	if f == nil {
		return ""
	}

	return f.text
}

// LineByOffset searches this index to find the line number for the line
// containing this byte offset.
//
// This operation is O(log n).
func (f *File) LineByOffset(offset int) (number int) {
	lines := f.lines()

	// Find the smallest index in c.lines such that lines[line] <= offset.
	line, exact := slices.BinarySearch(lines, offset)
	if !exact {
		line--
	}

	return line
}

// Location searches this index to build full Location information for the given
// byte offset.
//
// This operation is O(log n).
func (f *File) Location(offset int, units length.Unit) Location {
	if f == nil || offset == 0 {
		return Location{Offset: 0, Line: 1, Column: 1}
	}

	return location(f, offset, units, true)
}

// InverseLocation inverts the operation in [File.Location].
//
// line and column should be 1-indexed, and units should be the units used to
// measure the column width. If units is [TermWidth], this function panics,
// because inverting a [TermWidth] location is not supported.
func (f *File) InverseLocation(line, column int, units length.Unit) Location {
	if f == nil || (line == 1 && column == 1) {
		return Location{Offset: 0, Line: 1, Column: 1}
	}

	return Location{
		Line: line, Column: column,
		Offset: inverseLocation(f, line, column, units),
	}
}

// Indentation calculates the indentation some offset.
//
// Indentation is defined as the substring between the last newline in
// before the offset and the first non-Pattern_White_Space after that newline.
func (f *File) Indentation(offset int) string {
	nl := strings.LastIndexByte(f.Text()[:offset], '\n') + 1
	margin := strings.IndexFunc(f.Text()[nl:], func(r rune) bool {
		return !unicode.In(r, unicode.Pattern_White_Space)
	})
	return f.Text()[nl : nl+margin]
}

// Span is a shorthand for creating a new Span.
func (f *File) Span(start, end int) Span {
	if f == nil {
		return Span{}
	}

	return Span{f, start, end}
}

// Line returns the given line, including its trailing newline.
//
// line is expected to be 1-indexed.
func (f *File) Line(line int) string {
	start, end := f.LineOffsets(line)
	return f.text[start:end]
}

// LineOffsets returns the offsets for the given line, including its trailing
// newline.
//
// line is expected to be 1-indexed.
func (f *File) LineOffsets(line int) (start, end int) {
	lines := f.lines()
	if len(lines) == line {
		return lines[line-1], len(f.Text())
	}
	return lines[line-1], lines[line]
}

// EOF returns a Span pointing to the end-of-file.
func (f *File) EOF() Span {
	if f == nil {
		return Span{}
	}

	// Find the last non-space rune; we moor the span immediately after it.
	eof := strings.LastIndexFunc(f.Text(), func(r rune) bool {
		return !unicode.In(r, unicode.Pattern_White_Space)
	})
	if eof == -1 {
		eof = 0 // The whole file is whitespace.
	}

	return f.Span(eof+1, eof+1)
}

//go:linkname location github.com/bufbuild/protocompile/experimental/report.fileLocation
func location(f *File, offset int, units length.Unit, allowNonPrint bool) Location {
	lines := f.lines()

	// Find the smallest index in c.lines such that lines[line] <= offset.
	line, exact := slices.BinarySearch(lines, offset)
	if !exact {
		line--
	}

	chunk := f.Text()[lines[line]:offset]
	var column int
	switch units {
	case length.Runes:
		for range chunk {
			column++
		}
	case length.Bytes:
		column = len(chunk)
	case length.UTF16:
		for _, r := range chunk {
			column += utf16.RuneLen(r)
		}
	case length.TermWidth:
		w := &unicodex.Width{
			EscapeNonPrint: !allowNonPrint,
		}
		_, _ = w.WriteString(chunk)
		column = w.Column
	}

	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
	}
}

func inverseLocation(f *File, line, column int, units length.Unit) int {
	// Find the start the given line.
	start, end := f.LineOffsets(line)
	chunk := f.text[start:end]
	var offset int
	switch units {
	case length.Runes:
		for offset = range chunk {
			column--
			if column <= 0 {
				break
			}
		}
		offset += column
	case length.Bytes:
		offset = column - 1
	case length.UTF16:
		var r rune
		for offset, r = range chunk {
			column -= utf16.RuneLen(r)
			if column <= 0 {
				break
			}
		}
		if column > 0 {
			offset += column
		}
	case length.TermWidth:
		panic("protocompile/source: passed TermWidth to File.InvertLocation")
	}

	return start + offset
}

func (f *File) lines() []int {
	if f == nil {
		return nil
	}

	// Compute the prefix sum on-demand.
	f.once.Do(func() {
		var next int

		// We add 1 to the return value of IndexByte because we want to work
		// with the index immediately *after* the newline byte.
		text := f.Text()
		for {
			newline := strings.IndexByte(text, '\n') + 1
			if newline == 0 {
				break
			}

			text = text[newline:]

			f.lineIndex = append(f.lineIndex, next)
			next += newline
		}

		f.lineIndex = append(f.lineIndex, next)
	})
	return f.lineIndex
}
