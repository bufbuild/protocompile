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
	"strings"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// errInvalidNumber diagnoses a numeric literal with invalid syntax.
type errInvalidNumber struct {
	Token token.Token // The offending number token.
}

// Diagnose implements [report.Diagnose].
func (e errInvalidNumber) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("unexpected characters in %s literal", intOrFloat(e.Token)),
		report.Snippet(e.Token),
	)

	// TODO: This is a pretty terrible diagnostic. We should at least add a note
	// specifying the correct syntax. For example, there should be a way to tell
	// that the invalid character is an out-of-range digit.
}

// errInvalidBase diagnoses a numeric literal that uses a popular base that
// Protobuf does not support.
type errInvalidBase struct {
	Token token.Token
	Base  int
}

// Diagnose implements [report.Diagnose].
func (e errInvalidBase) Diagnose(d *report.Diagnostic) {
	d.Apply(report.Message("unsupported base for %s literal", intOrFloat(e.Token)))

	var base string
	switch e.Base {
	case 2:
		base = "binary"
	case 8:
		base = "octal"
	case 16:
		base = "hexadecimal"
	default:
		base = fmt.Sprintf("base-%d", e.Base)
	}

	isFloat := isFloatLiteral(e.Token)
	if !isFloat && e.Base == 8 {
		d.Apply(
			report.Snippet(e.Token, "replace `0o` with `0`"),
			report.Note("Protobuf does not support the `0o` prefix for octal literals"),
		)
		return
	}

	kind := "integer"
	if isFloat {
		kind = "floating-point"
	}

	d.Apply(
		report.Snippet(e.Token),
		report.Note("Protobuf does not support %s %s literals", base, kind),
	)
}

// errThousandsSep diagnoses a numeric literal that contains Go/Java/Rust-style
// thousands separators, e.g. 1_000.
//
// Protobuf does not support such separators, but we lex them anyways with a
// diagnostic.
type errThousandsSep struct {
	Token token.Token // The offending number token.
}

// Diagnose implements [report.Diagnose].
func (e errThousandsSep) Diagnose(d *report.Diagnostic) {
	d.Apply(
		report.Message("%s literal contains underscores", intOrFloat(e.Token)),
		report.Snippet(e.Token),
		report.Note("Protobuf does not support Go/Java/Rust-style thousands separators"),
	)
}

func intOrFloat(tok token.Token) string {
	switch {
	case isFloatLiteral(tok):
		return "floating-point"
	default:
		return "integer"
	}
}

func isFloatLiteral(tok token.Token) bool {
	digits := tok.Text()
	if strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X") {
		return strings.ContainsRune(digits, '.')
	}
	return strings.ContainsAny(digits, ".eE")
}
