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
	"github.com/bufbuild/protocompile/experimental/token"
)

// errUnmatched diagnoses a delimiter for which we found one half of a matched
// delimiter but not the other.
type errUnmatched struct {
	Span report.Span // The offending delimiter.

	// If present, this indicates that we did match with another brace delimiter, but it
	// was of the wrong kind
	Mismatch report.Span

	// If present, this is a brace delimiter we think we *should* have matched.
	ShouldMatch report.Span
}

// openClose returns the expected open/close delimiters for this matched pair.
func (e errUnmatched) openClose() (string, string) {
	a, b := bracePair(e.Span.Text())
	if a == "" {
		panic(fmt.Sprintf(
			"protocompile/ast: invalid token in %T: %q (byte offset %d:%d)",
			e, e.Span.Text(), e.Span.Start, e.Span.End,
		))
	}
	return a, b
}

// Diagnose implements [report.Diagnose].
func (e errUnmatched) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("encountered unmatched `%s` delimiter", e.Span.Text()))

	text := e.Span.Text()
	openTok, closeTok := e.openClose()

	if text == openTok {
		d.Apply(report.Snippetf(e.Span, "expected a closing `%s`", closeTok))
		if !e.Mismatch.IsZero() {
			d.Apply(report.Snippetf(e.Mismatch, "closed by this instead"))
		}
		if !e.ShouldMatch.IsZero() {
			d.Apply(report.Snippetf(e.ShouldMatch, "help: perhaps it was meant to match this?"))
		}
	} else {
		d.Apply(report.Snippetf(e.Span, "expected a closing `%s`", openTok))
	}
	if text == "*/" {
		d.Apply(report.Notef("Protobuf does not support nested block comments"))
	}
}

// errImpureString diagnoses a string literal that probably should not contain
// escapes or concatenation.
type errImpureString struct {
	lit   token.Token
	where taxa.Place
}

// Diagnose implements [report.Diagnose].
func (e errImpureString) Diagnose(d *report.Diagnostic) {
	text, _ := e.lit.AsString()
	quote := e.lit.Text()[0]
	d.Apply(
		report.Message("non-canonical string literal %s", e.where.String()),
		report.Snippet(e.lit),
		report.SuggestEdits(e.lit, "replace it with a canonical string", report.Edit{
			Start: 0, End: e.lit.Span().Len(),
			Replace: fmt.Sprintf("%c%v%c", quote, text, quote),
		}),
	)

	if !e.lit.IsLeaf() {
		d.Apply(
			report.Notef(
				"Protobuf implicitly concatenates adjacent %ss, like C or Python; this can lead to surprising behavior",
				taxa.String,
			),
		)
	}
}
