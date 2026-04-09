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
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/bufbuild/protocompile/internal/ext/bigx"
	"github.com/bufbuild/protocompile/internal/ext/bytesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

var bufs = sync.Pool{New: func() any { return new(bytesx.Writer) }}
var bigFloats = sync.Pool{New: func() any { return new(big.Float) }}

// Formatter implements generic decimal formatting operations.
type Formatter struct {
	Precision int // Number of digits after the decimal point to print. Use < 0 for infinite precision.

	Hex   bool // Print mantissa in hex.
	Exp   bool // Print a scientific notation exponent.
	Upper bool // Use uppercase letters.
	Plus  bool // Print a + sign if the value is positive.
	Extra bool // Print extra information, compare %#v.

	Repr bool // If set, prints in a special dddde+nn format, which is always exact.
}

// Format formats a decimal value according to f's rules.
func (f Formatter) Format(w io.Writer, z *Decimal) (total int, err error) {
	if z == nil {
		return fmt.Fprintf(w, "<nil>")
	}

	try := func(n int, e error) bool {
		total += n
		err = e
		return e == nil
	}

	writeStr := func(s ...string) bool {
		for _, s := range s {
			if !try(io.WriteString(w, s)) {
				return false
			}
		}
		return true
	}

	if f.Repr || !z.IsFinite() {
		if !try(f.sign(w, z)) {
			return
		}

		switch {
		case z.IsInf():
			if !writeStr("Infinity") {
				return
			}

		case z.IsNaN():
			if !writeStr("NaN") {
				return
			}
			if f.Extra && !try(fmt.Fprintf(w, "#%014x", z.NaN())) {
				return
			}

		default:
			buf, _ := bufs.Get().(*bytesx.Writer)
			defer func() {
				buf.Reset()
				bufs.Put(buf)
			}()

			*buf = bigx.Format(*buf, z.get(), 10)
			n := len(*buf)
			e := 'e'
			if z.base2() {
				n = z.digits()
				e = 'p'
			}

			if !try(fmt.Fprintf(w, "%s%c%+03d", *buf, e, int(z.exp)-n)) {
				return
			}
		}

		return
	}

	exp := int(z.exp)
	prec := max(f.Precision, -1)
	havePrec := f.Precision >= 0

	e := "e"
	if f.Upper {
		e = "E"
	}

	if !try(f.sign(w, z)) {
		return
	}

	if z.IsZero() {
		// Handling zero explicitly simplifies the logic below substantially.
		if !writeStr("0") {
			return
		}
		if prec > 0 {
			if !writeStr(".") {
				return
			}
			for range prec {
				if !writeStr("0") {
					return
				}
			}
		}
		if f.Exp && !writeStr(e, "+00") {
			return
		}
		return
	}

	buf, _ := bufs.Get().(*bytesx.Writer)
	defer func() {
		try(w.Write(*buf))
		buf.Reset()
		bufs.Put(buf)
	}()

	// Write out all the digits we have to work with in this base.
	start := len(*buf)

	// Four cases:
	// 1. base 10 exp, base 10 output.
	// 2. base 10 exp, base 16 output.
	// 3. base 2 exp, base 10 output.
	// 4. base 2 exp, base 16 output.
	switch {
	case z.base10() && !f.Hex:
		*buf = bigx.Format(*buf, z.get(), 10)

	case z.base10() && f.Hex:
		// Convert to float. It's best to take advantage of Go's existing
		// implementation of formatting big floats.
		float, _ := bigFloats.Get().(*big.Float)
		float = z.float(float)

		*buf = float.Append(*buf, 'p', -1)

		// Delete the exponent.
		idx := bytes.LastIndexByte(*buf, 'p')
		*buf = (*buf)[:idx]

		// Delete the 0x. prefix.
		*buf = slices.Delete(*buf, start, start+len("0x."))

		bigFloats.Put(float)

	case z.base2() && f.Hex:
		*buf = bigx.Format(*buf, z.get(), 16)
		if f.Upper {
			// Correct all of the digits to be uppercase if needed.
			for i := start; i < len(*buf); i++ {
				(*buf)[i] = byte(unicode.ToUpper(rune((*buf)[i])))
			}
		}

	case z.base2() && !f.Hex:
		// Also convert to float here.
		float, _ := bigFloats.Get().(*big.Float)
		float = z.float(float)

		*buf = float.Append(*buf, 'f', -1)

		// Delete the decimal dot.
		idx := bytes.LastIndexByte(*buf, '.')
		*buf = slices.Delete(*buf, idx, idx+1)

		bigFloats.Put(float)
	default:
		panic("unreachable")
	}
	digits := len(*buf) - start

	// Point is the number of digits before the decimal point.
	//
	// Note that for negative values, this means we need to insert leading
	// zeros.
	point := exp
	if z.base2() {
		point = (point + 1) / 2
	}
	if f.Exp {
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

	if f.Exp {
		exp := z.exp - 1
		if z.base2() {
			z.exp--
		}
		if z.IsZero() {
			exp = 0
		}
		// Print the exponent: we're done!
		fmt.Fprintf(buf, "%s%+03d", e, exp)
	}

	return
}

func (f Formatter) sign(w io.Writer, z *Decimal) (int, error) {
	switch {
	case z.Negative():
		return io.WriteString(w, "-")
	case f.Plus:
		return io.WriteString(w, "+")
	default:
		return 0, nil
	}
}

// formatString writes an appropriate format string to b.
func (f Formatter) formatString(z *Decimal, b []byte) string {
	b = append(b, '%')

	if f.Plus {
		b = append(b, '+')
	}
	if f.Extra {
		b = append(b, '#')
	}

	prec := f.Precision
	if prec < 0 {
		prec = max(0, int(z.exp))
		fmt.Println(z.get(), z.digits(), z.exp, prec)
		if f.Hex {
			prec = (prec + 1) / 2
		} else if z.base2() && z.exp < 0 {
			prec = int(float64(prec)*(math.Ln2/math.Ln10) + 1)
		}
	}
	b = append(b, '.')
	b = strconv.AppendInt(b, int64(prec), 10)

	var c byte
	switch {
	case f.Hex:
		if f.Upper {
			c = 'X'
		} else {
			c = 'x'
		}
	case f.Exp:
		if f.Upper {
			c = 'E'
		} else {
			c = 'e'
		}
	default:
		if f.Upper {
			c = 'F'
		} else {
			c = 'f'
		}
	}

	b = append(b, c)

	s := unsafex.StringAlias(b)
	fmt.Println(f, s)
	return s
}

// Format implements [fmt.Formatter].
//
//nolint:errcheck
func (z *Decimal) Format(state fmt.State, verb rune) {
	prec, havePrec := state.Precision()
	if !havePrec {
		prec = -1
	}

	f := Formatter{
		Precision: prec,
		Plus:      state.Flag('+'),
		Exp:       state.Flag('#'),
		Upper:     unicode.IsUpper(verb),
	}

	switch verb {
	case 'b':
		f.Repr = true
	case 'v', 'g', 'G':
		exp := int(z.exp)
		exp = max(exp, exp-z.digits())
		f.Exp = exp <= -4 || (havePrec && exp > prec)
	case 'e', 'E':
		f.Exp = true
		fallthrough
	case 'f', 'F':
		if !havePrec {
			f.Precision = 6
		}
	case 'x', 'X':
		f.Hex = true
	default:
		fmt.Fprintf(state, "%%%c<%T=%v>", verb, z, z)
		return
	}

	_, _ = f.Format(state, z)
}
