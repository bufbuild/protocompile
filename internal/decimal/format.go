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

package decimal

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/bytesx"
)

var bufs sync.Pool

// Format implements [fmt.Formatter].
//
//nolint:errcheck
func (z *Decimal) Format(state fmt.State, verb rune) {
	if z == nil {
		fmt.Fprintf(state, "<nil>")
		return
	}

	if verb == 'b' || z.inf {
		switch {
		case z.neg:
			fmt.Fprint(state, "-")
		case state.Flag('+'):
			fmt.Fprint(state, "+")
		}

		if z.inf {
			fmt.Fprint(state, "Infinity")
			return
		}

		n, _ := bigx.Format(state, "%v", z.get())
		e := 'e'
		if z.base2 {
			n = z.digits()
			e = 'p'
		}

		fmt.Fprintf(state, "%c%+03d", e, int(z.exp)-n)
		return
	}

	if z.base2 {
		fmt.Fprintf(state, fmt.FormatString(state, verb), z.Float())
		return
	}

	exp := int(z.exp)
	prec, havePrec := state.Precision()
	if !havePrec {
		prec = -1
	}

	var scientific bool
	switch verb {
	case 'v', 'g', 'G':
		exp := max(exp, exp-z.digits())
		scientific = exp <= -4 || (havePrec && exp > prec)
	case 'e', 'E':
		scientific = true
		fallthrough
	case 'f', 'F':
		if !havePrec {
			prec = 6
			havePrec = true
		}
	case 'x', 'X':
		fmt.Fprintf(state, fmt.FormatString(state, verb), z.Float())
	case 'b':
		n, _ := bigx.Format(state, "%v", z.get())
		e := 'e'
		if z.base2 {
			e = 'p'
		}

		fmt.Fprintf(state, "%c%+03d", e, exp-n)
		return
	default:
		fmt.Fprintf(state, "%%%c<%T=%v>", verb, z, z)
		return
	}

	buf, _ := bufs.Get().(*bytesx.Writer)
	if buf == nil {
		buf = new(bytesx.Writer)
	}
	defer func() {
		state.Write(*buf)
		buf.Reset()
		bufs.Put(buf)
	}()

	switch {
	case z.neg:
		buf.WriteByte('-')
	case state.Flag('+'):
		buf.WriteByte('+')
	}

	if z.IsZero() {
		// Handling zero explicitly simplifies the logic below substantially.
		buf.WriteByte('0')
		if prec > 0 {
			buf.WriteByte('.')
			for range prec {
				buf.WriteByte('0')
			}
		}
	} else {
		// Write out all the digits we have to work with in this base.
		start := len(*buf)
		bigx.Format(buf, "%v", z.get())
		digits := len(*buf) - start

		// Point is the number of digits before the decimal point.
		//
		// Note that for negative values, this means we need to insert leading
		// zeros.
		point := exp
		if z.base2 {
			point = (point + 1) / 2
		}
		if scientific {
			point = 1
		}

		// If point is negative, we need to insert that many leading zeros, plus
		// a zero before the decimal point.
		if point <= 0 {
			buf.InsertString(start, strings.Repeat("0", 1-point))
			point = 1
		} else if point > digits {
			// If it's positive, we need to append zeros at the end.
			buf.WriteString(strings.Repeat("0", point-digits))
		}
		digits = len(*buf) - start

		// Now we need to discard extra digits, or add extra digits if necessary.
		if havePrec {
			if prec+1 < digits-point {
				// TODO: rounding!
				*buf = (*buf)[:start+prec+1]
			}

			// If there aren't enough digits, append some.
			for range prec - (digits - point) {
				buf.WriteByte('0')
			}
		}

		// If prec == 0, we don't include a decimal point.
		if n := start + max(1, point); n < len(*buf) {
			*buf = slices.Insert(*buf, n, '.')
		}
	}

	if scientific {
		e := 'e'
		if unicode.IsUpper(verb) {
			e = unicode.ToUpper(e)
		}

		exp := z.exp - 1
		if z.base2 {
			z.exp--
		}
		if z.IsZero() {
			exp = 0
		}
		// Print the exponent: we're done!
		fmt.Fprintf(buf, "%c%+03d", e, exp)
	}
}
