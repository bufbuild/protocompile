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

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
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
	insertJustify report.Justify
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
		d.Apply(report.SuggestEdits(
			what,
			fmt.Sprintf("consider inserting a %v", want),
			report.Edit{
				Replace: e.insert,
				Justify: e.insertJustify,
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
