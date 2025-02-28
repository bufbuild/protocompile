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

// errUnexpectedEOF is a helper for constructing EOF diagnostics that need to
// provide *no* suggestions. This is used in places where any suggestion we
// could provide would be nonsensical.
func errUnexpectedEOF(c *token.Cursor, where taxa.Place) errUnexpected {
	tok, span := c.Clone().SeekToEnd()
	if tok.IsZero() {
		return errUnexpected{
			what:  span,
			where: where,
			got:   taxa.EOF,
		}
	}
	return errUnexpected{what: tok, where: where}
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
	justifyLeft
	justifyRight
)

type justified struct {
	report.Edit
	justify int
}

// justify generates suggested edits using justification information.
//
// "Justification" is a token-aware operation that ensures that each suggested
// edit is either:
//
// 1. Surrounded on both sides by at least once space. (justifyBetween)
// 2. Has no whitespace to its left or its right. (justifyLeft, justifyRight)
//
// See the comments on doJustify* for details on the different cases this
// function handles.
func justify(stream *token.Stream, span report.Span, message string, edits ...justified) report.DiagnosticOption {
	for i := range edits {
		switch edits[i].justify {
		case justifyBetween:
			doJustifyBetween(span, &edits[i].Edit)
		case justifyLeft:
			doJustifyLeft(stream, span, &edits[i].Edit)
		case justifyRight:
			doJustifyRight(stream, span, &edits[i].Edit)
		}
	}

	return report.SuggestEditsWithWidening(span, message,
		slicesx.Collect(slicesx.Map(edits, func(j justified) report.Edit { return j.Edit }))...)
}

// doJustifyBetween performs "between" justification.
//
// In well-formatted Protobuf, an equals sign should be surrounded by spaces on
// both sides. Thus, if the user wrote [option: 5], we want to suggest
// [option = 5]. justifyBetween handles this case by inserting an extra space
// into the replacement string, so that it goes from "=" to " =". We need to
// not blindly convert it into " = ", because that would suggest [option =  5],
// which looks ugly.
//
// It also handles the case [option/*foo*/: 5] by *not* being token aware: it
// will suggest [option/*foo*/ = 5].
//
// We *also* need to handle cases like [foo  5], where we want to insert an
// sign that somehow got deleted. The suggestion will probably be placed right
// after foo, so naively it will become [foo=  5], and after justification,
// [foo =  5]. To avoid this, we have a special case where we move the insertion
// point one space over to avoid needing to insert an extra space, producing
// [foo = 5].
//
// Of course, all of these operations are performed symmetrically.
func doJustifyBetween(span report.Span, e *report.Edit) {
	text := span.File.Text()

	// Helpers which returns the number of bytes of the space before or
	// after the given offset. This byte width is used to shift the
	// replaced region when there are extra spaces around it.
	spaceAfter := func(idx int) int {
		r, ok := stringsx.Rune(text, idx+span.Start)
		if !ok || !unicode.IsSpace(r) {
			return 0
		}
		return utf8.RuneLen(r)
	}
	spaceBefore := func(idx int) int {
		r, ok := stringsx.PrevRune(text, idx+span.Start)
		if !ok || !unicode.IsSpace(r) {
			return 0
		}
		return utf8.RuneLen(r)
	}

	// If possible, shift the offset such that it is surrounded by
	// whitespace. However, this is not always possible, in which case we
	// must add whitespace to text.
	prev := spaceBefore(e.Start)
	next := spaceAfter(e.End)
	switch {
	case prev > 0 && next > 0:
		// Nothing to do here.

	case prev > 0:
		if !e.IsDeletion() && spaceBefore(e.Start-prev) > 0 {
			// Case for inserting = into [foo  5].
			e.Start -= prev
			e.End -= prev
		} else {
			// Case for replacing : -> = in [foo :5].
			e.Replace += " "
		}

	case next > 0:
		if !e.IsDeletion() && spaceAfter(e.End+next) > 0 {
			// Mirror-image case for inserting = into [foo  5].
			e.Start += next
			e.End += next
		} else {
			// Case for replacing : -> = in [foo: 5].
			e.Replace = " " + e.Replace
		}

	default:
		// Case for replacing : -> = in [foo:5].
		e.Replace = " " + e.Replace + " "
	}
}

// doJustifyLeft performs left justification.
//
// This will ensure that the suggestion is as far to the left as possible before
// any other token.
//
// For example, consider the following fragment.
//
//	int32 x
//	int32 y;
//
// We want to suggest a semicolon after x. However, the parser won't give up
// parsing followers of x until it hits int32 on the second line, by which time
// it's very hard to figure out, from the parser state, where the semicolon
// should go. So, we suggest inserting it immediately before the second int32,
// but with left justification: that will cause the suggestion to move until
// just after x on the first line.
//
// This must use token information to work correctly. Consider now
//
//	int32 x // comment
//	int32 y;
//
// If we simply chased spaces backwards, we would wind up with the following
// bad suggestion:
//
//	int32 x // comment;
//	int32 y;
//
// To avoid this, we instead rewind past any skippable tokens, which is why
// we use a stream here.
//
// This is used in some other palces, such as when converting {x = y} into
// {x: y}. In this case, because we're performing a deletion, we *consume*
// the extra space, instead of merely moving the insertion point. This case
// can result in comments getting deleted; avoiding this is probably not
// worth it. E.g. `{x/*f*/ = y}` becomes `{x: y}`, because the deleted region
// is expanded from "=" into "/*f*/ =".
func doJustifyLeft(stream *token.Stream, span report.Span, e *report.Edit) {
	wasDelete := e.IsDeletion()

	// Get the token at the start of the span.
	start, _ := stream.Around(e.Start + span.Start)
	if start.IsZero() {
		// Start of the file, so we can't rewind beyond this.
		return
	}

	// Seek to the previous unskippable token, and use its end as
	// the start of the justification.
	e.Start = token.NewCursorAt(start).Prev().Span().End - span.Start

	if !wasDelete {
		e.End = e.Start
	}
}

// doJustifyRight is the mirror image of doJustifyLeft.
func doJustifyRight(stream *token.Stream, span report.Span, e *report.Edit) {
	wasDelete := e.IsDeletion()

	// Get the token at the start of the span.
	_, end := stream.Around(e.Start + span.Start)
	if end.IsZero() {
		// End of the file, so we can't fast-forward beyond this.
		return
	}

	// Seek to the next unskippable token, and use its start as
	// the start of the justification.
	e.End = token.NewCursorAt(end).Next().Span().Start - span.Start

	if !wasDelete {
		e.Start = e.End
	}
}
