// Copyright 2020-2026 Buf Technologies, Inc.
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
	"cmp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
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

	once, checkpointsOnce sync.Once
	// A prefix sum of the line lengths of text. Given a byte offset, it is possible
	// to recover which line that offset is on by performing a binary search on this
	// list.
	//
	// Alternatively, this slice can be interpreted as the index after each \n in the
	// original file.
	lineIndex []int

	// Column checkpoints on long lines, sorted by offset. See [checkpoint].
	checkpoints []checkpoint
}

// checkpointStride is the minimum distance in bytes between two checkpoints
// on the same line. Lines no longer than this have no checkpoints.
const checkpointStride = 256

// checkpoint records the column of an offset on a long line in each unit, so
// that computing a location only needs to measure the text after the nearest
// preceding checkpoint, rather than everything since the start of the line.
type checkpoint struct {
	offset, runes, utf16 int
	// Terminal width with and without escaping non-printable characters.
	width, widthEscaped int
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

	cp := f.checkpoint(lines[line], offset)
	chunk := f.Text()[cp.offset:offset]
	var column int
	switch units {
	case length.Runes:
		column = cp.runes
		for range chunk {
			column++
		}
	case length.Bytes:
		column = offset - lines[line]
	case length.UTF16:
		column = cp.utf16
		for _, r := range chunk {
			column += utf16.RuneLen(r)
		}
	case length.TermWidth:
		w := &unicodex.Width{
			Column:         cp.width,
			EscapeNonPrint: !allowNonPrint,
		}
		if !allowNonPrint {
			w.Column = cp.widthEscaped
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

// checkpoint returns the last checkpoint at or before offset on the line that
// starts at start, or a checkpoint for start itself if there is none.
func (f *File) checkpoint(start, offset int) checkpoint {
	f.checkpointsOnce.Do(f.buildCheckpoints)

	i, exact := slices.BinarySearchFunc(f.checkpoints, offset, func(cp checkpoint, offset int) int {
		return cmp.Compare(cp.offset, offset)
	})
	if !exact {
		i--
	}
	if i < 0 || f.checkpoints[i].offset < start {
		return checkpoint{offset: start}
	}
	return f.checkpoints[i]
}

func (f *File) buildCheckpoints() {
	text := f.Text()
	lines := f.lines()
	for i, start := range lines {
		end := len(text)
		if i+1 < len(lines) {
			end = lines[i+1]
		}

		cp := checkpoint{offset: start}
		width := unicodex.Width{}
		escaped := unicodex.Width{EscapeNonPrint: true}
		for next := start + checkpointStride; next < end; next++ {
			// Only place checkpoints between two ASCII bytes other than \r\n.
			// That is always a grapheme cluster boundary, so measuring the
			// line in two pieces gives the same column as measuring it whole.
			if text[next-1] >= utf8.RuneSelf || text[next] >= utf8.RuneSelf ||
				(text[next-1] == '\r' && text[next] == '\n') {
				continue
			}

			chunk := text[cp.offset:next]
			for _, r := range chunk {
				cp.runes++
				cp.utf16 += utf16.RuneLen(r)
			}
			_, _ = width.WriteString(chunk)
			_, _ = escaped.WriteString(chunk)
			cp.offset = next
			cp.width = width.Column
			cp.widthEscaped = escaped.Column
			f.checkpoints = append(f.checkpoints, cp)

			next += checkpointStride - 1
		}
	}
}
