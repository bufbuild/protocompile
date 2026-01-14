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

import (
	"fmt"
	"io"
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

const (
	// TabstopWidth is the size we render all tabstops as.
	TabstopWidth int = 4
	// MaxMessageWidth is the maximum width of a diagnostic message before it is
	// word-wrapped, to try to keep everything within the bounds of a terminal.
	MaxMessageWidth int = 80
)

// NonPrint defines whether or not a rune is considered "unprintable for the
// purposes of diagnostics", that is, whether it is a rune that the diagnostics
// engine will replace with <U+NNNN> when printing.
func NonPrint(r rune) bool {
	return !strings.ContainsRune(" \r\t\n", r) && !unicode.IsPrint(r)
}

// Width is used for calculating the approximate width of a string in terminal
// columns.
type Width struct {
	// The column at which the text is being rendered. This is necessary for
	// tabstop calculations.
	Column int

	// The width of a tabstop in columns. If set to zero, a default value will
	// be selected.
	Tabstop int

	// If set, non-printable characters are escaped in the format <U+NNNN>.
	EscapeNonPrint bool

	// If non-nil, text will be output to this function, converting tabs to
	// spaces and escaping unprintables as requested.
	Out io.StringWriter
}

// WriteString writes the given text, advancing w.Column and writing to w.Out.
func (w *Width) WriteString(text string) (int, error) {
	// We can't just use StringWidth, because that doesn't respect tabstops
	// correctly.
	n := 0
	write := func(s string) error {
		if w.Out != nil {
			m, err := w.Out.WriteString(s)
			n += m
			return err
		}
		return nil
	}

	tabstop := w.Tabstop
	if tabstop <= 0 {
		tabstop = TabstopWidth
	}

	for i, next := range iterx.Enumerate(stringsx.Split(text, '\t')) {
		if i > 0 {
			tab := tabstop - (w.Column % tabstop)
			w.Column += tab

			// Repeat(" ", n) will typically not allocate.
			spaces := strings.Repeat(" ", tab)
			if err := write(spaces); err != nil {
				return n, err
			}
		}

		if !w.EscapeNonPrint {
			w.Column += uniseg.StringWidth(next)
			if err := write(next); err != nil {
				return n, err
			}

			continue
		}

		// Handle unprintable characters. We render those as <U+NNNN>.
		for next != "" {
			pos, nextNonPrint, nonPrint := iterx.Find2(stringsx.Runes(next), func(_ int, r rune) bool {
				return r == -1 || NonPrint(r)
			})

			if pos == -1 {
				w.Column += uniseg.StringWidth(next)
				if err := write(next); err != nil {
					return n, err
				}

				break
			}

			var chunk string
			chunk, next = next[:nextNonPrint], next[nextNonPrint:]

			var escape string
			if nonPrint == -1 {
				escape = fmt.Sprintf("<%02X>", next[0])
				next = next[1:]
			} else {
				escape = fmt.Sprintf("<U+%04X>", nonPrint)
				next = next[utf8.RuneLen(nonPrint):]
			}

			w.Column += uniseg.StringWidth(chunk) + len(escape)
			if err := write(chunk); err != nil {
				return n, err
			}
			if err := write(escape); err != nil {
				return n, err
			}
		}
	}

	return n, nil
}

// WordWrap returns an iterator over chunks of s that are no wider than maxWidth,
// which can be printed as their own lines.
func (w *Width) WordWrap(text string, maxWidth int) iter.Seq[string] {
	return func(yield func(string) bool) {
		// Split along lines first, since those are hard breaks we don't plan
		// to change.
		for line := range stringsx.Lines(text) {
			w.Column = 0
			var nextIsSpace bool
			var cursor int

			for start, chunk := range stringsx.PartitionKey(line, unicode.IsSpace) {
				isSpace := nextIsSpace
				nextIsSpace = !nextIsSpace

				if isSpace && w.Column == 0 {
					continue
				}

				_, _ = w.WriteString(chunk)
				if w.Column <= maxWidth {
					continue
				}

				if !yield(strings.TrimSpace(line[cursor:start])) {
					return
				}

				w.Column = 0
				if isSpace {
					cursor = start + len(chunk)
				} else {
					cursor = start
					_, _ = w.WriteString(chunk)
				}
			}

			rest := line[cursor:]
			if rest != "" && !yield(rest) {
				return
			}
		}
	}
}
