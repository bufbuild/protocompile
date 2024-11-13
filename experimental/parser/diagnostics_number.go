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
	"strings"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// ErrInvalidNumber diagnoses a numeric literal with invalid syntax.
type ErrInvalidNumber struct {
	Token token.Token // The offending number token.
}

// Error implements [error].
func (e ErrInvalidNumber) Error() string {
	switch {
	case isFloatLiteral(e.Token):
		return "unexpected characters in floating-point literal"
	default:
		return "unexpected characters in integer literal"
	}
}

// Diagnose implements [report.Diagnose].
func (e ErrInvalidNumber) Diagnose(d *report.Diagnostic) {
	d.With(report.Snippet(e.Token))

	// TODO: This is a pretty terrible diagnostic. We should at least add a note
	// specifying the correct syntax. For example, there should be a way to tell
	// that the invalid character is an out-of-range digit.
}

// ErrInvalidBase diagnoses a numeric literal that uses a popular base that
// Protobuf does not support.
type ErrInvalidBase struct {
	Token token.Token
	Base  int
}

// Error implements [error].
func (e ErrInvalidBase) Error() string {
	switch {
	case isFloatLiteral(e.Token):
		return "unsupported base for floating-point literal"
	default:
		return "unsupported base for integer literal"
	}
}

// Diagnose implements [report.Diagnose].
func (e ErrInvalidBase) Diagnose(d *report.Diagnostic) {
	base := "<unknown>"
	switch e.Base {
	case 2:
		base = "binary"
	case 8:
		base = "octal"
	case 16:
		base = "hexadecimal"
	}

	isFloat := isFloatLiteral(e.Token)
	if !isFloat && e.Base == 8 {
		d.With(
			report.Snippetf(e.Token, "replace `0o` with `0`"),
			report.Note("Protobuf does not support the `0o` prefix for octal literals"),
		)
		return
	}

	kind := "integer"
	if isFloat {
		kind = "floating-point"
	}

	d.With(
		report.Snippet(e.Token),
		report.Notef("Protobuf does not support %s %s literals", base, kind),
	)
}

// ErrThousandsSep diagnoses a numeric literal that contains Go/Java/Rust-style
// thousands separators, e.g. 1_000.
//
// Protobuf does not support such separators, but we lex them anyways with a
// diagnostic.
type ErrThousandsSep struct {
	Token token.Token // The offending number token.
}

// Error implements [error].
func (e ErrThousandsSep) Error() string {
	switch {
	case isFloatLiteral(e.Token):
		return "floating-point literal contains underscores"
	default:
		return "integer literal contains underscores"
	}
}

// Diagnose implements [report.Diagnose].
func (e ErrThousandsSep) Diagnose(d *report.Diagnostic) {
	d.With(
		report.Snippet(e.Token),
		report.Note("Protobuf does not support Go/Java/Rust-style thousands separators"),
	)
}

func isFloatLiteral(tok token.Token) bool {
	digits := tok.Text()
	if strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X") {
		return strings.ContainsRune(digits, '.')
	}
	return strings.ContainsAny(digits, ".eE")
}
