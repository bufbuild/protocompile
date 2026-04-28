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

package cases

import (
	"iter"
	"unicode"
	"unicode/utf8"
)

// Words breaks up s into words according to the algorithm specified at
// https://docs.rs/heck/latest/heck/#definition-of-a-word-boundary.
func Words(str string) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := -1
		var prev rune

		for i := 0; i < len(str); {
			curr, size := utf8.DecodeRuneInString(str[i:])
			alnum := unicode.IsLetter(curr) || unicode.IsDigit(curr)

			switch {
			case !alnum:
				// Punctuation and spaces act as hard boundaries.
				// Yield the current word and reset the tracker.
				if start != -1 {
					if !yield(str[start:i]) {
						return
					}
					start = -1
				}

			case start == -1:
				// We found an alphanumeric character, start tracking a new word.
				start = i

			default:
				// We are inside a word. Look ahead to check for case-transition boundaries.
				var next rune
				if i+size < len(str) {
					next, _ = utf8.DecodeRuneInString(str[i+size:])
				}

				splitHere :=
					// Rule A: Transition from lowercase/digit to uppercase (e.g., camelCase -> camel, Case)
					(!unicode.IsUpper(prev) && unicode.IsUpper(curr)) ||
						// Rule B: Uppercase sequence followed by lowercase (e.g., XMLHttp -> XML, Http)
						(unicode.IsUpper(prev) && unicode.IsUpper(curr) && unicode.IsLower(next))

				if splitHere {
					if !yield(str[start:i]) {
						return
					}
					// The current uppercase character becomes the start of the next word.
					start = i
				}
			}

			prev = curr
			i += size
		}

		// Yield any remaining alphanumeric characters as the final word.
		if start != -1 {
			yield(str[start:])
		}
	}
}
