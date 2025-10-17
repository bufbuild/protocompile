package erredition

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	whitespacePattern = regexp.MustCompile(`[ \t\r\n]+`)
	protoDevPattern   = regexp.MustCompile(` See http:\/\/protobuf\.dev\/[^ ]+ for more information\.?`)
	periodPattern     = regexp.MustCompile(`\.( [A-Z]|$)`)
	editionPattern    = regexp.MustCompile(`edition [0-9]+`)
)

// normalizeReason canonicalizes the appearance of deprecation reasons.
// Some built-in deprecation warnings have double spaces after periods.
func normalizeReason(text string) string {
	// First, normalize all whitespace.
	text = whitespacePattern.ReplaceAllString(text, " ")

	// Delete protobuf.dev links; these should ideally use our specialized link
	// formatting instead.
	text = protoDevPattern.ReplaceAllString(text, "")

	// Replace all sentence-ending periods with semicolons.
	text = periodPattern.ReplaceAllStringFunc(text, func(match string) string {
		if match == "." {
			return ""
		}
		return ";" + strings.ToLower(match[1:])
	})

	// Capitalize "Edition" when followed by an edition number.
	text = editionPattern.ReplaceAllStringFunc(text, func(s string) string {
		return "E" + s[1:]
	})

	// Finally, de-capitalize the first letter.
	r, n := utf8.DecodeRuneInString(text)
	return string(unicode.ToLower(r)) + text[n:]
}
