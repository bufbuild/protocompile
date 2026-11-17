// Copyright 2020-2026 Buf Technologies, Inc.
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
	"math"
	"math/big"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var nzero = func() float64 {
	zero := float64(0)
	return -zero
}()

func TestParse(t *testing.T) {
	t.Parallel()

	type m map[string]string
	tests := []struct {
		in         string
		invalid    bool
		want, ints m
		u64s, f64s m
	}{
		{
			in: "0",
			want: m{
				"%f":  "0.000000",
				"%F":  "0.000000",
				"%+f": "+0.000000",
				"%+F": "+0.000000",
				"%e":  "0.000000e+00",
				"%E":  "0.000000E+00",
				"%+e": "+0.000000e+00",
				"%+E": "+0.000000E+00",
				"%g":  "0",
				"%G":  "0",
				"%+g": "+0",
				"%+G": "+0",
			},
			f64s: m{"%g": "0"},
		},

		{
			in: "0x00",
			want: m{
				"%f":  "0.000000",
				"%F":  "0.000000",
				"%+f": "+0.000000",
				"%+F": "+0.000000",
				"%e":  "0.000000e+00",
				"%E":  "0.000000E+00",
				"%+e": "+0.000000e+00",
				"%+E": "+0.000000E+00",
				"%g":  "0",
				"%G":  "0",
				"%+g": "+0",
				"%+G": "+0",
			},
			f64s: m{"%g": "0"},
		},

		{
			in: "-0",
			want: m{
				"%f":  "-0.000000",
				"%F":  "-0.000000",
				"%+f": "-0.000000",
				"%+F": "-0.000000",
				"%e":  "-0.000000e+00",
				"%E":  "-0.000000E+00",
				"%+e": "-0.000000e+00",
				"%+E": "-0.000000E+00",
				"%g":  "-0",
				"%G":  "-0",
				"%+g": "-0",
				"%+G": "-0",
			},
			f64s: m{"%g": "-0"},
		},

		{
			in: "-0x00",
			want: m{
				"%f":  "-0.000000",
				"%F":  "-0.000000",
				"%+f": "-0.000000",
				"%+F": "-0.000000",
				"%e":  "-0.000000e+00",
				"%E":  "-0.000000E+00",
				"%+e": "-0.000000e+00",
				"%+E": "-0.000000E+00",
				"%g":  "-0",
				"%G":  "-0",
				"%+g": "-0",
				"%+G": "-0",
			},
			f64s: m{"%g": "-0"},
		},

		{
			in: "1",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "0x01",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "-1",
			want: m{
				"%f":  "-1.000000",
				"%F":  "-1.000000",
				"%+f": "-1.000000",
				"%+F": "-1.000000",
				"%e":  "-1.000000e+00",
				"%E":  "-1.000000E+00",
				"%+e": "-1.000000e+00",
				"%+E": "-1.000000E+00",
				"%g":  "-1",
				"%G":  "-1",
				"%+g": "-1",
				"%+G": "-1",
			},
			f64s: m{"%g": "-1"},
		},

		{
			in: "-0x01",
			want: m{
				"%f":  "-1.000000",
				"%F":  "-1.000000",
				"%+f": "-1.000000",
				"%+F": "-1.000000",
				"%e":  "-1.000000e+00",
				"%E":  "-1.000000E+00",
				"%+e": "-1.000000e+00",
				"%+E": "-1.000000E+00",
				"%g":  "-1",
				"%G":  "-1",
				"%+g": "-1",
				"%+G": "-1",
			},
			f64s: m{"%g": "-1"},
		},

		{
			in: "+1",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "+0x01",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "42",
			want: m{
				"%f":  "42.000000",
				"%F":  "42.000000",
				"%+f": "+42.000000",
				"%+F": "+42.000000",
				"%e":  "4.200000e+01",
				"%E":  "4.200000E+01",
				"%+e": "+4.200000e+01",
				"%+E": "+4.200000E+01",
				"%g":  "42",
				"%G":  "42",
				"%+g": "+42",
				"%+G": "+42",
			},
			f64s: m{"%g": "42"},
		},

		{
			in: "0x42",
			want: m{
				"%f":  "66.000000",
				"%F":  "66.000000",
				"%+f": "+66.000000",
				"%+F": "+66.000000",
				"%e":  "6.600000e+01",
				"%E":  "6.600000E+01",
				"%+e": "+6.600000e+01",
				"%+E": "+6.600000E+01",
				"%g":  "66",
				"%G":  "66",
				"%+g": "+66",
				"%+G": "+66",
			},
			f64s: m{"%g": "66"},
		},

		{
			in: "100",
			want: m{
				"%f":  "100.000000",
				"%F":  "100.000000",
				"%+f": "+100.000000",
				"%+F": "+100.000000",
				"%e":  "1.000000e+02",
				"%E":  "1.000000E+02",
				"%+e": "+1.000000e+02",
				"%+E": "+1.000000E+02",
				"%g":  "100",
				"%G":  "100",
				"%+g": "+100",
				"%+G": "+100",
			},
			f64s: m{"%g": "100"},
		},

		{
			in: "999999",
			want: m{
				"%f":  "999999.000000",
				"%F":  "999999.000000",
				"%+f": "+999999.000000",
				"%+F": "+999999.000000",
				"%e":  "9.999990e+05",
				"%E":  "9.999990E+05",
				"%+e": "+9.999990e+05",
				"%+E": "+9.999990E+05",
				"%g":  "999999",
				"%G":  "999999",
				"%+g": "+999999",
				"%+G": "+999999",
			},
			f64s: m{"%g": "999999"},
		},

		{
			in: "12345678901234567890123456789012345678901234567890123456789012345678901234567890",
			want: m{
				"%f":  "12345678901234567890123456789012345678901234567890123456789012345678901234567890.000000",
				"%F":  "12345678901234567890123456789012345678901234567890123456789012345678901234567890.000000",
				"%+f": "+12345678901234567890123456789012345678901234567890123456789012345678901234567890.000000",
				"%+F": "+12345678901234567890123456789012345678901234567890123456789012345678901234567890.000000",
				"%e":  "1.234567e+79",
				"%E":  "1.234567E+79",
				"%+e": "+1.234567e+79",
				"%+E": "+1.234567E+79",
				"%g":  "12345678901234567890123456789012345678901234567890123456789012345678901234567890",
				"%G":  "12345678901234567890123456789012345678901234567890123456789012345678901234567890",
				"%+g": "+12345678901234567890123456789012345678901234567890123456789012345678901234567890",
				"%+G": "+12345678901234567890123456789012345678901234567890123456789012345678901234567890",
			},
			f64s: m{"%g": "1.2345678901234568e+79"},
		},

		{
			in: "0.12345678901234567890123456789012345678901234567890123456789012345678901234567890",
			want: m{
				"%f":  "0.123456",
				"%F":  "0.123456",
				"%+f": "+0.123456",
				"%+F": "+0.123456",
				"%e":  "1.234567e-01",
				"%E":  "1.234567E-01",
				"%+e": "+1.234567e-01",
				"%+E": "+1.234567E-01",
				"%g":  "0.1234567890123456789012345678901234567890123456789012345678901234567890123456789",
				"%G":  "0.1234567890123456789012345678901234567890123456789012345678901234567890123456789",
				"%+g": "+0.1234567890123456789012345678901234567890123456789012345678901234567890123456789",
				"%+G": "+0.1234567890123456789012345678901234567890123456789012345678901234567890123456789",
			},
			f64s: m{"%g": "0.12345678901234568"},
		},

		{
			in: "0xffffffffffffffffffffffffffffffff",
			want: m{
				"%g": "340282366920938463463374607431768211455",
				"%x": "0xffffffffffffffffffffffffffffffff",
			},
			ints: m{"%#x": "0xffffffffffffffffffffffffffffffff"},
		},

		{
			in: "0.1",
			want: m{
				"%f":  "0.100000",
				"%F":  "0.100000",
				"%+f": "+0.100000",
				"%+F": "+0.100000",
				"%e":  "1.000000e-01",
				"%E":  "1.000000E-01",
				"%+e": "+1.000000e-01",
				"%+E": "+1.000000E-01",
				"%g":  "0.1",
				"%G":  "0.1",
				"%+g": "+0.1",
				"%+G": "+0.1",
			},
			f64s: m{"%g": "0.1"},
		},

		{
			in: "0.5",
			want: m{
				"%f":  "0.500000",
				"%F":  "0.500000",
				"%+f": "+0.500000",
				"%+F": "+0.500000",
				"%e":  "5.000000e-01",
				"%E":  "5.000000E-01",
				"%+e": "+5.000000e-01",
				"%+E": "+5.000000E-01",
				"%g":  "0.5",
				"%G":  "0.5",
				"%+g": "+0.5",
				"%+G": "+0.5",
			},
			f64s: m{"%g": "0.5"},
		},

		{
			in: "1.5",
			want: m{
				"%f":  "1.500000",
				"%F":  "1.500000",
				"%+f": "+1.500000",
				"%+F": "+1.500000",
				"%e":  "1.500000e+00",
				"%E":  "1.500000E+00",
				"%+e": "+1.500000e+00",
				"%+E": "+1.500000E+00",
				"%g":  "1.5",
				"%G":  "1.5",
				"%+g": "+1.5",
				"%+G": "+1.5",
			},
			f64s: m{"%g": "1.5"},
		},

		{
			in: "-1.5",
			want: m{
				"%f":  "-1.500000",
				"%F":  "-1.500000",
				"%+f": "-1.500000",
				"%+F": "-1.500000",
				"%e":  "-1.500000e+00",
				"%E":  "-1.500000E+00",
				"%+e": "-1.500000e+00",
				"%+E": "-1.500000E+00",
				"%g":  "-1.5",
				"%G":  "-1.5",
				"%+g": "-1.5",
				"%+G": "-1.5",
			},
			f64s: m{"%g": "-1.5"},
		},

		{
			in: "3.14",
			want: m{
				"%f":  "3.140000",
				"%F":  "3.140000",
				"%+f": "+3.140000",
				"%+F": "+3.140000",
				"%e":  "3.140000e+00",
				"%E":  "3.140000E+00",
				"%+e": "+3.140000e+00",
				"%+E": "+3.140000E+00",
				"%g":  "3.14",
				"%G":  "3.14",
				"%+g": "+3.14",
				"%+G": "+3.14",
			},
			f64s: m{"%g": "3.14"},
		},

		{
			in: "123.456",
			want: m{
				"%f":  "123.456000",
				"%F":  "123.456000",
				"%+f": "+123.456000",
				"%+F": "+123.456000",
				"%e":  "1.234560e+02",
				"%E":  "1.234560E+02",
				"%+e": "+1.234560e+02",
				"%+E": "+1.234560E+02",
				"%g":  "123.456",
				"%G":  "123.456",
				"%+g": "+123.456",
				"%+G": "+123.456",
			},
			f64s: m{"%g": "123.456"},
		},

		{
			in: "0.001",
			want: m{
				"%f":  "0.001000",
				"%F":  "0.001000",
				"%+f": "+0.001000",
				"%+F": "+0.001000",
				"%e":  "1.000000e-03",
				"%E":  "1.000000E-03",
				"%+e": "+1.000000e-03",
				"%+E": "+1.000000E-03",
				"%g":  "0.001",
				"%G":  "0.001",
				"%+g": "+0.001",
				"%+G": "+0.001",
			},
			f64s: m{"%g": "0.001"},
		},

		{
			in: "0.0001",
			want: m{
				"%f":  "0.000100",
				"%F":  "0.000100",
				"%+f": "+0.000100",
				"%+F": "+0.000100",
				"%e":  "1.000000e-04",
				"%E":  "1.000000E-04",
				"%+e": "+1.000000e-04",
				"%+E": "+1.000000E-04",
				"%g":  "0.0001",
				"%G":  "0.0001",
				"%+g": "+0.0001",
				"%+G": "+0.0001",
			},
			f64s: m{"%g": "0.0001"},
		},

		{
			in: "0.00001",
			want: m{
				"%f":  "0.000010",
				"%F":  "0.000010",
				"%+f": "+0.000010",
				"%+F": "+0.000010",
				"%e":  "1.000000e-05",
				"%E":  "1.000000E-05",
				"%+e": "+1.000000e-05",
				"%+E": "+1.000000E-05",
				"%g":  "1e-05",
				"%G":  "1E-05",
				"%+g": "+1e-05",
				"%+G": "+1E-05",
			},
			f64s: m{"%g": "1e-05"},
		},

		{
			in: "0.00000001",
			want: m{
				"%f":  "0.000000",
				"%F":  "0.000000",
				"%+f": "+0.000000",
				"%+F": "+0.000000",
				"%e":  "1.000000e-08",
				"%E":  "1.000000E-08",
				"%+e": "+1.000000e-08",
				"%+E": "+1.000000E-08",
				"%g":  "1e-08",
				"%G":  "1E-08",
				"%+g": "+1e-08",
				"%+G": "+1E-08",
			},
			f64s: m{"%g": "1e-08"},
		},

		{
			in: "1e0",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "1e-0",
			want: m{
				"%f":  "1.000000",
				"%F":  "1.000000",
				"%+f": "+1.000000",
				"%+F": "+1.000000",
				"%e":  "1.000000e+00",
				"%E":  "1.000000E+00",
				"%+e": "+1.000000e+00",
				"%+E": "+1.000000E+00",
				"%g":  "1",
				"%G":  "1",
				"%+g": "+1",
				"%+G": "+1",
			},
			f64s: m{"%g": "1"},
		},

		{
			in: "1e5",
			want: m{
				"%f":  "100000.000000",
				"%F":  "100000.000000",
				"%+f": "+100000.000000",
				"%+F": "+100000.000000",
				"%e":  "1.000000e+05",
				"%E":  "1.000000E+05",
				"%+e": "+1.000000e+05",
				"%+E": "+1.000000E+05",
				"%g":  "100000",
				"%G":  "100000",
				"%+g": "+100000",
				"%+G": "+100000",
			},
			f64s: m{"%g": "100000"},
		},

		{
			in: "1e+5",
			want: m{
				"%f":  "100000.000000",
				"%F":  "100000.000000",
				"%+f": "+100000.000000",
				"%+F": "+100000.000000",
				"%e":  "1.000000e+05",
				"%E":  "1.000000E+05",
				"%+e": "+1.000000e+05",
				"%+E": "+1.000000E+05",
				"%g":  "100000",
				"%G":  "100000",
				"%+g": "+100000",
				"%+G": "+100000",
			},
			f64s: m{"%g": "100000"},
		},

		{
			in: "1e-5",
			want: m{
				"%f":  "0.000010",
				"%F":  "0.000010",
				"%+f": "+0.000010",
				"%+F": "+0.000010",
				"%e":  "1.000000e-05",
				"%E":  "1.000000E-05",
				"%+e": "+1.000000e-05",
				"%+E": "+1.000000E-05",
				"%g":  "1e-05",
				"%G":  "1E-05",
				"%+g": "+1e-05",
				"%+G": "+1E-05",
			},
			f64s: m{"%g": "1e-05"},
		},

		{
			in: "2.5e3",
			want: m{
				"%f":  "2500.000000",
				"%F":  "2500.000000",
				"%+f": "+2500.000000",
				"%+F": "+2500.000000",
				"%e":  "2.500000e+03",
				"%E":  "2.500000E+03",
				"%+e": "+2.500000e+03",
				"%+E": "+2.500000E+03",
				"%g":  "2500",
				"%G":  "2500",
				"%+g": "+2500",
				"%+G": "+2500",
			},
			f64s: m{"%g": "2500"},
		},

		{
			in: "1_000.5",
			want: m{
				"%f":  "1000.500000",
				"%F":  "1000.500000",
				"%+f": "+1000.500000",
				"%+F": "+1000.500000",
				"%e":  "1.000500e+03",
				"%E":  "1.000500E+03",
				"%+e": "+1.000500e+03",
				"%+E": "+1.000500E+03",
				"%g":  "1000.5",
				"%G":  "1000.5",
				"%+g": "+1000.5",
				"%+G": "+1000.5",
			},
			f64s: m{"%g": "1000.5"},
		},

		// Negative fraction.
		{
			in: "-0.5",
			want: m{
				"%f":  "-0.500000",
				"%F":  "-0.500000",
				"%+f": "-0.500000",
				"%+F": "-0.500000",
				"%e":  "-5.000000e-01",
				"%E":  "-5.000000E-01",
				"%+e": "-5.000000e-01",
				"%+E": "-5.000000E-01",
				"%g":  "-0.5",
				"%G":  "-0.5",
				"%+g": "-0.5",
				"%+G": "-0.5",
			},
			f64s: m{"%g": "-0.5"},
		},

		// Negative with large magnitude and fraction.
		{
			in: "-123.456",
			want: m{
				"%f":  "-123.456000",
				"%F":  "-123.456000",
				"%+f": "-123.456000",
				"%+F": "-123.456000",
				"%e":  "-1.234560e+02",
				"%E":  "-1.234560E+02",
				"%+e": "-1.234560e+02",
				"%+E": "-1.234560E+02",
				"%g":  "-123.456",
				"%G":  "-123.456",
				"%+g": "-123.456",
				"%+G": "-123.456",
			},
			f64s: m{"%g": "-123.456"},
		},

		// Scientific input with negative fractional mantissa.
		{
			in: "-2.5e-3",
			want: m{
				"%f":  "-0.002500",
				"%F":  "-0.002500",
				"%+f": "-0.002500",
				"%+F": "-0.002500",
				"%e":  "-2.500000e-03",
				"%E":  "-2.500000E-03",
				"%+e": "-2.500000e-03",
				"%+E": "-2.500000E-03",
				"%g":  "-0.0025",
				"%G":  "-0.0025",
				"%+g": "-0.0025",
				"%+G": "-0.0025",
			},
			f64s: m{"%g": "-0.0025"},
		},

		{
			in:   ".953e20",
			want: m{"%g": "95300000000000000000"},
			f64s: m{"%g": "9.53e+19"},
			ints: m{"%v": "95300000000000000000"},
		},

		{
			in:   "1.7976931348623157E+308",
			f64s: m{"%g": "1.7976931348623157e+308"},
		},

		{
			in:   "2.2250738585072014E-308",
			want: m{"%g": "2.2250738585072014e-308"},
			ints: m{"%v": "0"},
			f64s: m{"%g": "2.2250738585072014e-308"},
		},

		// Invalid: empty input.
		{in: "", invalid: true},
		// Invalid: non-numeric.
		{in: "abc", invalid: true},
		// Invalid: double dot.
		{in: "1.2.3", invalid: true},
		// Invalid: exponent with no digits.
		{in: "1e", invalid: true},
		// Mixing bases: e is ambiguous here, and when we get to the + we see
		// an error. 0x0.fe4 is valid, because e is a hex digit.
		{in: "0x0.fe+4", invalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			z, err := new(Decimal).Parse(tt.in)
			if tt.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			t.Logf("%b", z)

			for format, want := range tt.want {
				t.Run(format, func(t *testing.T) {
					t.Parallel()
					assert.Equal(t, want, fmt.Sprintf(format, z))
				})
			}

			for format, want := range tt.ints {
				t.Run("int/"+format, func(t *testing.T) {
					t.Parallel()
					assert.Equal(t, want, fmt.Sprintf(format, z.Int(nil)))
				})
			}

			for format, want := range tt.u64s {
				t.Run("u64/"+format, func(t *testing.T) {
					t.Parallel()
					assert.Equal(t, want, fmt.Sprintf(format, z.Int(nil).Uint64()))
				})
			}

			for format, want := range tt.f64s {
				t.Run("f64/"+format, func(t *testing.T) {
					t.Parallel()
					f, _ := z.Float64()
					assert.Equal(t, want, fmt.Sprintf(format, f))
				})
			}
		})
	}
}

func TestIsInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{in: "0", want: true},
		{in: "1", want: true},
		{in: "42", want: true},
		{in: "1000000000000000000000000000000000000000000000000000", want: true},
		{in: "1e100", want: true},
		{in: "1.999e100", want: true},

		{in: "0.5", want: false},
		{in: "1.5", want: false},
		{in: "1e-100", want: false},
		{in: "10000e-6", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			z, err := new(Decimal).Parse(tt.in)
			require.NoError(t, err)
			t.Logf("%b", z)

			assert.Equal(t, tt.want, z.IsInt())
		})
	}
}

func TestUint64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in    string
		want  uint64
		fits  bool
		large bool // Too large to check against Int.
	}{
		{in: "0", want: 0, fits: true},
		{in: "-0", want: 0, fits: true},
		{in: "42", want: 42, fits: true},
		{in: "1e19", want: 1e19, fits: true},
		{in: "18446744073709551615", want: math.MaxUint64, fits: true},
		{in: "1844674407370955161.5e1", want: math.MaxUint64, fits: true},
		{in: "0x1p63", want: 1 << 63, fits: true},
		{in: "0xffffffffffffffff", want: math.MaxUint64, fits: true},

		{in: "1.5", fits: false},
		{in: "123.5", fits: false},
		{in: "-5", fits: false},
		{in: "-0x1p10", fits: false},
		{in: "18446744073709551616", fits: false},
		{in: "2e19", fits: false},
		{in: "1e20", fits: false},
		{in: "0x1p64", fits: false},
		{in: "0x10000000000000000", fits: false},
		{in: "1e2147483646", fits: false, large: true},
		{in: "0x1p2147483646", fits: false, large: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			z, err := new(Decimal).Parse(tt.in)
			require.NoError(t, err)

			got, fits := z.Uint64()
			assert.Equal(t, tt.fits, fits)
			if tt.fits {
				assert.Equal(t, tt.want, got)
			}
			if !tt.large {
				// Int ignores the sign and truncates non-integers to zero, so
				// it only agrees with Uint64 on nonnegative integers.
				n := z.Int(nil)
				if z.IsInt() && !z.Negative() {
					assert.Equal(t, n.IsUint64(), fits)
				}
				if fits {
					assert.Equal(t, n.Uint64(), got)
				}
			}
		})
	}

	_, fits := new(Decimal).SetInf(false).Uint64()
	assert.False(t, fits)
	_, fits = new(Decimal).SetNaN().Uint64()
	assert.False(t, fits)
}

