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
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/bufbuild/protocompile/internal/iter"
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
		stringsx.Lines(text)(func(line string) bool {
			var nextIsSpace bool
			var column, cursor int

			stringsx.PartitionKey(line, unicode.IsSpace)(func(start int, chunk string) bool {
				isSpace := nextIsSpace
				nextIsSpace = !nextIsSpace

				if isSpace && column == 0 {
					return true
				}

				w := stringWidth(column, chunk, true, nil) - column
				if column+w <= width {
					column += w
					return true
				}

				if !yield(strings.TrimSpace(line[cursor:start])) {
					return false
				}

				if isSpace {
					cursor = start + len(chunk)
					column = 0
				} else {
					cursor = start
					column = w
				}
				return true
			})

			rest := line[cursor:]
			return rest == "" || yield(rest)
		})
	}
}

// stringWidth calculates the rendered width of text if placed at the given column,
// accounting for tabstops.
func stringWidth(column int, text string, allowNonPrint bool, out *writer) int {
	// We can't just use StringWidth, because that doesn't respect tabstops
	// correctly.
	for text != "" {
		nextTab := strings.IndexByte(text, '\t')
		haveTab := nextTab != -1
		next := text
		if haveTab {
			next, text = text[:nextTab], text[nextTab+1:]
		} else {
			text = ""
		}

		if !allowNonPrint {
			// Handle unprintable characters. We render those as <U+NNNN>.
			for next != "" {
				nextNonPrint := strings.IndexFunc(next, NonPrint)
				chunk := next
				if nextNonPrint != -1 {
					chunk, next = next[:nextNonPrint], next[nextNonPrint:]
					nonPrint, runeLen := utf8.DecodeRuneInString(next)
					next = next[runeLen:]

					escape := fmt.Sprintf("<U+%04X>", nonPrint)
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

		if haveTab {
			tab := TabstopWidth - (column % TabstopWidth)
			column += tab
			if out != nil {
				out.WriteSpaces(tab)
			}
		}
	}
	return column
}
