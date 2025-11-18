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
	"fmt"
	"iter"
	"math"
	"slices"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/source/length"
)

// Spanner is any type with a [Span].
type Spanner interface {
	// Should return the zero [Span] to indicate that it does not contribute
	// span information.
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

// Location is a user-displayable location within a source code file.
type Location struct {
	// The byte offset for this location.
	Offset int

	// The line and column for this location, 1-indexed.
	//
	// The units of measurement for column depend on the [length.Unit] used when
	// constructing it.
	//
	// Because these are 1-indexed, a zero Line can be used as a sentinel.
	Line, Column int
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

// After returns all text after this span.
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

// GrowRight returns a new span which contains the largest prefix of [Span.After]
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
	return s.Location(s.Start, length.TermWidth)
}

// EndLoc returns the end location for this span.
func (s Span) EndLoc() Location {
	return s.Location(s.End, length.TermWidth)
}

// Span implements [Spanner].
func (s Span) Span() Span {
	return s
}

// Range slices this span along the given byte indices.
//
// Unlike slicing into a string, out-of-bounds indices are snapped to the
// boundaries of the string, and negative indices are taken from the back of
// the span. For example, s.RuneRange(-2, -1) is the final rune of the span
// (or an empty span, if s is empty).
func (s Span) Range(i, j int) Span {
	i = idxToByteOffset(s.Text(), i)
	j = idxToByteOffset(s.Text(), j)
	if i > j {
		i, j = j, i
	}
	return s.File.Span(i+s.Start, j+s.Start)
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
		span := GetSpan(spanner)
		if span.IsZero() {
			continue
		}

		if joined.IsZero() {
			joined.File = span.File
		} else if joined.File != span.File {
			panic(fmt.Sprintf(
				"protocompile/source: passed spans with distinct files to JoinSpans(): %q != %q",
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

// GetSpan extracts a span from a Spanner, but returns the zero span when
// s is zero, which would otherwise panic.
func GetSpan(s Spanner) Span {
	if s == nil {
		return Span{}
	}
	return s.Span()
}

// idxToByteOffset converts a byte index into s into a byte offset.
//
// If i is negative, this produces the index of the -ith byte from the end of
// the string.
//
// If i > len(s) or i < -len(s), returns len(s) or 0, respectively; i is always
// valid to index into s with.
func idxToByteOffset(s string, i int) int {
	switch {
	case i > len(s):
		return len(s)
	case i < -len(s):
		return 0
	case i < 0:
		return len(s) + i
	default:
		return i
	}
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
