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

// ErrUnrecognized diagnoses the presence of an unrecognized token.
type ErrUnrecognized struct {
	Token token.Token // The offending token.
}

// Error implements [error].
func (e ErrUnrecognized) Error() string {
	return "unrecognized token"
}

// Diagnose implements [report.Diagnose].
func (e ErrUnrecognized) Diagnose(d *report.Diagnostic) {
	d.With(
		report.Snippet(e.Token),
		report.Debugf("%v, %v, %q", e.Token.ID(), e.Token.Span(), e.Token.Text()),
	)
}

// ErrNonASCIIIdent diagnoses an identifier that contains non-ASCII runes.
type ErrNonASCIIIdent struct {
	Token token.Token // The offending identifier token.
}

// Error implements [error].
func (e ErrNonASCIIIdent) Error() string {
	return "non-ASCII identifiers are not allowed"
}

// Diagnose implements [report.Diagnose].
func (e ErrNonASCIIIdent) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))
}

// ErrUnmatched diagnoses a delimiter for which we found one half of a matched
// delimiter but not the other.
type ErrUnmatched struct {
	Span report.Span // The offending delimiter.

	// If present, this indicates that we did match with another brace delimiter, but it
	// was of the wrong kind
	Mismatch report.Span

	// If present, this is a brace delimiter we think we *should* have matched.
	ShouldMatch report.Span
}

// OpenClose returns the expected open/close delimiters for this matched pair.
func (e ErrUnmatched) OpenClose() (string, string) {
	a, b := bracePair(e.Span.Text())
	if a == "" {
		panic(fmt.Sprintf("protocompile/ast: invalid token in ErrUnterminated: %q (byte offset %d:%d)", e.Span.Text(), e.Span.Start, e.Span.End))
	}
	return a, b
}

// Error implements [error].
func (e ErrUnmatched) Error() string {
	return fmt.Sprintf("encountered unmatched `%s` delimiter", e.Span.Text())
}

// Diagnose implements [report.Diagnose].
func (e ErrUnmatched) Diagnose(d *report.Diagnostic) {
	text := e.Span.Text()
	openTok, closeTok := e.OpenClose()

	if text == openTok {
		d.With(report.Snippetf(e.Span, "expected a closing `%s`", closeTok))
		if e.Mismatch.IndexedFile != nil {
			d.With(report.Snippetf(e.Mismatch, "closed by this instead"))
		}
		if e.ShouldMatch.IndexedFile != nil {
			d.With(report.Snippetf(e.ShouldMatch, "help: perhaps it was meant to match this?"))
		}
	} else {
		d.With(report.Snippetf(e.Span, "expected a closing `%s`", openTok))
	}
	if text == "*/" {
		d.With(report.Note("Protobuf does not support nested block comments"))
	}
}
