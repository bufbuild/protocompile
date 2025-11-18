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

// Package errtoken contains standard diagnostics involving tokens, usually emitted
// by the lexer.
package errtoken

import (
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ImpureString diagnoses a string literal that probably should not contain
// escapes or concatenation.
type ImpureString struct {
	Token token.Token
	Where taxa.Place
}

// Diagnose implements [report.Diagnose].
func (e ImpureString) Diagnose(d *report.Diagnostic) {
	text := e.Token.AsString().Text()
	quote := e.Token.Text()[0]
	d.Apply(
		report.Message("non-canonical string literal %s", e.Where.String()),
		report.Snippet(e.Token),
		report.SuggestEdits(e.Token, "replace it with a canonical string", report.Edit{
			Start: 0, End: e.Token.Span().Len(),
			Replace: fmt.Sprintf("%c%v%c", quote, text, quote),
		}),
	)

	if !e.Token.IsLeaf() {
		d.Apply(
			report.Notef(
				"Protobuf implicitly concatenates adjacent %ss, like C or Python; this can lead to surprising behavior",
				taxa.String,
			),
		)
	}
}

// InvalidEscape diagnoses an invalid escape sequence within a string
// literal.
type InvalidEscape struct {
	Span source.Span // The span of the offending escape within a literal.
}

// Diagnose implements [report.Diagnose].
func (e InvalidEscape) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("invalid escape sequence"))

	text := e.Span.Text()

	if len(text) < 2 {
		d.Apply(report.Snippet(e.Span))
	}

	switch c := text[1]; c {
	case 'x', 'X':
		if len(text) < 3 {
			d.Apply(report.Snippetf(e.Span, "`\\%c` must be followed by at least one hex digit", c))
			return
		}
		return
	case 'u', 'U':
		expected := 4
		if c == 'U' {
			expected = 8
		}

		if len(text[2:]) != expected {
			d.Apply(report.Snippetf(e.Span, "`\\%c` must be followed by exactly %d hex digits", c, expected))
			return
		}

		value, _ := strconv.ParseUint(text[2:], 16, 32)
		if !utf8.ValidRune(rune(value)) {
			d.Apply(report.Snippetf(e.Span, "must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF"))
			return
		}
		return
	}

	d.Apply(report.Snippet(e.Span))
}
