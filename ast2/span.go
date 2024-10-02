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

import (
	"math"

	"github.com/bufbuild/protocompile/report2"
)

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

var _ report2.Span = Span{}

// Span implements [Spanner] for Span.
func (s Span) Span() Span {
	return s
}

// Offsets returns the byte offsets for this span.
func (s Span) Offsets() (start, end int) {
	return s.start, s.end
}

// File returns the file this span is for.
func (s Span) File() report2.File {
	return s.Context().file.File()
}

// Start returns the start location for this span.
func (s Span) Start() report2.Location {
	return s.Context().file.Search(s.start)
}

// Start returns the end location for this span.
func (s Span) End() report2.Location {
	return s.Context().file.Search(s.end)
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
