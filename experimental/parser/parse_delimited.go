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
	"slices"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// delimited is a mechanism for parsing a punctuation-delimited list.
type delimited[T source.Spanner] struct {
	p *parser
	c *token.Cursor

	// What are we parsing, and within what context? This is used for
	// generating diagnostics.
	what, in taxa.Noun

	// Permitted delimiters. If empty, assumed to be keyword.Comma.
	delims []keyword.Keyword
	// Whether a delimiter must be present, rather than merely optional.
	required bool
	// Whether iteration should expect to exhaust c.
	exhaust bool
	// Whether trailing delimiters are permitted.
	trailing bool

	// A function for parsing elements as they come.
	//
	// This function is expected to exhaust
	parse func(*token.Cursor) (T, bool)

	// Used for skipping tokens until we can begin parsing.
	//
	// start is called until we see a token that returns true for it. However,
	// if stop is not nil and it returns true for that token, parsing stops.
	start, stop func(token.Token) bool
}

func (d delimited[T]) appendTo(commas ast.Commas[T]) {
	for v, d := range d.iter {
		commas.AppendComma(v, d)
	}
}

func (d delimited[T]) iter(yield func(value T, delim token.Token) bool) {
	// NOTE: We do not use errUnexpected here, because we want to insert the
	// terms "leading", "extra", and "trailing" where appropriate, and because
	// we don't want to have to deal with asking the caller to provide Nouns
	// for each delimiter.
	if len(d.delims) == 0 {
		d.delims = []keyword.Keyword{keyword.Comma}
	}

	var delim token.Token
	var latest int // The index of the most recently seen delimiter.

	next := d.c.Peek()
	if idx := slices.Index(d.delims, next.Keyword()); idx >= 0 {
		_ = d.c.Next()
		latest = idx

		d.p.Error(errUnexpected{
			what:  next,
			where: d.in.In(),
			want:  d.what.AsSet(),
			got:   fmt.Sprintf("leading `%s`", next.Text()),
		}).Apply(report.SuggestEdits(
			next.Span(),
			fmt.Sprintf("delete this `%s`", next.Text()),
			report.Edit{Start: 0, End: len(next.Text())},
		))
	}

	var needDelim bool
	var mark token.CursorMark
	for !d.c.Done() {
		ensureProgress(d.c, &mark)

		// Set if we should not diagnose a missing comma, because there was
		// garbage in front of the call to parse().
		var badPrefix bool
		if !d.start(d.c.Peek()) {
			if d.stop != nil && d.stop(d.c.Peek()) {
				break
			}

			first := d.c.Next()
			var last token.Token
			for !d.c.Done() && !d.start(d.c.Peek()) {
				if d.stop != nil && d.stop(d.c.Peek()) {
					break
				}
				last = d.c.Next()
			}

			want := d.what.AsSet()
			if needDelim && delim.IsZero() {
				want = d.delimNouns()
			}

			what := source.Spanner(first)
			if !last.IsZero() {
				what = source.Join(first, last)
			}

			badPrefix = true
			d.p.Error(errUnexpected{
				what:  what,
				where: d.in.In(),
				want:  want,
			})
		}

		v, ok := d.parse(d.c)
		if !ok {
			break
		}

		if !badPrefix && needDelim && delim.IsZero() {
			d.p.Error(errUnexpected{
				what:  v,
				where: d.in.In(),
				want:  d.delimNouns(),
			}).Apply(
				report.Snippetf(v.Span().Rune(0), "note: assuming a missing `%s` here", d.delims[latest]),
				justify(
					d.p.File().Stream(),
					v.Span(),
					fmt.Sprintf("add a `%s` here", d.delims[latest]),
					justified{
						report.Edit{Replace: d.delims[latest].String()},
						justifyLeft,
					},
				),
			)
		}
		needDelim = d.required

		// Pop as many delimiters as we can.
		delim = token.Zero
		for {
			which := slices.Index(d.delims, d.c.Peek().Keyword())
			if which < 0 {
				break
			}
			latest = which

			next := d.c.Next()
			if delim.IsZero() {
				delim = next
				continue
			}

			// Diagnose all extra delimiters after the first.
			d.p.Error(errUnexpected{
				what:  next,
				where: d.in.In(),
				want:  d.what.AsSet(),
				got:   fmt.Sprintf("extra `%s`", next.Text()),
			}).Apply(
				report.Snippetf(delim, "first delimiter is here"),
				report.SuggestEdits(
					next.Span(),
					fmt.Sprintf("delete this `%s`", next.Text()),
					report.Edit{Start: 0, End: len(next.Text())},
				),
			)
		}

		if !yield(v, delim) {
			break
		}

		// In non-exhaust mode, if we miss a required comma, bail if we have
		// reached a stop token, or if we don't have a stop predicate.
		// Otherwise, go again to parse another thing.
		if delim.IsZero() && d.required && !d.exhaust {
			if d.stop == nil || d.stop(d.c.Peek()) {
				break
			}
		}
	}

	switch {
	case d.exhaust && !d.c.Done():
		d.p.Error(errUnexpected{
			what:  source.JoinSeq(d.c.Rest()),
			where: d.in.In(),
			want:  d.what.AsSet(),
			got:   "tokens",
		})
	case !d.trailing && !delim.IsZero():
		d.p.Error(errUnexpected{
			what:  delim,
			where: d.in.In(),
			got:   fmt.Sprintf("trailing `%s`", delim.Text()),
		}).Apply(report.SuggestEdits(
			delim.Span(),
			fmt.Sprintf("delete this `%s`", delim.Text()),
			report.Edit{Start: 0, End: len(delim.Text())},
		))
	}
}

func (d delimited[T]) delimNouns() taxa.Set {
	var set taxa.Set
	for _, delim := range d.delims {
		set = set.With(taxa.Keyword(delim))
	}
	return set
}
