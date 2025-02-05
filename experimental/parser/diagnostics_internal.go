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

package parser

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// errUnexpected is a low-level parser error for when we hit a token we don't
// know how to handle.
type errUnexpected struct {
	// The unexpected thing (may be a token or AST node).
	what report.Spanner

	// The context we're in. Should be format-able with %v.
	where taxa.Place
	// Useful when where is an "after" position: if non-nil, this will be
	// highlighted as "previous where.Object is here"
	prev report.Spanner

	// What we wanted vs. what we got. Got can be used to customize what gets
	// shown, but if it's not set, we call describe(what) to get a user-visible
	// description.
	want taxa.Set
	got  any

	// If nonempty, inserting this text will be suggested at the given offset.
	insert        string
	insertAt      int
	insertJustify int
	stream        *token.Stream
}

func (e errUnexpected) Diagnose(d *report.Diagnostic) {
	got := e.got
	if got == nil {
		got = taxa.Classify(e.what)
		if got == taxa.Unknown {
			got = "tokens"
		}
	}

	var message report.DiagnosticOption
	if e.where.Subject() == taxa.Unknown {
		message = report.Message("unexpected %v", got)
	} else {
		message = report.Message("unexpected %v %v", got, e.where)
	}

	what := e.what.Span()
	snippet := report.Snippet(what)
	if e.want.Len() > 0 {
		snippet = report.Snippetf(what, "expected %v", e.want.Join("or"))
	}

	d.Apply(
		message,
		snippet,
		report.Snippetf(e.prev, "previous %v is here", e.where.Subject()),
	)

	if e.insert != "" {
		want, _ := iterx.First(e.want.All())
		d.Apply(justify(
			e.stream,
			what,
			fmt.Sprintf("consider inserting a %v", want),
			justified{
				report.Edit{Replace: e.insert},
				e.insertJustify,
			},
		))
	}
}

// errMoreThanOne is used to diagnose the occurrence of some construct more
// than one time, when it is expected to occur at most once.
type errMoreThanOne struct {
	first, second report.Spanner
	what          taxa.Noun
}

func (e errMoreThanOne) Diagnose(d *report.Diagnostic) {
	what := e.what
	if what == taxa.Unknown {
		what = taxa.Classify(e.first)
	}

	d.Apply(
		report.Message("encountered more than one %v", what),
		report.Snippetf(e.second, "help: consider removing this"),
		report.Snippetf(e.first, "first one is here"),
	)
}

const (
	justifyNone int = iota
	justifyBetween
	justifyRight
	justifyLeft
)

type justified struct {
	report.Edit
	justify int
}

func justify(stream *token.Stream, span report.Span, message string, edits ...justified) report.DiagnosticOption {
	text := span.File.Text()
	for i := range edits {
		e := &edits[i]
		// Convert the edits to absolute offsets.
		e.Start += span.Start
		e.End += span.Start
		empty := e.Start == e.End

		spaceAfter := func(text string, idx int) int {
			r, ok := stringsx.Rune(text, idx)
			if !ok || !unicode.IsSpace(r) {
				return 0
			}
			return utf8.RuneLen(r)
		}
		spaceBefore := func(text string, idx int) int {
			r, ok := stringsx.PrevRune(text, idx)
			if !ok || !unicode.IsSpace(r) {
				return 0
			}
			return utf8.RuneLen(r)
		}

		switch edits[i].justify {
		case justifyBetween:
			// If possible, shift the offset such that it is surrounded by
			// whitespace. However, this is not always possible, in which case we
			// must add whitespace to text.
			prev := spaceBefore(text, e.Start)
			next := spaceAfter(text, e.End)
			switch {
			case prev > 0 && next > 0:
				// Nothing to do here.

			case empty && prev > 0 && spaceBefore(text, e.Start-prev) > 0:
				e.Start -= prev
				e.End -= prev
			case prev > 0:
				e.Replace += " "

			case empty && next > 0 && spaceAfter(text, e.End+next) > 0:
				e.Start += next
				e.End += next
			case next > 0:
				e.Replace = " " + e.Replace

			default:
				// We're crammed between non-whitespace.
				e.Replace = " " + e.Replace + " "
			}

		case justifyLeft:
			// Get the token at the start of the span.
			start, _ := stream.Around(e.Start)
			c := token.NewCursorAt(start)
			// Seek to the previous unskippable token, and use its end as
			// the start of the justification.
			e.Start = c.Prev().Span().End
			if empty {
				e.End = e.Start
			}

		case justifyRight:
			// Identical to the above, but reversed.
			_, end := stream.Around(e.Start)
			c := token.NewCursorAt(end)
			e.End = c.Next().Span().Start
			if empty {
				e.Start = e.End
			}
		}
	}

	span = report.JoinSeq(iterx.Map(slicesx.Values(edits), func(j justified) report.Span {
		return span.File.Span(j.Start, j.End)
	}))
	return report.SuggestEdits(
		span, message,
		slicesx.Collect(iterx.Map(slicesx.Values(edits),
			func(j justified) report.Edit {
				j.Start -= span.Start
				j.End -= span.Start
				return j.Edit
			}))...,
	)
}
