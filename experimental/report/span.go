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

package report

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

//go:generate go run ../../internal/enum units.yaml

// Spanner is any type with a [Span].
type Spanner interface {
	// Should return the zero [Span] to indicate that it does not contribute
	// span information.
	Span() Span
}

// getSpan extracts a span from a Spanner, but returns the zero span when
// s is zero, which would otherwise panic.
func getSpan(s Spanner) Span {
	if s == nil {
		return Span{}
	}
	return s.Span()
}

// Span is a location within a [File].
type Span struct {
	// The file this span refers to. The file must be indexed, since we plan to
	// convert Start/End into editor coordinates.
	*File

	// The start and end byte offsets for this span.
	Start, End int
}

// IsZero returns whether or not this is the zero span.
func (s Span) IsZero() bool {
	return s.File == nil
}

// Text returns the text corresponding to this span.
func (s Span) Text() string {
	return s.File.Text()[s.Start:s.End]
}

// Indentation calculates the indentation at this span.
//
// Indentation is defined as the substring between the last newline in
// [Span.Before] and the first non-Pattern_White_Space after that newline.
func (s Span) Indentation() string {
	return s.File.Indentation(s.Start)
}

// Before returns all text before this span.
func (s Span) Before() string {
	return s.File.Text()[:s.Start]
}

// Before returns all text after this span.
func (s Span) After() string {
	return s.File.Text()[s.End:]
}

// GrowLeft returns a new span which contains the largest suffix of [Span.Before]
// which match p.
func (s Span) GrowLeft(p func(r rune) bool) Span {
	for {
		r, sz := utf8.DecodeLastRuneInString(s.Before())
		if r == utf8.RuneError || !p(r) {
			break
		}
		s.Start -= sz
	}
	return s
}

// GrowLeft returns a new span which contains the largest prefix of [Span.After]
// which match p.
func (s Span) GrowRight(p func(r rune) bool) Span {
	for {
		r, sz := utf8.DecodeRuneInString(s.After())
		if r == utf8.RuneError || !p(r) {
			break
		}
		s.End += sz
	}
	return s
}

// Len returns the length of this span, in bytes.
func (s Span) Len() int {
	return s.End - s.Start
}

// StartLoc returns the start location for this span.
func (s Span) StartLoc() Location {
	return s.Location(s.Start, TermWidth)
}

// EndLoc returns the end location for this span.
func (s Span) EndLoc() Location {
	return s.Location(s.End, TermWidth)
}

// Span implements [Spanner].
func (s Span) Span() Span {
	return s
}

// RuneRange slices this span along the given rune indices.
//
// For example, s.RuneRange(0, 2) returns at most the first two runes of the
// span.
//
// Unlike slicing into a string, out-of-bounds indices are snapped to the
// boundaries of the string, and negative indices are taken from the back of
// the span. For example, s.RuneRange(-2, -1) is the final rune of the span
// (or an empty span, if s is empty).
func (s Span) RuneRange(i, j int) Span {
	i = runeIdxToByteOffset(s.Text(), i)
	j = runeIdxToByteOffset(s.Text(), j)
	if i > j {
		i, j = j, i
	}
	return s.File.Span(i+s.Start, j+s.Start)
}

// Rune is a shorthand for RuneRange(i, i+1) or RuneRange(i-1, i), depending
// on the sign of i.
func (s Span) Rune(i int) Span {
	if i < 0 {
		return s.RuneRange(i-1, i)
	}
	return s.RuneRange(i, i+1)
}

// runeIdxToByteOffset converts a rune index into s into a byte offset.
//
// If i is negative, this produces the index of the -ith rune from the end of
// the string.
//
// If i > len(s) or i < -len(s), returns len(s) or 0, respectively; i is always
// valid to index into s with.
func runeIdxToByteOffset(s string, i int) int {
	for i < 0 {
		i++
		if i == 0 || s == "" {
			return len(s)
		}
		_, j := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-j]
	}

	for j := range s {
		if i == 0 {
			return j
		}
		i--
	}
	return len(s)
}

// String implements [string.Stringer].
func (s Span) String() string {
	start := s.StartLoc()
	return fmt.Sprintf("%q:%d:%d[%d:%d]", s.Path(), start.Line, start.Column, s.Start, s.End)
}

// Join joins a collection of spans, returning the smallest span that
// contains all of them.
//
// IsZero spans among spans are ignored. If every span in spans is zero, returns
// the zero span.
//
// If there are at least two distinct files among the non-zero spans,
// this function panics.
func Join(spans ...Spanner) Span {
	return JoinSeq[Spanner](slices.Values(spans))
}

// JoinSeq is like [Join], but takes a sequence of any spannable type.
func JoinSeq[S Spanner](seq iter.Seq[S]) Span {
	joined := Span{Start: math.MaxInt}
	for spanner := range seq {
		span := getSpan(spanner)
		if span.IsZero() {
			continue
		}

		if joined.IsZero() {
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
	// The units of measurement for column depend on the [LengthUnit] used when
	// constructing it.
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

// Location searches this index to build full Location information for the given
// byte offset.
//
// This operation is O(log n).
func (f *File) Location(offset int, units LengthUnit) Location {
	if f == nil && offset == 0 {
		return Location{Offset: 0, Line: 1, Column: 1}
	}

	return f.location(offset, units, true)
}

// InverseLocation inverts the operation in [File.Location].
//
// line and column should be 1-indexed, and units should be the units used to
// measure the column width. If units is [TermWidth], this function panics,
// because inverting a [TermWidth] location is not supported.
func (f *File) InverseLocation(line, column int, units LengthUnit) Location {
	if f == nil && line == 1 && column == 1 {
		return Location{Offset: 0, Line: 1, Column: 1}
	}

	return Location{
		Line: line, Column: column,
		Offset: f.inverseLocation(line, column, units),
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

// LineOffsets returns the given line, including its trailing newline.
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

func (f *File) location(offset int, units LengthUnit, allowNonPrint bool) Location {
	lines := f.lines()

	// Find the smallest index in c.lines such that lines[line] <= offset.
	line, exact := slices.BinarySearch(lines, offset)
	if !exact {
		line--
	}

	chunk := f.Text()[lines[line]:offset]
	var column int
	switch units {
	case RuneLength:
		for range chunk {
			column++
		}
	case ByteLength:
		column = len(chunk)
	case UTF16Length:
		for _, r := range chunk {
			column += utf16.RuneLen(r)
		}
	case TermWidth:
		column = stringWidth(0, chunk, allowNonPrint, nil)
	}

	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
	}
}

func (f *File) inverseLocation(line, column int, units LengthUnit) int {
	// Find the start the given line.
	start, end := f.LineOffsets(line)
	chunk := f.text[start:end]
	var offset int
	switch units {
	case RuneLength:
		for offset = range chunk {
			column--
			if column <= 0 {
				break
			}
		}
	case ByteLength:
		offset = column - 1
	case UTF16Length:
		var r rune
		for offset, r = range chunk {
			column -= utf16.RuneLen(r)
			if column <= 0 {
				break
			}
		}
	case TermWidth:
		panic("protocompile/report: passed TermWidth to File.InvertLocation")
	}

	return start + offset
}

func (f *File) lines() []int {
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
