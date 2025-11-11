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
