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
		input := str // Not yet yielded.

		// wordMode tracks the case of the most recent cased (letter) rune,
		// mirroring heck's WordMode. Digits inherit the previous mode rather
		// than resetting it, so "abc123" leaves the mode as modeLowercase and
		// a following uppercase letter still triggers a boundary.
		type wordMode int
		const (
			modeBoundary  wordMode = iota // no cased rune seen since last boundary
			modeLowercase                 // last cased rune was lowercase
			modeUppercase                 // last cased rune was uppercase
		)

		var prev rune
		var mode wordMode
		for str != "" {
			next, n := utf8.DecodeRuneInString(str)
			str = str[n:]

			switch {
			case !unicode.IsLetter(next) && !unicode.IsDigit(next):
				// Punctuation: split around next, yield if nonempty.
				word := input[:len(input)-len(str)-n]
				input = input[len(input)-len(str):]
				if word != "" && !yield(word) {
					return
				}

			case mode == modeLowercase && unicode.IsUpper(next) && str != "":
				// Boundary before the uppercase rune.
				idx := len(input) - len(str) - n
				word := input[:idx]
				input = input[idx:]
				if word != "" && !yield(word) {
					return
				}

			case unicode.IsUpper(prev) && unicode.IsLower(next):
				// Boundary before prev (last of a run of uppercase letters).
				idx := len(input) - len(str) - n - utf8.RuneLen(prev)
				word := input[:idx]
				input = input[idx:]
				if word != "" && !yield(word) {
					return
				}
				// If next was also the last rune, yield the remaining word and
				// return. Without this, the last-rune case below never fires
				// for 2-char [A-Z][a-z] strings (e.g. "Ab"), because the loop
				// exits after this iteration without yielding input.
				if str == "" {
					yield(input)
					return
				}

			case str == "":
				// Last rune. We want FooBAR and FooBar → foo_bar, FooX → foo_x:
				// insert a boundary before a trailing uppercase only when the
				// preceding cased rune was lowercase.
				if mode == modeLowercase && unicode.IsUpper(next) {
					idx := len(input) - len(str) - n
					word := input[:idx]
					input = input[idx:]
					if word != "" && !yield(word) {
						return
					}
				}
				yield(input)
				return
			}

			prev = next
			switch {
			case unicode.IsLower(next):
				mode = modeLowercase
			case unicode.IsUpper(next):
				mode = modeUppercase
			case !unicode.IsDigit(next):
				mode = modeBoundary // non-alphanumeric: reset for next word
			}
			// Digits: mode unchanged (inherited from last cased rune).
		}
	}
}
