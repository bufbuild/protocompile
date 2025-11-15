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
	"strings"

	"github.com/bufbuild/protocompile/experimental/internal/taxa"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/unicodex"
)

// errInvalidNumber diagnoses a numeric literal with invalid syntax.
type InvalidNumber struct {
	Token token.Token // The offending number token.
}

// Diagnose implements [report.Diagnose].
func (e InvalidNumber) Diagnose(d *report.Diagnostic) {
	// Check for an extra decimal point in the mantissa.
	mant := e.Token.AsNumber().Mantissa()
	first := strings.Index(mant.Text(), ".")
	if first != -1 {
		second := strings.Index(mant.Text()[first+1:], ".")
		if second != -1 {
			second += first + 1
			d.Apply(
				report.Message("extra decimal point in %s", taxa.Classify(e.Token)),
				report.Snippet(mant.Range(second, second)),
				report.Snippetf(mant.Range(first, first), "first one is here"),
			)
			return
		}
	}

	// Ditto for the exponent.
	exp := e.Token.AsNumber().Exponent()
	first = strings.Index(exp.Text(), ".")
	if first != -1 {
		if first < exp.Len()-1 {
			first++
		}

		d.Apply(
			report.Message("non-integer exponent in %s", taxa.Classify(e.Token)),
			report.Snippetf(exp.Range(first, exp.Len()), "fractional part given here"),
		)
		return
	}

	// Check for bad digits.
	if e.badDigit(d, e.Token.AsNumber().Mantissa()) {
		return
	}
	if e.badDigit(d, e.Token.AsNumber().Exponent()) {
		return
	}

	// Fallback for when we don't have anything useful to say.
	d.Apply(
		report.Message("unexpected characters in %s", taxa.Classify(e.Token)),
		report.Snippet(e.Token),
	)
}

func (e InvalidNumber) badDigit(d *report.Diagnostic, digits source.Span) bool {
	if digits.IsZero() {
		return false
	}

	base := e.Token.AsNumber().Base()
	for i, r := range digits.Text() {
		if strings.ContainsRune("_-+.", r) {
			continue
		}
		if _, ok := unicodex.Digit(r, base); ok {
			continue
		}

		d.Apply(
			report.Message("invalid digit in %s %s", e.baseName(), taxa.Classify(e.Token)),
			report.Snippetf(digits.Range(i, i), "expected %s", e.digits()),
			report.Snippetf(e.Token.AsNumber().Prefix(), "implies %s", e.baseName()),
		)

		if e.Token.AsNumber().IsLegacyOctal() {
			d.Apply(report.Helpf("a leading `0` digit causes the whole literal to be interpreted as octal"))
		}
		return true
	}

	return false
}

func (e InvalidNumber) baseName() string {
	switch e.Token.AsNumber().Base() {
	case 2:
		return "binary"
	case 8:
		return "octal"
	case 10:
		return "decimal"
	case 16:
		return "hexadecimal"
	default:
		return ""
	}
}

func (e InvalidNumber) digits() string {
	switch e.Token.AsNumber().Base() {
	case 2:
		return "`0` or `1`"
	case 8:
		return "`0` to `7`"
	case 10:
		return "`0` to `9`"
	case 16:
		return "`0` to `9`, or `a` to `f`"
	default:
		return ""
	}
}
