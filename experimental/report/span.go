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
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// TabstopWidth is the size we render all tabstops as.
const TabstopWidth int = 4

// File is a source code file involved in a diagnostic.
type File struct {
	// The filesystem path for this string. It doesn't need to be a real path, but
	// it will be used to deduplicate spans according to their file.
	Path string

	// The complete text of the file.
	Text string
}

// Spanner is any type with a span.
type Spanner interface {
	Span() Span
}

// Span is a location within a [File].
type Span struct {
	// The file this span refers to. The file must be indexed, since we plan to
	// convert Start/End into editor coordinates.
	*IndexedFile

	// The start and end byte offsets for this span.
	Start, End int
}

// Text returns the text corresponding to this span.
func (s Span) Text() string {
	return s.File().Text[s.Start:s.End]
}

// StartLoc returns the start location for this span.
func (s Span) StartLoc() Location {
	return s.Search(s.Start)
}

// EndLoc returns the end location for this span.
func (s Span) EndLoc() Location {
	return s.Search(s.End)
}

// Span implements [Spanner].
func (s Span) Span() Span {
	return s
}

// String implements [string.Stringer].
func (s Span) String() string {
	return fmt.Sprintf("%s[%d:%d]", s.Path(), s.Start, s.End)
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
		if span == nil {
			continue
		}
		span := span.Span()
		if joined.IndexedFile == nil {
			joined.IndexedFile = span.IndexedFile
		} else if joined.IndexedFile != span.IndexedFile {
			panic("protocompile/report: passed spans with distinct files to JoinSpans()")
		}

		joined.Start = min(joined.Start, span.Start)
		joined.End = max(joined.End, span.End)
	}

	if joined.IndexedFile == nil {
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

// Path returns i.File().Path.
func (i *IndexedFile) Path() string {
	return i.File().Path
}

// Text returns i.File().Text.
func (i *IndexedFile) Text() string {
	return i.File().Text
}

// Search searches this index to build full Location information for the given byte
// offset.
func (i *IndexedFile) Search(offset int) Location {
	return i.search(offset, true)
}

func (i *IndexedFile) search(offset int, allowNonPrint bool) Location {
	// Compute the prefix sum on-demand.
	i.once.Do(func() {
		var next int

		// We add 1 to the return value of IndexByte because we want to work
		// with the index immediately *after* the newline byte.
		text := i.file.Text
		for {
			newline := strings.IndexByte(text, '\n') + 1
			if newline == 0 {
				break
			}

			text = text[newline:]

			i.lines = append(i.lines, next)
			next += newline
		}

		i.lines = append(i.lines, next)
	})

	// Find the smallest index in c.lines such that lines[line] <= offset.
	line, exact := slices.BinarySearch(i.lines, offset)
	if !exact {
		line--
	}

	column := stringWidth(0, i.file.Text[i.lines[line]:offset], allowNonPrint, nil)
	return Location{
		Offset: offset,
		Line:   line + 1,
		Column: column + 1,
	}
}

// stringWidth calculates the rendered width of text if placed at the given column,
// accounting for tabstops.
func stringWidth(column int, text string, allowNonPrint bool, out *strings.Builder) int {
	// We can't just use StringWidth, because that doesn't respect tabstops
	// correctly.
	for text != "" {
		nextTab := strings.IndexByte(text, '\t')
		haveTab := nextTab != -1
		next := text
		if haveTab {
			next, text = text[:nextTab], text[nextTab+1:]
		} else {
			text = ""
		}

		if !allowNonPrint {
			// Handle unprintable characters. We render those as <U+NNNN>.
			for next != "" {
				nextNonPrint := strings.IndexFunc(next, NonPrint)
				chunk := next
				if nextNonPrint != -1 {
					chunk, next = next[:nextNonPrint], next[nextNonPrint:]
					nonPrint, runeLen := utf8.DecodeRuneInString(next)
					next = next[runeLen:]

					escape := fmt.Sprintf("<U+%04X>", nonPrint)
					if out != nil {
						out.WriteString(chunk)
						out.WriteString(escape)
					}

					column += uniseg.StringWidth(chunk) + len(escape)
				} else {
					if out != nil {
						out.WriteString(chunk)
					}
					column += uniseg.StringWidth(chunk)
					next = ""
				}
			}
		} else {
			column += uniseg.StringWidth(next)
			if out != nil {
				out.WriteString(next)
			}
		}

		if haveTab {
			tab := TabstopWidth - (column % TabstopWidth)
			column += tab
			if out != nil {
				padBy(out, tab)
			}
		}
	}
	return column
}

// NonPrint defines whether or not a rune is considered "unprintable for the
// purposes of diagnostics", that is, whether it is a rune that the diagnostics
// engine will replace with <U+NNNN> when printing.
func NonPrint(r rune) bool {
	return !strings.ContainsRune(" \r\t\n", r) && !unicode.IsPrint(r)
}