func TestUint64Allocs(t *testing.T) {
	// Not parallel: testing.AllocsPerRun panics during parallel tests.
	for _, in := range []string{"1e5", "18446744073709551615e0", "0x1p63", "1e20"} {
		z, err := new(Decimal).Parse(in)
		require.NoError(t, err)
		allocs := testing.AllocsPerRun(100, func() { _, _ = z.Uint64() })
		assert.Zero(t, allocs, in)
	}
}

func TestIsFloat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{in: "0", want: true},
		{in: "1", want: true},
		{in: "42", want: true},
		{in: "1000000000000000000000000000000000000000000000000000", want: true},
		{in: "1e100", want: true},
		{in: "1.999e100", want: true},

		{in: "0.1", want: false},
		{in: "0.3", want: false},
		{in: "1.1", want: false},
		{in: "0.5", want: true},
		{in: "1.5", want: true},
		{in: "1e-100", want: false},
		{in: "10000e-6", want: false},
		{in: "8470329472543003390683225006796419620513916015625e-70", want: true}, // 2^-70
		{in: "1e-2147483646", want: false},
		{in: "1.5e-2147483600", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			z, err := new(Decimal).Parse(tt.in)
			require.NoError(t, err)
			t.Logf("%b", z)

			assert.Equal(t, tt.want, z.IsFloat())
		})
	}
}

