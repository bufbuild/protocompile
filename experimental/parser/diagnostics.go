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

package ast

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// MaxFileSize is the maximum file size Protocompile supports.
const MaxFileSize int = math.MaxInt32 // 2GB

// ErrFileTooBig diagnoses a file that is beyond Protocompile's implementation limits.
type ErrFileTooBig struct {
	Path string
}

func (e ErrFileTooBig) Error() string {
	return "files larger than 2GB are not supported"
}

func (e ErrFileTooBig) Diagnose(d *report.Diagnostic) {
	d.With(report.InFile(e.Path))
}

// ErrNotUTF8 diagnoses a file that contains non-UTF-8 bytes.
type ErrNotUTF8 struct {
	Path string
	At   int
	Byte byte
}

func (e ErrNotUTF8) Error() string {
	return "files must be encoded as valid UTF-8"
}

func (e ErrNotUTF8) Diagnose(d *report.Diagnostic) {
	d.With(
		report.InFile(e.Path),
		report.Notef("found a 0x%02x byte at offset %d", e.Byte, e.At),
	)
}

// ErrUnrecognized diagnoses the presence of an unrecognized token.
type ErrUnrecognized struct{ Token token.Token }

func (e ErrUnrecognized) Error() string {
	return "unrecongnized token"
}

func (e ErrUnrecognized) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))
}

// ErrUnterminated diagnoses a delimiter for which we found one half of a matched
// delimiter but not the other.
type ErrUnterminated struct {
	Span report.Span

	// If present, this indicates that we did match with another brace delimiter, but it
	// was of the wrong kind
	Mismatch report.Span
}

// OpenClose returns the expected open/close delimiters for this matched pair.
func (e ErrUnterminated) OpenClose() (string, string) {
	switch t := e.Span.Text(); t {
	case "(", ")":
		return "(", ")"
	case "[", "]":
		return "[", "]"
	case "{", "}":
		return "{", "}"
	case "<", ">":
		return "<", ">"
	case "/*", "*/":
		return "/*", "*/"
	default:
		panic(fmt.Sprintf("protocompile/ast: invalid token in ErrUnterminated: %q (byte offset %d:%d)", t, e.Span.Start, e.Span.End))
	}
}

func (e ErrUnterminated) Error() string {
	return fmt.Sprintf("encountered unterminated `%s` delimiter", e.Span.Text())
}

func (e ErrUnterminated) Diagnose(d *report.Diagnostic) {
	text := e.Span.Text()
	openTok, closeTok := e.OpenClose()

	if text == openTok {
		d.With(report.Snippetf(e.Span, "expected to be closed by `%s", closeTok))
		if e.Mismatch.IndexedFile != nil {
			d.With(report.Snippetf(e.Mismatch, "closed by this instead"))
		}
	} else {
		d.With(report.Snippetf(e.Span, "expected to be opened by `%s", openTok))
	}
	if text == "*/" {
		d.With(report.Note("Protobuf does not support nested block comments"))
	}
}

// ErrUnterminatedStringLiteral diagnoses a string literal that continues to EOF.
type ErrUnterminatedStringLiteral struct{ Token token.Token }

func (e ErrUnterminatedStringLiteral) Error() string {
	return "unterminated string literal"
}

func (e ErrUnterminatedStringLiteral) Diagnose(d *report.Diagnostic) {
	open := e.Token.Text()[:1]
	d.With(report.Snippetf(e.Token, "expected to be terminated by `%s`", open))
	// TODO: check to see if a " or ' escape exists in the string?
}

// ErrInvalidEscape diagnoses an invalid escape sequence.
type ErrInvalidEscape struct {
	Span report.Span
}

func (e ErrInvalidEscape) Error() string {
	return "invalid escape sequence"
}

func (e ErrInvalidEscape) Diagnose(d *report.Diagnostic) {
	text := e.Span.Text()

	if len(text) >= 2 {
		switch c := text[1]; c {
		case 'x':
			if len(text) < 3 {
				d.With(report.Snippetf(e.Span, "`\\x` must be followed by at least one hex digit"))
				return
			}
		case 'u', 'U':
			expected := 4
			if c == 'U' {
				expected = 8
			}

			if len(text[2:]) != expected {
				d.With(report.Snippetf(e.Span, "`\\%c` must be followed by exactly %d hex digits", c, expected))
				return
			}

			value, _ := strconv.ParseUint(text[2:], 16, 32)
			if !utf8.ValidRune(rune(value)) {
				d.With(report.Snippetf(e.Span, "must be in the range U+0000 to U+10FFFF, except U+DC00 to U+DFFF"))
				return
			}
		}
	}

	d.With(report.Snippet(e.Span))
}

// ErrNonASCIIIdent diagnoses an identifier that contains non-ASCII runes.
type ErrNonASCIIIdent struct{ Token token.Token }

func (e ErrNonASCIIIdent) Error() string {
	return "non-ASCII identifiers are not allowed"
}

func (e ErrNonASCIIIdent) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))
}

// ErrIntegerOverflow diagnoses an integer literal that does not fit into the required range.
type ErrIntegerOverflow struct {
	Token token.Token

	// TODO: Extend this to other bit-sizes. This currently assumes a 64-bit value.
}

func (e ErrIntegerOverflow) Error() string {
	return "integer literal out of range"
}

func (e ErrIntegerOverflow) Diagnose(d *report.Diagnostic) {
	text := strings.ToLower(e.Token.Text())
	switch {
	case strings.HasPrefix(text, "0x"):
		d.With(
			report.Snippetf(e.Token, "must be in the range `0x0` to `0x%x`", uint64(math.MaxUint64)),
			report.Note("hexadecimal literals must always fit in a uint64"),
		)
	case strings.HasPrefix(text, "0b"):
		d.With(
			report.Snippetf(e.Token, "must be in the range `0b0` to `0b%b`", uint64(math.MaxUint64)),
			report.Note("binary literals must always fit in a uint64"),
		)
	default:
		// NB: Decimal literals cannot overflow, at least in the lexer.
		d.With(
			report.Snippetf(e.Token, "must be in the range `0` to `0%o`", uint64(math.MaxUint64)),
			report.Note("octal literals must always fit in a uint64"),
		)
	}
}

// ErrInvalidNumber diagnoses a numeric literal with invalid syntax.
type ErrInvalidNumber struct {
	Token token.Token
}

func (e ErrInvalidNumber) Error() string {
	if strings.ContainsRune(e.Token.Text(), '.') {
		return "invalid floating-point literal"
	}

	return "invalid integer literal"
}

func (e ErrInvalidNumber) Diagnose(d *report.Diagnostic) {
	text := strings.ToLower(e.Token.Text())
	d.With(report.Snippet(e.Token))
	if strings.ContainsRune(e.Token.Text(), '.') && strings.HasPrefix(text, "0x") {
		d.With(report.Note("Protobuf does not support binary floats"))
	}
}
