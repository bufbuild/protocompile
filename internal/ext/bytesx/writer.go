package bytesx

import "unicode/utf8"

// Writer is like [bytes.Buffer], but only provides writing operations and
// only appends directly to the buffer.
//
// This is essentially just a byte slice that cna also be passed into
// [fmt.Fprintf].
type Writer []byte

// Write appends the contents of p to the buffer, growing the buffer as
// needed. The return value n is the length of p; err is always nil.
func (w *Writer) Write(p []byte) (n int, err error) {
	*w = append(*w, p...)
	return len(p), nil
}

// WriteString appends the contents of s to the buffer, growing the buffer as
// needed. The return value n is the length of s; err is always nil.
func (w *Writer) WriteString(s string) (n int, err error) {
	*w = append(*w, s...)
	return len(s), nil
}

// WriteByte appends the byte c to the buffer, growing the buffer as needed.
// The returned error is always nil, but is included to match [bufio.Writer]'s
// WriteByte.
func (w *Writer) WriteByte(c byte) error {
	*w = append(*w, c)
	return nil
}

// WriteRune appends the UTF-8 encoding of Unicode code point r to the
// buffer, returning its length and an error, which is always nil but is
// included to match [bufio.Writer]'s WriteRune.
func (w *Writer) WriteRune(r rune) (n int, err error) {
	if r < 0x80 {
		w.WriteByte(byte(r))
		return 1, nil
	}

	var buf [4]byte
	n = utf8.EncodeRune(buf[:], r)
	return w.Write(buf[:n])
}

// Reset resets the buffer to be empty, but it retains the underlying storage
// for use by future writes.
func (w *Writer) Reset() {
	*w = (*w)[:0]
}