func TestSetFloat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float64
		want string
	}{
		{in: 0, want: "0"},
		{in: nzero, want: "-0"},
		{in: math.Inf(1), want: "Infinity"},
		{in: math.Inf(-11), want: "-Infinity"},

		{in: 1, want: "1"},
		{in: -1, want: "-1"},
		{in: 1.5, want: "1.5"},
		{in: -1.5, want: "-1.5"},
		{in: 0.125, want: "0.125"},
		{in: -0.125, want: "-0.125"},
		{in: 0x42, want: "66"},
		{in: 1e100, want: "10000000000000000159028911097599180468360808563945281389781327557747838772170381060813469985856815104"},
		{in: 0x1p100, want: "1267650600228229401496703205376"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%g", tt.in), func(t *testing.T) {
			t.Parallel()
			z := new(Decimal).SetFloat64(tt.in)
			t.Logf("%b", z)
			t.Logf("%e", z)

			assert.Equal(t, tt.want, fmt.Sprint(z))
		})
	}
}

func TestNonFinite(t *testing.T) {
	t.Parallel()

	n := new(Decimal)
	f, _ := n.Float64()
	assert.True(t, n.IsFinite())
	assert.False(t, n.IsInf())
	assert.False(t, n.IsNaN())
	assert.False(t, n.IsQuietNaN())
	assert.False(t, n.IsSignalingNaN())
	assert.False(t, math.IsInf(f, 0))
	assert.False(t, math.IsNaN(f))
	assert.Equal(t, int64(0), n.Int(nil).Int64())
	assert.Equal(t, int64(-1), n.NaN())

	n = new(Decimal).SetInf(false)
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.True(t, n.IsInf())
	assert.False(t, n.IsNaN())
	assert.False(t, n.IsQuietNaN())
	assert.False(t, n.IsSignalingNaN())
	assert.True(t, math.IsInf(f, 1))
	assert.False(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(-1), n.NaN())

	n = new(Decimal).SetInf(true)
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.True(t, n.IsInf())
	assert.False(t, n.IsNaN())
	assert.False(t, n.IsQuietNaN())
	assert.False(t, n.IsSignalingNaN())
	assert.True(t, math.IsInf(f, -1))
	assert.False(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(-1), n.NaN())

	n = new(Decimal).SetNaN()
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.False(t, n.IsInf())
	assert.True(t, n.IsNaN())
	assert.True(t, n.IsQuietNaN())
	assert.False(t, n.IsSignalingNaN())
	assert.False(t, math.IsInf(f, 0))
	assert.True(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(0x0_0000_0000_0000), n.NaN())

	n = new(Decimal).SetNaNPayload(false, false, 0)
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.False(t, n.IsInf())
	assert.True(t, n.IsNaN())
	assert.False(t, n.IsQuietNaN())
	assert.True(t, n.IsSignalingNaN())
	assert.False(t, math.IsInf(f, 0))
	assert.True(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(0x0_0000_0000_0000), n.NaN())

	n = new(Decimal).SetNaNPayload(false, false, math.MaxUint64)
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.False(t, n.IsInf())
	assert.True(t, n.IsNaN())
	assert.False(t, n.IsQuietNaN())
	assert.True(t, n.IsSignalingNaN())
	assert.False(t, math.IsInf(f, 0))
	assert.True(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(0x7_ffff_ffff_ffff), n.NaN())

	n = new(Decimal).SetNaNPayload(false, true, math.MaxUint64)
	f, _ = n.Float64()
	assert.False(t, n.IsFinite())
	assert.False(t, n.IsInf())
	assert.True(t, n.IsNaN())
	assert.True(t, n.IsQuietNaN())
	assert.False(t, n.IsSignalingNaN())
	assert.False(t, math.IsInf(f, 0))
	assert.True(t, math.IsNaN(f))
	assert.Nil(t, n.Int(nil))
	assert.Equal(t, int64(0x7_ffff_ffff_ffff), n.NaN())
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	// Each of these is checked against strconv.ParseFloat.
	inputs := []string{
		"1e23", "1.5e24", "1e-23", "0.00000000000000000000001", "123e21",
		"1e55", "1e-55", "1e87", "2.5e-310",
		"1e-30", "3.14159265358979e40", "1.1754942e-38",
		"1e22", "1e-22", "0.1", "9007199254740993e-5",

		// Base 2 mantissas wider than a float64 mantissa: halfway, halfway
		// with sticky bits, carries into the exponent, and overflow.
		"0x1.00000000000008p0", "0x1.00000000000018p0",
		"0x1.000000000000080000000000001p0", "0x1.fffffffffffff8p0",
		"0x1.fffffffffffff8p1023", "0x1.fffffffffffff7ffffffffp1023",
		"0x1.00000000000000000001p-1074", "0x1.fffffffffffffffffff8p-1075",
		"0x3.0000000000000000001p-1075", "0x1.00000000000008p-1022",
		"0x1.8p-1075", "0x3p-1075", "0x1p-1075",
	}
	// Sweep every power of ten with a few mantissas, to cover each entry of
	// the power-of-five tables.
	for _, mant := range []string{"1", "25", "15", "123456789", "9007199254740993"} {
		for exp := -330; exp <= 310; exp++ {
			inputs = append(inputs, fmt.Sprintf("%se%d", mant, exp))
		}
	}

	for _, s := range inputs {
		d, err := new(Decimal).Parse(s)
		require.NoError(t, err, s)

		got, _ := d.Float64()
		want, _ := strconv.ParseFloat(s, 64)
		assert.Equal(t, math.Float64bits(want), math.Float64bits(got), "%s: got %v, want %v", s, got, want)
	}
}

