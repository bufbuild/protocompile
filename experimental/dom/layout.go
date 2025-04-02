package dom

import (
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
	"github.com/rivo/uniseg"
)

type layout struct {
	Options

	indent []int
	column int

	prevText *tag
}

func (l *layout) do(doc doc) {
	l.flat(doc.cursor())
	l.prevText = nil

	l.broken(doc.cursor())
}

// flat calculates the potential flat width of an element.
func (l *layout) flat(cursor cursor) (total int, broken bool) {
	for tag, cursor := range cursor {
		switch tag.kind {
		case kindText, kindSpace, kindBreak:
			if l.prevText != nil {
				prev, next := shouldMerge(l.prevText, tag)
				if !prev {
					total -= l.prevText.width
					l.prevText = nil
				} else if !next {
					continue
				}
			}

			tag.broken = strings.Contains(tag.text, "\n")

			// With tabs, we need to be pessimistic, because we don't
			// know whether groups are broken yet.
			tag.width = stringWidth(l.Options, -1, tag.text)

			if tag.check(Flat) {
				l.prevText = tag
			}
		}

		n, br := l.flat(cursor)
		tag.width += n
		tag.broken = tag.broken || br

		if tag.check(Flat) {
			total += tag.width
			broken = broken || tag.broken
		}
	}
	return total, broken
}

// broken calculates the layout of a group we have decided to break.
func (l *layout) broken(cursor cursor) {
	for tag, cursor := range cursor {
		if !tag.check(Broken) {
			continue
		}

		tag.column = l.column

		switch tag.kind {
		case kindText, kindSpace, kindBreak:
			if l.prevText != nil {
				prev, next := shouldMerge(l.prevText, tag)
				if !prev {
					if !l.prevText.broken {
						l.column -= l.prevText.width
					}
					l.prevText = nil
				} else if !next {
					continue
				}
			}

			if l.column == 0 {
				l.column, _ = slicesx.Last(l.indent)
			}

			last := stringsx.LastLine(tag.text)
			if len(last) < len(tag.text) {
				l.column = 0
			}
			l.column = stringWidth(l.Options, l.column, last)

		case kindGroup:
			// This enforces that groups break if:
			//
			// 1. The would cause overflow of the global max width.
			//
			// 2. The group itself is too wide.
			tag.broken = tag.broken ||
				tag.column+tag.width > l.MaxWidth ||
				tag.width > tag.limit

			if !tag.broken {
				// No need to recurse; we are leaving this group unbroken.
				l.column += tag.width
			} else {
				l.broken(cursor)
			}

		case kindIndent:
			prev, _ := slicesx.Last(l.indent)
			next := stringWidth(l.Options, prev, tag.text)
			l.indent = append(l.indent, next)
			l.broken(cursor)
			l.indent = l.indent[:len(l.indent)-1]

		case kindUnindent:
			prev, ok := slicesx.Pop(&l.indent)
			l.broken(cursor)
			if ok {
				l.indent = append(l.indent, prev)
			}
		}
	}
}

// stringWidth calculates the rendered width of text if placed at the given column,
// accounting for tabstops.
//
// If column is -1, all tabstops are given their maximum width.
func stringWidth(options Options, column int, text string) int {
	maxWidth := column < 0
	column = max(0, column)

	// We can't just use StringWidth, because that doesn't respect tabstops
	// correctly.
	for i, next := range iterx.Enumerate(stringsx.Split(text, '\t')) {
		if i > 0 {
			tab := options.TabstopWidth
			if !maxWidth {
				tab -= (column % options.TabstopWidth)
			}
			column += tab
		}
		column += uniseg.StringWidth(next)
	}

	return column
}
