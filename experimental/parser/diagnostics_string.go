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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// errUnclosedString diagnoses a string literal that continues to EOF.
type errUnclosedString struct {
	Token token.Token // The offending string literal token.
}

// Diagnose implements [report.Diagnose].
func (e errUnclosedString) Diagnose(d *report.Diagnostic) {
	open := e.Token.Text()[:1]
	d.Apply(
		report.Message("unterminated string literal"),
		report.Snippetf(e.Token, "expected to be terminated by `%s`", open),
	)

	quoted := e.Token.Text()
	quote := quoted[:1]
	if len(quoted) == 1 {
		d.Apply(report.Notef("this string consists of a single orphaned quote"))
	} else if strings.HasSuffix(quoted, quote) {
		d.Apply(report.SuggestEdits(
			e.Token,
			"this string appears to end in an escaped quote",
			report.Edit{
				Start: e.Token.Span().Len() - 2, End: e.Token.Span().Len(),
				Replace: fmt.Sprintf(`\\%v%v`, quote, quote),
			},
		))
	}
}

// errInvalidEscape diagnoses an invalid escape sequence within a string
// literal.
type errInvalidEscape struct {
	Span report.Span // The span of the offending escape within a literal.
}

// Diagnose implements [report.Diagnose].
func (e errInvalidEscape) Diagnose(d *report.Diagnostic) {
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
