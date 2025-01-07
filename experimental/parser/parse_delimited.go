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
	"github.com/bufbuild/protocompile/experimental/token"
)

// delimited is a mechanism for parsing a punctuation-delimited list.
type delimited[T any] struct {
	p *parser
	c *token.Cursor

	// What are we parsing, and within what context? This is used for
	// generating diagnostics.
	what, in taxa.Noun

	// Permitted delimiters. If empty, assumed to be []string{","}.
	delims []string
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
}

func (d delimited[T]) appendTo(commas ast.Commas[T]) {
	d.iter(func(v T, delim token.Token) bool {
		commas.AppendComma(v, delim)
		return true
	})
}

func (d delimited[T]) iter(yield func(value T, delim token.Token) bool) {
	// NOTE: We do not use errUnexpected here, because we want to insert the
	// terms "leading", "extra", and "trailing" where appropriate, and because
	// we don't want to have to deal with asking the caller to provide Nouns
	// for each delimiter.
	if len(d.delims) == 0 {
		d.delims = []string{","}
	}

	var delim token.Token

	if next := d.c.Peek(); slices.Contains(d.delims, next.Text()) {
		_ = d.c.Pop()

		d.p.Error(errUnexpected{
			what:  next,
			where: d.in.In(),
			want:  d.what.AsSet(),
			got:   fmt.Sprintf("leading `%s`", next.Text()),
		})
	}

	for !d.c.Done() {
		v, ok := d.parse(d.c)
		if !ok {
			break
		}

		// Pop as many delimiters as we can.
		delim = token.Zero
		for slices.Contains(d.delims, d.c.Peek().Text()) {
			next := d.c.Pop()
			if delim.IsZero() {
				delim = next
				continue
			}

			d.p.Error(errUnexpected{
				what:  next,
				where: d.in.In(),
				want:  d.what.AsSet(),
				got:   fmt.Sprintf("extra `%s`", next.Text()),
			}).Apply(report.Snippetf(delim, "first delimiter is here"))
		}

		if !yield(v, delim) || (d.required && delim.IsZero()) {
			break
		}
	}

	switch {
	case d.exhaust && !d.c.Done():
		d.p.Error(errUnexpected{
			what:  report.JoinSeq(d.c.Rest()),
			where: d.in.In(),
			want:  d.what.AsSet(),
			got:   "tokens",
		})
	case !d.trailing && !delim.IsZero():
		d.p.Error(errUnexpected{
			what:  delim,
			where: d.in.In(),
			got:   fmt.Sprintf("trailing `%s`", delim.Text()),
		})
	}
}
