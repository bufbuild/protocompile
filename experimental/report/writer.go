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
}

// Write implements [io.Writer].
func (w *writer) Write(data []byte) (int, error) {
	w.WriteString(unsafex.StringAlias(data))
	return len(data), nil
}

func (w *writer) WriteBytes(b byte, n int) {
	for i := 0; i < n; i++ {
		w.buf = append(w.buf, b)
	}
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
		w.buf = bytes.TrimRightFunc(w.buf, func(r rune) bool {
			return r != '\n' && unicode.IsSpace(r)
		})
		w.buf = append(w.buf, '\n')
	}
}

// Flush flushes the buffer to the writer's output.
func (w *writer) Flush() error {
	rest := bytes.TrimRight(w.buf, " ")
	n, err := w.out.Write(rest)

	copy(w.buf, w.buf[n:])
	w.buf = w.buf[:len(w.buf)-n]

	return err
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