func TestFloat64Exact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in    string
		want  float64
		exact bool
	}{
		{in: "0.5", want: 0.5, exact: true},
		{in: "1.5e3", want: 1500, exact: true},
		{in: "390625e-8", want: 0x1p-8, exact: true},
		{in: "9007199254740992e1", want: 0x1.4p56, exact: true},
		{in: "0x1p-1074", want: 0x1p-1074, exact: true},
		{in: "0x1.0000000000001p-1022", want: 0x1.0000000000001p-1022, exact: true},

		{in: "0.1", want: 0.1},
		{in: "1e-5", want: 1e-5},
		{in: "1_000.000_1", want: 1000.0001},
		{in: "123456789012345e10", want: 123456789012345e10},
		{in: "9007199254740991e1", want: 9007199254740991e1},
		{in: "0x3p-1075", want: 0x1p-1073},
		{in: "0x1p-1075", want: 0},
		{in: "0x1.0000000000001p-1023", want: 0x1p-1023},
	}

	for _, tt := range tests {
		z, err := new(Decimal).Parse(tt.in)
		require.NoError(t, err, tt.in)

		got, exact := z.Float64()
		assert.Equal(t, math.Float64bits(tt.want), math.Float64bits(got), "%s: got %x, want %x", tt.in, got, tt.want)
		assert.Equal(t, tt.exact, exact, tt.in)
	}
}

