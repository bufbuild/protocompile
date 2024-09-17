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

package ast2

import "math"

// Spanner is any type that has a span, given as a range of tokens.
type Spanner interface {
	Contextual

	// Returns this value's span, if known.
	//
	// Some nodes may not have spans, such as those produced synthetically
	// during rewrites. In this case, the returned span will be the zero
	// value.
	Span() Span
}

// Span is a source code span, i.e., a range of tokens.
//
// Spans are used primarily for error reporting.
type Span struct {
	withContext

	start, end int
}

// Span implements [Spanner] for Span.
func (s Span) Span() Span {
	return s
}

// Offsets returns the byte offsets for this span.
func (s Span) Offsets() (start, end int) {
	return s.start, s.end
}

// Start returns the start location for this span.
func (s Span) Start() Location {
	return s.Context().Location(s.start)
}

// Start returns the end location for this span.
func (s Span) End() Location {
	return s.Context().Location(s.end)
}

// JoinSpans joins a collection of spans, returning the smallest span that
// contains all of them.
//
// Nil spans among spans are ignored. If every span in spans is nil, returns
// the nil span.
//
// If there are at least two distinct non-nil contexts among the spans,
// this function panics.
func JoinSpans(spans ...Spanner) Span {
	span := Span{start: math.MaxInt}
	for _, span := range spans {
		span := span.Span()
		if span.ctx == nil {
			span.ctx = span.Context()
		} else if span.ctx != span.Context() {
			panic("protocompile/ast: passed spans with incompatible contexts to JoinSpans()")
		}

		span.start = min(span.start, span.start)
		span.end = max(span.end, span.end)
	}

	if span.ctx == nil {
		return Span{}
	}
	return span
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
	Line, Column int

	// The ostensible UTF-16 codepoint offset from the start of the line
	// for this location. This exists for the benefit of LSP
	// implementations.
	UTF16 int
}
