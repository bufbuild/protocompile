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

package report

import (
	"bytes"
	"io"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

// writer implements low-level writing helpers, including a custom buffering
// routine to avoid printing trailing whitespace to the output.
type writer struct {
	out io.Writer
	buf []byte
	err error
}

// Write implements [io.Writer].
func (w *writer) Write(data []byte) (int, error) {
	w.WriteString(unsafex.StringAlias(data))
	return len(data), nil
}

func (w *writer) WriteSpaces(n int) {
	const spaces = "                                        "
	for n > len(spaces) {
		w.buf = append(w.buf, spaces...)
		n -= len(spaces)
	}
	w.buf = append(w.buf, spaces[:n]...)
}

func (w *writer) WriteString(data string) {
	// Break the input along newlines; each time we're about to append a
	// newline, discard all trailing whitespace that isn't a newline.
	for {
		nl := strings.IndexByte(data, '\n')
		if nl < 0 {
			w.buf = append(w.buf, data...)
			return
		}

		line := data[:nl]
		data = data[nl+1:]
		w.buf = append(w.buf, line...)
		w.flush(true)
	}
}

// Flush flushes the buffer to the writer's output.
func (w *writer) Flush() error {
	w.flush(false)
	err := w.err
	w.err = nil
	return err
}

// flush is like [writer.Flush], but instead retains the error to be returned
// out of Flush later. This allows e.g. WriteString to call flush() without
// needing to return an error and complicating the rendering code.
func (w *writer) flush(withNewline bool) {
	orig := w.buf
	if w.err == nil {
		w.buf = bytes.TrimRightFunc(w.buf, unicode.IsSpace)
		if withNewline {
			w.buf = append(w.buf, '\n')
		}

		// NOTE: The contract for Write requires that it return len(buf) when
		// the error is nil. This means that the length return only matters if
		// we hit an error condition, which we treat as fatal anyways.
		_, w.err = w.out.Write(w.buf)
	}

	if withNewline {
		w.buf = w.buf[:0]
		return
	}

	// This trick is used in slices.Delete.
	w.buf = append(orig[:0], orig[len(w.buf):]...) //nolint:gocritic // Assigning to a different slice is on purpose!
}

// plural is a helper for printing out plurals of numbers.
type plural int

// String implements [fmt.Stringer].
func (p plural) String() string {
	if p == 1 {
		return ""
	}
	return "s"
}
