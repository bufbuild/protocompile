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

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/bufbuild/protocompile/experimental/internal"
)

// TabstopWidth is the size we render all tabstops as.
const TabstopWidth int = 4

// Spanner is any type with a span.
type Spanner interface {
	Span() Span
}

// Span is a location within a [File].
type Span struct {
	// The file this span refers to. The file must be indexed, since we plan to
	// convert Start/End into editor coordinates.
	*File

	// The start and end byte offsets for this span.
	Start, End int
}

// Nil returns whether or not this is the "nil" span.
func (s Span) Nil() bool {
	return s.File == nil
}

// Text returns the text corresponding to this span.
func (s Span) Text() string {
	return s.File.Text()[s.Start:s.End]
}

// StartLoc returns the start location for this span.
func (s Span) StartLoc() Location {
	return s.Location(s.Start)
}

// EndLoc returns the end location for this span.
func (s Span) EndLoc() Location {
	return s.Location(s.End)
}

// Span implements [Spanner].
func (s Span) Span() Span {
	return s
}

// String implements [string.Stringer].
func (s Span) String() string {
	start := s.StartLoc()
	return fmt.Sprintf("%q:%d:%d[%d:%d]", s.Path(), start.Line, start.Column, s.Start, s.End)
}

// Join joins a collection of spans, returning the smallest span that
// contains all of them.
//
// Nil spans among spans are ignored. If every span in spans is nil, returns
// the nil span.
//
// If there are at least two distinct non-nil files among the spans,
// this function panics.
func Join(spans ...Spanner) Span {
	joined := Span{Start: math.MaxInt}
	for _, span := range spans {
		if internal.Nil(span) {
			continue
		}

		span := span.Span()
		if span.File == nil {
			continue
		}

		if joined.File == nil {
			joined.File = span.File
		} else if joined.File != span.File {
			panic(fmt.Sprintf(
				"protocompile/report: passed spans with distinct files to JoinSpans(): %q != %q",
				joined.File.Path(),
				span.File.Path(),
			))
		}

		joined.Start = min(joined.Start, span.Start)
		joined.End = max(joined.End, span.End)
	}

	if joined.File == nil {
		return Span{}
	}
	return joined
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
}

// File is a source code file involved in a diagnostic.
//
// It contains additional book-keeping information for resolving span locations.
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
	lines []int
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

// Location searches this index to build full Location information for the given
// byte offset.
//
// This operation is O(log n).
func (f *File) Location(offset int) Location {
	if f == nil && offset == 0 {
		return Location{Offset: 0, Line: 1, Column: 1}
	}

	return f.location(offset, true)
}

// Span is a shorthand for creating a new Span.
func (f *File) Span(start, end int) Span {
	if f == nil {
		return Span{}
	}

	return Span{f, start, end}
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

func (f *File) location(offset int, allowNonPrint bool) Location {
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

			f.lines = append(f.lines, next)
			next += newline
		}

		f.lines = append(f.lines, next)
	})

	// Find the smallest index in c.lines such that lines[line] <= offset.
	line, exact := slices.BinarySearch(f.lines, offset)
	if !exact {
		line--
	}

	column := stringWidth(0, f.Text()[f.lines[line]:offset], allowNonPrint, nil)
	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
	}
}
