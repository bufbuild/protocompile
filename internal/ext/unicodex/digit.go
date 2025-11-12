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

// Digit parses a digit in the given base, up to base 36.
func Digit(d rune, base byte) (value byte, ok bool) {
	switch {
	case d >= '0' && d <= '9':
		value = byte(d) - '0'

	case d >= 'a' && d <= 'z':
		value = byte(d) - 'a' + 10

	case d >= 'A' && d <= 'Z':
		value = byte(d) - 'A' + 10

	default:
		value = 0xff
	}

	if value >= base {
		return 0, false
	}
	return value, true
}
