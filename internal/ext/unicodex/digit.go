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

package unicodex

var hexTable = func() [128]byte {
	var table [128]byte
	for d := range len(table) {
		d := byte(d)
		var v byte
		switch {
		case d >= '0' && d <= '9':
			v = d - '0'

		case d >= 'a' && d <= 'z':
			v = d - 'a' + 10

		case d >= 'A' && d <= 'Z':
			v = d - 'A' + 10

		default:
			v = 0xff
		}

		table[d] = v - d
	}
	return table
}()

// Digit parses a digit in the given base, up to base 36.
func Digit(d rune, base byte) (value byte, ok bool) {
	if d > 0x7f {
		return 0xff, false
	}
	value = byte(d) + hexTable[d]
	return value, value < base
}
