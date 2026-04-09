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

	if !try(f.sign(w, z)) {
		return
	}

	if f.Repr || !z.IsFinite() {
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

	if f.Hex {
		if !writeStr("0x") {
			return
		}
	}

	prec := max(f.Precision, -1)
	havePrec := f.Precision >= 0

	var e string
	switch {
	case !f.Hex && !f.Upper:
		e = "e"
	case !f.Hex && f.Upper:
		e = "E"
	case f.Hex && !f.Upper:
		e = "p"
	case f.Hex && f.Upper:
		e = "P"
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
	//
	// Four cases:
	// 1. base 10 exp, base 10 output.
	// 2. base 10 exp, base 16 output.
	// 3. base 2 exp, base 10 output.
	// 4. base 2 exp, base 16 output.
	//
	// When the bases are different, the easiest way to convert (right now)
	// is to go through big.Float. Eventually it may be wise to (in base 10
	// mode) to make the mantissa binary-coded decimal.
	//
	// Point is the number of digits before the decimal point.
	//
	// Note that for negative values, this means we need to insert leading
	// zeros.
	var exp, point int
	switch {
	case z.base10() && !f.Hex:
		exp = int(z.exp) - 1
		point = exp + 1
		*buf = bigx.Format(*buf, z.get(), 10)

	case z.base10() && f.Hex:
		// Convert to f. It's best to take advantage of Go's existing
		// implementation of formatting big floats.
		f, _ := bigFloats.Get().(*big.Float)
		f = z.float(f)
		*buf = f.Append(*buf, 'p', -1)

		// Retrieve and delete the exponent.
		expIdx := bytes.LastIndexByte(*buf, 'p')
		exp, _ = strconv.Atoi(unsafex.StringAlias((*buf)[:expIdx+1]))
		point = exp + 1

		*buf = (*buf)[:expIdx]

		// Delete the 0x. prefix.
		*buf = slices.Delete(*buf, 0, len("0x."))

		bigFloats.Put(f)

	case z.base2() && f.Hex:
		exp = int(z.exp) - 4
		point = (int(exp)+3)/4 + 1
		*buf = bigx.Format(*buf, z.get(), 16)
		if f.Upper {
			// Correct all of the digits to be uppercase if needed.
			for i := 0; i < len(*buf); i++ {
				(*buf)[i] = byte(unicode.ToUpper(rune((*buf)[i])))
			}
		}

	case z.base2() && !f.Hex:
		// Also convert to f here.
		f, _ := bigFloats.Get().(*big.Float)
		f = z.float(f)
		*buf = f.Append(*buf, 'f', -1)

		// Delete the decimal dot and retrieve its position.
		point = bytes.IndexByte(*buf, '.')
		if point != -1 {
			*buf = slices.Delete(*buf, point, point+1)
		} else {
			point = len(*buf)
		}
		exp = point - 1

		bigFloats.Put(f)
	default:
		panic("unreachable")
	}

	digits := len(*buf)
	if f.Exp {
		point = 1
	}

	// If point is negative, we need to insert that many leading zeros, plus
	// a zero before the decimal point.
	if point <= 0 {
		buf.InsertString(0, strings.Repeat("0", 1-point))
		point = 1
	} else if point > digits {
		// If it's positive, we need to append zeros at the end.
		buf.WriteString(strings.Repeat("0", point-digits))
	}
	digits = len(*buf)

	// Now we need to discard extra digits, or add extra digits if necessary.
	if havePrec {
		if prec+1 < digits-point {
			// TODO: rounding!
			*buf = (*buf)[:prec+1]
		}

		// If there aren't enough digits, append some.
		for range prec - (digits - point) {
			buf.WriteByte('0')
		}
	}

	// If prec == 0, we don't include a decimal point.
	if n := max(1, point); n < len(*buf) {
		*buf = slices.Insert(*buf, n, '.')
	}

	// Print the exponent: we're done!
	if f.Exp {
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
