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

// package just adds a "justification" helper for diagnostics.
//
// This package is currently internal because the API is a bit too messy to
// expose in report.
package just

import (
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// Kind is a kind of justification implemented by this package.
//
// See [Justify] for details.
type Kind int

const (
	None Kind = iota
	Between
	Left
	Right
)

// Edit is a [report.Edit] with attached justification information, which
// can be passed to [Justify].
type Edit struct {
	report.Edit
	Kind Kind
}

// Justify generates suggested edits using justification information.
//
// "Justification" is a token-aware operation that ensures that each suggested
// edit is either:
//
// 1. Is [Between] spaces: surrounded on both sides by at least once space.
// 2. Has no whitespace to its [Left] or its [Right].
//
// See the comments on doJustify* for details on the different cases this
// function handles.
func Justify(stream *token.Stream, span source.Span, message string, edits ...Edit) report.DiagnosticOption {
	for i := range edits {
		switch edits[i].Kind {
		case Between:
			between(span, &edits[i].Edit)
		case Left:
			left(stream, span, &edits[i].Edit)
		case Right:
			right(stream, span, &edits[i].Edit)
		}
	}

	return report.SuggestEditsWithWidening(span, message,
		slices.Collect(slicesx.Map(edits, func(e Edit) report.Edit { return e.Edit }))...)
}

// between performs "between" justification.
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
func between(span source.Span, e *report.Edit) {
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

// left performs left justification.
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
func left(stream *token.Stream, span source.Span, e *report.Edit) {
	wasDelete := e.IsDeletion()

	// Get the token at the start of the span.
	start, _ := stream.Around(e.Start + span.Start)
	if start.IsZero() {
		// Start of the file, so we can't rewind beyond this.
		return
	}

	if start.Kind().IsSkippable() {
		// Seek to the previous unskippable token, and use its end as
		// the start of the justification.
		start = token.NewCursorAt(start).Prev()
	}

	e.Start = start.Span().End - span.Start
	if !wasDelete {
		e.End = e.Start
	}
}

// right is the mirror image of doJustifyLeft.
func right(stream *token.Stream, span source.Span, e *report.Edit) {
	wasDelete := e.IsDeletion()

	// Get the token at the end of the span.
	_, end := stream.Around(e.End + span.Start)
	if end.IsZero() {
		// End of the file, so we can't fast-forward beyond this.
		return
	}

	if end.Kind().IsSkippable() {
		// Seek to the next unskippable token, and use its start as
		// the start of the justification.
		end = token.NewCursorAt(end).Next()
	}

	e.End = end.Span().Start - span.Start
	if !wasDelete {
		e.Start = e.End
	}
}
