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

package bytesx

import (
	"slices"
	"unicode/utf8"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

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

// Insert inserts the given buffer into the given offset, shifting everything
// after it over if needed.
func (w *Writer) Insert(n int, p []byte) {
	*w = slices.Insert(*w, n, p...)
}

// WriteString appends the contents of s to the buffer, growing the buffer as
// needed. The return value n is the length of s; err is always nil.
func (w *Writer) WriteString(s string) (n int, err error) {
	*w = append(*w, s...)
	return len(s), nil
}

// InsertString is like [Writer.Insert], but takes a string instead.
func (w *Writer) InsertString(n int, p string) {
	*w = slices.Insert(*w, n, unsafex.BytesAlias[[]byte](p)...)
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
		return 1, w.WriteByte(byte(r))
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
