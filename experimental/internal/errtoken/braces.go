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
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// Unmatched diagnoses a delimiter for which we found one half of a matched
// delimiter but not the other.
type Unmatched struct {
	Span    source.Span // The offending delimiter.
	Keyword keyword.Keyword

	// If present, this indicates that we did match with another brace delimiter, but it
	// was of the wrong kind
	Mismatch source.Span

	// If present, this is a brace delimiter we think we *should* have matched.
	ShouldMatch source.Span
}

// Diagnose implements [report.Diagnose].
func (e Unmatched) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("encountered unmatched `%s` delimiter", e.Span.Text()))

	left, right, _ := e.Keyword.Brackets()

	if e.Keyword == left {
		d.Apply(report.Snippetf(e.Span, "expected a closing `%s`", right))
		if !e.Mismatch.IsZero() {
			d.Apply(report.Snippetf(e.Mismatch, "closed by this instead"))
		}
		if !e.ShouldMatch.IsZero() {
			d.Apply(report.Snippetf(e.ShouldMatch, "help: perhaps it was meant to match this?"))
		}
	} else {
		d.Apply(report.Snippetf(e.Span, "expected a closing `%s`", left))
	}
}