func TestFloat64Int(t *testing.T) {
	t.Parallel()

	// Integers are stored with a base 2 exponent, so these do not go through
	// strconv.ParseFloat.
	tests := []struct {
		in    string
		want  float64
		exact bool
	}{
		{in: "18446744073709551615", want: 0x1p64},
		{in: "36893488147419103231", want: 0x1p65},               // 2^65 - 1
		{in: "36893488147419107328", want: 0x1p65},               // Tie, rounds to even.
		{in: "36893488147419107329", want: 0x1.0000000000001p65}, // Above tie.
		{in: "36893488147419115520", want: 0x1.0000000000002p65}, // Tie, rounds to even.
		{in: "36893488147419111424", want: 0x1.0000000000001p65, exact: true},
		{in: "1267650600228229401496703205376", want: 0x1p100, exact: true},
	}

	for _, tt := range tests {
		x, ok := new(big.Int).SetString(tt.in, 10)
		require.True(t, ok, tt.in)

		for _, neg := range []bool{false, true} {
			x := new(big.Int).Set(x)
			want := tt.want
			if neg {
				x.Neg(x)
				want = -want
			}

			got, exact := new(Decimal).ReuseInt(x).Float64()
			assert.Equal(t, math.Float64bits(want), math.Float64bits(got), "%v: got %x, want %x", x, got, want)
			assert.Equal(t, tt.exact, exact, x)
		}
	}
}

