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

package errtoken

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/internal/just"
	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
)

// Unexpected is a low-level parser error for when we hit a token we don't
// know how to handle.
type Unexpected struct {
	// The unexpected thing (may be a token or AST node).
	What source.Spanner

	// The context we're in. Should be format-able with %v.
	Where taxa.Place
	// Useful when where is an "after" position: if non-nil, this will be
	// highlighted as "previous where.Object is here"
	Prev source.Spanner

	// What we wanted vs. what we got. Got can be used to customize what gets
	// shown, but if it's not set, we call describe(what) to get a user-visible
	// description.
	Want taxa.Set
	// If set and want is empty, the snippet will repeat the "unexpected foo"
	// text under the snippet.
	RepeatUnexpected bool
	Got              any

	// If nonempty, inserting this text will be suggested at the given offset.
	Insert        string
	InsertAt      int
	InsertJustify just.Kind
	Stream        *token.Stream
}

// UnexpectedEOF is a helper for constructing EOF diagnostics that need to
// provide *no* suggestions. This is used in places where any suggestion we
// could provide would be nonsensical.
func UnexpectedEOF(c *token.Cursor, where taxa.Place) Unexpected {
	tok, span := c.Clone().SeekToEnd()
	if tok.IsZero() {
		return Unexpected{
			What:  span,
			Where: where,
			Got:   taxa.EOF,
		}
	}
	return Unexpected{What: tok, Where: where}
}

func (e Unexpected) Diagnose(d *report.Diagnostic) {
	got := e.Got
	if got == nil {
		got = taxa.Classify(e.What)
		if got == taxa.Unknown {
			got = "tokens"
		}
	}

	var message string
	if e.Where.Subject() == taxa.Unknown {
		message = fmt.Sprintf("unexpected %v", got)
	} else {
		message = fmt.Sprintf("unexpected %v %v", got, e.Where)
	}

	what := e.What.Span()
	snippet := report.Snippet(what)
	if e.Want.Len() > 0 {
		snippet = report.Snippetf(what, "expected %v", e.Want.Join("or"))
	} else if e.RepeatUnexpected {
		snippet = report.Snippetf(what, "%v", message)
	}

	d.Apply(
		report.Message("%v", message),
		snippet,
		report.Snippetf(e.Prev, "previous %v is here", e.Where.Subject()),
	)

	if tok, ok := e.What.(token.Token); ok {
		d.Apply(
			report.Debugf("token: %v, kind: %#v, keyword: %#v", tok.ID(), tok.Kind(), tok.Keyword()),
		)
	}

	if e.Insert != "" {
		want, _ := iterx.First(e.Want.All())
		d.Apply(just.Justify(
			e.Stream,
			what,
			fmt.Sprintf("consider inserting a %v", want),
			just.Edit{
				Edit: report.Edit{Replace: e.Insert},
				Kind: e.InsertJustify,
			},
		))
	}
}
