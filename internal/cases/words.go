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

		var prev rune
		first := true
		for str != "" {
			next, n := utf8.DecodeRuneInString(str)
			str = str[n:]

			switch {
			case !unicode.IsLetter(next) && !unicode.IsDigit(next):
				// This is punctuation. Split the string around next and
				// yield the result if it's nonempty.
				word := input[:len(input)-len(str)-n]
				input = input[len(input)-len(str):]
				if word != "" && !yield(word) {
					return
				}

			case unicode.IsUpper(prev) && unicode.IsLower(next):
				// If the previous rune is uppercase and the next is lowercase,
				// we want to insert a boundary before prev.
				idx := len(input) - len(str) - n - utf8.RuneLen(prev)

				word := input[:idx]
				input = input[idx:]
				if word != "" && !yield(word) {
					return
				}

			case str == "":
				if first { // Single-rune string.
					yield(input)
					return
				}

				// This is the last rune, which gets special handling. We want
				// FooBAR and FooBar to become foo_bar but FooX to become foo_x.
				// Hence, if next is uppercase and prev is not, then we insert a
				// boundary between them.

				if !unicode.IsUpper(prev) && unicode.IsUpper(next) {
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
			first = false
		}
	}
}