func TestFloat64Wide(t *testing.T) {
	// Not parallel: this measures allocation.

	// 2^1000 plus 2^947 is halfway between two float64 values. A single bit
	// far below decides the rounding, and so does an exact tie.
	tie := new(big.Int).Lsh(big.NewInt(1), 1000)
	tie.SetBit(tie, 947, 1)
	got, exact := new(Decimal).ReuseInt(tie).Float64()
	assert.Equal(t, math.Float64bits(0x1p1000), math.Float64bits(got))
	assert.False(t, exact)

	above := new(big.Int).SetBit(new(big.Int).Set(tie), 3, 1)
	got, exact = new(Decimal).ReuseInt(above).Float64()
	assert.Equal(t, math.Float64bits(0x1.0000000000001p1000), math.Float64bits(got))
	assert.False(t, exact)

	// Converting must not copy the mantissa: 100 calls on a 100 KB mantissa
	// would allocate 10 MB if it did.
	// The low bit is set so the mantissa keeps its full width.
	wideInt := new(big.Int).Lsh(big.NewInt(1), 800_000)
	wide := new(Decimal).ReuseInt(wideInt.SetBit(wideInt, 0, 1))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 100 {
		wide.Float64()
	}
	runtime.ReadMemStats(&after)
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(1<<20))
}
