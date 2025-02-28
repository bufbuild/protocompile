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
	"fmt"
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

// wordWrap returns an iterator over chunks of s that are no wider than width,
// which can be printed as their own lines.
func wordWrap(text string, width int) iter.Seq[string] {
	return func(yield func(string) bool) {
		// Split along lines first, since those are hard breaks we don't plan
		// to change.
		for line := range stringsx.Lines(text) {
			var nextIsSpace bool
			var column, cursor int

			for start, chunk := range stringsx.PartitionKey(line, unicode.IsSpace) {
				isSpace := nextIsSpace
				nextIsSpace = !nextIsSpace

				if isSpace && column == 0 {
					continue
				}

				w := stringWidth(column, chunk, true, nil) - column
				if column+w <= width {
					column += w
					continue
				}

				if !yield(strings.TrimSpace(line[cursor:start])) {
					return
				}

				if isSpace {
					cursor = start + len(chunk)
					column = 0
				} else {
					cursor = start
					column = w
				}
			}

			rest := line[cursor:]
			if rest != "" && !yield(rest) {
				return
			}
		}
	}
}

// stringWidth calculates the rendered width of text if placed at the given column,
// accounting for tabstops.
func stringWidth(column int, text string, allowNonPrint bool, out *writer) int {
	// We can't just use StringWidth, because that doesn't respect tabstops
	// correctly.
	for i, next := range iterx.Enumerate(stringsx.Split(text, '\t')) {
		if i > 0 {
			tab := TabstopWidth - (column % TabstopWidth)
			column += tab
			if out != nil {
				out.WriteSpaces(tab)
			}
		}

		if !allowNonPrint {
			// Handle unprintable characters. We render those as <U+NNNN>.
			for next != "" {
				pos, nextNonPrint, nonPrint := iterx.Find2(stringsx.Runes(next), func(_ int, r rune) bool {
					return r == -1 || NonPrint(r)
				})

				chunk := next
				if pos != -1 {
					chunk, next = next[:nextNonPrint], next[nextNonPrint:]

					var escape string
					if nonPrint == -1 {
						escape = fmt.Sprintf("<%02X>", next[0])
						next = next[1:]
					} else {
						escape = fmt.Sprintf("<U+%04X>", nonPrint)
						next = next[utf8.RuneLen(nonPrint):]
					}

					if out != nil {
						out.WriteString(chunk)
						out.WriteString(escape)
					}
					column += uniseg.StringWidth(chunk) + len(escape)
				} else {
					if out != nil {
						out.WriteString(chunk)
					}
					column += uniseg.StringWidth(chunk)
					next = ""
				}
			}
		} else {
			column += uniseg.StringWidth(next)
			if out != nil {
				out.WriteString(next)
			}
		}
	}

	return column
}
