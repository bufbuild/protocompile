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
	"strconv"
	"strings"
	"unicode/utf8"

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
	d.With(
		report.Message("unterminated string literal"),
		report.Snippet(e.Token, "expected to be terminated by `%s`", open),
	)

	quoted := e.Token.Text()
	quote := quoted[:1]
	if len(quoted) == 1 {
		d.With(report.Note("this string consists of a single orphaned quote"))
	} else if strings.HasSuffix(quoted, quote) {
		d.With(report.Note("this string appears to end in an escaped quote; replace `\\%s` with `\\\\%[1]s%[1]s`", quote))
	}

	// TODO: check to see if a " or ' escape exists in the string?
}

// errInvalidEscape diagnoses an invalid escape sequence within a string
// literal.
type errInvalidEscape struct {
	Span report.Span // The span of the offending escape within a literal.
}

// Diagnose implements [report.Diagnose].
func (e errInvalidEscape) Diagnose(d *report.Diagnostic) {
	d.With(report.Message("invalid escape sequence"))

	text := e.Span.Text()

	if len(text) < 2 {
		d.With(report.Snippet(e.Span))
	}

	switch c := text[1]; c {
	case 'x', 'X':
		if len(text) < 3 {
			d.With(report.Snippet(e.Span, "`\\%c` must be followed by at least one hex digit", c))
			return
		}
		return
	case 'u', 'U':
		expected := 4
		if c == 'U' {
			expected = 8
		}

		if len(text[2:]) != expected {
			d.With(report.Snippet(e.Span, "`\\%c` must be followed by exactly %d hex digits", c, expected))
			return
		}

		value, _ := strconv.ParseUint(text[2:], 16, 32)
		if !utf8.ValidRune(rune(value)) {
			d.With(report.Snippet(e.Span, "must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF"))
			return
		}
		return
	}

	d.With(report.Snippet(e.Span))
}
