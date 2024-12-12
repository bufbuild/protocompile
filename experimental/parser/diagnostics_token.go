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

package parser

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// errUnrecognized diagnoses the presence of an unrecognized token.
type errUnrecognized struct {
	Token token.Token // The offending token.
}

// Diagnose implements [report.Diagnose].
func (e errUnrecognized) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("unrecognized token"),
		report.Snippet(e.Token),
		report.Debug("%v, %v, %q", e.Token.ID(), e.Token.Span(), e.Token.Text()),
	)
}

// errNonASCIIIdent diagnoses an identifier that contains non-ASCII runes.
type errNonASCIIIdent struct {
	Token token.Token // The offending identifier token.
}

// Diagnose implements [report.Diagnose].
func (e errNonASCIIIdent) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("non-ASCII identifiers are not allowed"),
		report.Snippet(e.Token),
	)
}

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
		d.Apply(report.Snippet(e.Span, "expected a closing `%s`", closeTok))
		if !e.Mismatch.Nil() {
			d.Apply(report.Snippet(e.Mismatch, "closed by this instead"))
		}
		if !e.ShouldMatch.Nil() {
			d.Apply(report.Snippet(e.ShouldMatch, "help: perhaps it was meant to match this?"))
		}
	} else {
		d.Apply(report.Snippet(e.Span, "expected a closing `%s`", openTok))
	}
	if text == "*/" {
		d.Apply(report.Note("Protobuf does not support nested block comments"))
	}
}
