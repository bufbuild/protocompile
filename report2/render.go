// Copyright 2020-2024 Buf Technologies, Inc.
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

package report2

import (
	"fmt"
	"iter"
	"math/bits"
	"slices"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/internal/width"
)

// Render renders this diagnostic report in a format suitable for showing to a user.
func (r *Report) Render(style Style) string {
	var out strings.Builder
	var errors, warnings int
	for _, diagnostic := range *r {
		out.WriteString(diagnostic.Render(style))
		out.WriteString("\n")
		if style != Simple {
			out.WriteString("\n")
		}
		if diagnostic.Level == Error {
			errors++
		}
		if diagnostic.Level == Warning {
			warnings++
		}
	}
	if style == Simple {
		return out.String()
	}

	var color color
	if style == Colored {
		color = ansiColor()
	}

	pluralize := func(count int, what string) string {
		if count == 1 {
			return "1 " + what
		}
		return fmt.Sprint(count, " ", what, "s")
	}

	if errors > 0 {
		fmt.Fprint(&out, color.bRed, "encountered ", pluralize(errors, "error"))
		if warnings > 0 {
			fmt.Fprint(&out, " and ", pluralize(warnings, "warning"))
		}
		fmt.Fprintln(&out, color.reset)
	} else if warnings > 0 {
		fmt.Fprintln(&out, color.bYellow, "encountered ", pluralize(warnings, "warning"))
	}

	return out.String()
}

// Render renders this diagnostic in a format suitable for showing to a user.
func (d *Diagnostic) Render(style Style) string {
	var level string
	switch d.Level {
	case Error:
		level = "error"
	case Warning:
		level = "warning"
	case Remark:
		level = "remark"
	}

	// For the simply style, we imitate the Go compiler.
	if style == Simple {
		file, start, _ := d.Primary()
		if file.Path == "" {
			file.Path = "<unknown>"
		}
		if start.Line == 0 {
			return fmt.Sprintf("%s: %s: %s", level, file.Path, d.Err.Error())
		}
		return fmt.Sprintf("%s: %s:%d:%d: %s", level, file.Path, start.Line, start.Column, d.Err.Error())
	}

	// For the other styles, we imitate the Rust compiler. See
	// https://github.com/rust-lang/rustc-dev-guide/blob/master/src/diagnostics.md

	var color color
	if style == Colored {
		color = ansiColor()
	}

	var out strings.Builder
	fmt.Fprint(&out, color.BoldForLevel(d.Level), level, ": ", d.Err.Error(), color.reset)

	// Figure out how wide the line bar needs to be. This is given by
	// the width of the largest line value among the snippets.
	var greatestLine int
	for _, snip := range d.snippets {
		greatestLine = max(greatestLine, snip.end.Line)
	}
	lineBarWidth := len(fmt.Sprint(greatestLine)) // Easier than messing with math.Log10()
	lineBarWidth = max(2, lineBarWidth)

	// Render all the diagnostic windows.
	for i, snippets := range partition(d.snippets, func(a, b *snippet) bool { return a.file.Path != b.file.Path }) {
		out.WriteByte('\n')
		out.WriteString(color.nBlue)
		for range lineBarWidth {
			out.WriteRune(' ')
		}
		if i == 0 {
			primary := d.snippets[0]
			fmt.Fprintf(&out, "--> %s:%d:%d", primary.file.Path, primary.start.Line, primary.start.Column)
		} else {
			primary := snippets[0]
			fmt.Fprintf(&out, "::: %s:%d:%d", primary.file.Path, primary.start.Line, primary.start.Column)
		}

		// Add a blank line after the file. This gives the diagnostic window some
		// visual breathing room.
		out.WriteByte('\n')
		out.WriteString(color.nBlue)
		for range lineBarWidth {
			out.WriteRune(' ')
		}
		out.WriteString(" | ")

		window := buildWindow(d.Level, snippets)
		window.Render(lineBarWidth, &color, &out)
	}

	// Render a remedial file name for spanless errors.
	if len(d.snippets) == 0 {
		out.WriteByte('\n')
		out.WriteString(color.nBlue)
		for range lineBarWidth - 1 {
			out.WriteRune(' ')
		}

		path := d.mention
		if path == "" {
			path = "<unknown>"
		}
		fmt.Fprintf(&out, "--> %s:?:?", path)
	}

	// Render the footers. For simplicity we collect them into an array first.
	var footers [][2]string
	for _, note := range d.notes {
		footers = append(footers, [2]string{"note", note})
	}
	for _, help := range d.help {
		footers = append(footers, [2]string{"help", help})
	}
	for i, frame := range d.trace {
		if debugMode < debugFull && i > 0 {
			break
		}
		// Dump the stack trace for the diagnostic if one was included.
		footers = append(footers, [2]string{"debug", fmt.Sprintf("at %s", frame.Function)})
		footers = append(footers, [2]string{"debug", fmt.Sprintf("   %s:%d", frame.File, frame.Line)})
	}
	for _, footer := range footers {
		out.WriteByte('\n')
		out.WriteString(color.nBlue)
		for range lineBarWidth {
			out.WriteRune(' ')
		}
		out.WriteString(" = ")
		fmt.Fprint(&out, color.bCyan, footer[0], ": ", color.reset, footer[1])
	}

	return out.String()
}

// window is an intermediate structure for rendering an annotated code snippet
// consisting of multiple spans on the same file.
type window struct {
	file File
	// The line number at which the text starts in the overall source file.
	start int
	// The range this window's text occupies in the containing source File.
	offsets [2]int
	// A list of all underline elements in this window. Must be sorted
	// according to cmpUnderlines.
	underlines []underline
	multilines []multiline
}

// buildWindow builds a diagnostic window for the given snippets, which must all have
// the same file.
func buildWindow(level Level, snippets []snippet) *window {
	w := new(window)
	w.file = snippets[0].file

	// Calculate the range of the file we will be printing. This is given
	// by every line that has a piece of diagnostic in it. To find this, we
	// calculate the join of all of the spans in the window, and find the
	// nearest \n runes in the text.
	w.start = snippets[0].start.Line
	w.offsets[0] = snippets[0].start.Offset
	for _, snip := range snippets {
		w.start = min(w.start, snip.start.Line)
		w.offsets[0] = min(w.offsets[0], snip.start.Offset)
		w.offsets[1] = max(w.offsets[1], snip.end.Offset)
	}
	// Now, find the newlines before and after the given ranges, respectively.
	// This snaps the range to start immediately after a newline (or SOF) and
	// end immediately before a newline (or EOF).
	w.offsets[0] = strings.LastIndexByte(w.file.Text[:w.offsets[0]], '\n') + 1 // +1 gives the byte *after* the newline.
	if end := strings.IndexByte(w.file.Text[w.offsets[1]:], '\n'); end != -1 {
		w.offsets[1] += end
	} else {
		w.offsets[1] = len(w.file.Text)
	}

	// Now, convert each span into an underline or multiline.
	for _, snippet := range snippets {
		if snippet.start.Line != snippet.end.Line {
			w.multilines = append(w.multilines, multiline{
				start:   snippet.start.Line,
				end:     snippet.end.Line,
				width:   snippet.end.Column,
				level:   note,
				message: snippet.message,
			})
			ml := &w.multilines[len(w.multilines)-1]
			if snippet.primary {
				ml.level = level
			}
			continue
		}

		w.underlines = append(w.underlines, underline{
			line:    snippet.start.Line,
			start:   snippet.start.Column,
			end:     snippet.end.Column,
			level:   note,
			message: snippet.message,
		})

		ul := &w.underlines[len(w.underlines)-1]
		if snippet.primary {
			ul.level = level
		}

		// Make sure no empty underlines exist.
		if ul.Len() == 0 {
			ul.start++
		}
	}

	slices.SortFunc(w.underlines, cmpUnderlines)
	return w
}

func (w *window) Render(lineBarWidth int, color *color, out *strings.Builder) {
	type lineInfo struct {
		sidebar    []*multiline
		underlines []string
		shouldEmit bool
	}

	lines := strings.Split(w.file.Text[w.offsets[0]:w.offsets[1]], "\n")
	// Populate ancillary info for each line.
	info := make([]lineInfo, len(lines))

	// First, lay out the multilines, and compute how wide the sidebar is.
	for i := range w.multilines {
		multi := &w.multilines[i]
		// Find the smallest unused index by every line in the range.
		var bitset uint
		for i := multi.start; i <= multi.end; i++ {
			for i, ml := range info[i-w.start].sidebar {
				if ml != nil {
					bitset |= 1 << i
				}
			}
		}
		idx := bits.TrailingZeros(^bitset)

		// Apply the index to every element of sidebar.
		for i := multi.start; i <= multi.end; i++ {
			line := &info[i-w.start].sidebar
			for len(*line) < idx+1 {
				*line = append(*line, nil)
			}
			(*line)[idx] = multi
		}

		// Mark the start and end as must-emit.
		info[multi.start-w.start].shouldEmit = true
		info[multi.end-w.start].shouldEmit = true
	}
	var sidebarLen int
	for _, info := range info {
		sidebarLen = max(sidebarLen, len(info.sidebar))
	}

	// Next, we can render the underline parts. This aggregates all underlines
	// for the same line into rendered chunks
	for _, part := range partition(w.underlines, func(a, b *underline) bool { return a.line != b.line }) {
		cur := &info[part[0].line-w.start]
		cur.shouldEmit = true

		// Arrange for a "sidebar prefix" for this line. This is determined by any sidebars that are
		// active on this line, even if they end on it.
		var prefixB strings.Builder
		for _, ml := range cur.sidebar {
			prefixB.WriteString(color.BoldForLevel(ml.level))
			prefixB.WriteByte('|')
		}
		if prefixB.Len() > 0 {
			prefixB.WriteByte(' ')
		}
		prefix := prefixB.String()

		// Lay out the physical underlines in reverse order. This will cause longer lines to be
		// laid out first, which will be overwritten by shorter ones.
		var buf []byte
		for i := len(part) - 1; i >= 0; i-- {
			element := part[i]
			if len(buf) < element.end {
				newBuf := make([]byte, element.end)
				copy(newBuf, buf)
				buf = newBuf
			}

			// Note that start/end are 1-indexed.
			for j := element.start - 1; j < element.end-1; j++ {
				buf[j] = byte(element.level)
			}
		}

		// Now, convert the buffer into a proper string.
		var out strings.Builder
		for _, line := range partition(buf, func(a, b *byte) bool { return *a != *b }) {
			level := Level(line[0])
			if line[0] == 0 {
				out.WriteString(color.reset)
			} else {
				out.WriteString(color.BoldForLevel(level))
			}
			for range line {
				switch level {
				case 0:
					out.WriteByte(' ')
				case note, Remark:
					out.WriteByte('-')
				default:
					out.WriteByte('^')
				}
			}
		}

		// Next we need to find the message that goes inline with the underlines. This will be
		// the message belonging to the rightmost underline.
		var rightmost *underline
		for i := range part {
			ul := &part[i]
			if rightmost == nil || ul.end > rightmost.end {
				rightmost = ul
			}
		}
		underlines := strings.TrimRight(out.String(), " ")
		cur.underlines = []string{prefix + underlines + " " + color.BoldForLevel(rightmost.level) + rightmost.message}

		// Now, do all the other messages, one per line. For each message, we also
		// need to draw pipes (|) above each one to connect it to its underline.
		//
		// This is slightly complicated, because there are two layers: the pipes, and
		// whatever message goes on the pipes.
		var rest []*underline
		for i := range part {
			ul := &part[i]
			if ul == rightmost || ul.message == "" {
				continue
			}
			rest = append(rest, ul)
		}
		for idx := range rest {
			buf = buf[:0] // Clear the temp buffer.

			// First, lay out the pipes.
			var nonColorLen int
			for _, ul := range rest[idx:] {
				idx := ul.start - 1
				for nonColorLen < idx {
					buf = append(buf, ' ')
					nonColorLen++
				}

				if nonColorLen == idx {
					// Two pipes may appear on the same column!
					// This is why this is in a conditional.
					buf = append(buf, color.BoldForLevel(ul.level)...)
					buf = append(buf, '|')
					nonColorLen++
				}
			}

			// Spat in the one with all the pipes in it as-is.
			cur.underlines = append(cur.underlines, strings.TrimRight(prefix+string(buf), " "))

			// Then, splat in the message. having two rows like this ensures that
			// each message has one pipe directly above it.
			if idx >= 0 {
				ul := rest[idx]

				actualStart := ul.start - 1
				for _, other := range rest[idx:] {
					if other.start <= ul.start {
						actualStart += len(color.BoldForLevel(ul.level))
					}
				}
				// FIXME: This assumes that ul.message does not contain wide characters.
				for len(buf) < actualStart+len(ul.message)+1 {
					buf = append(buf, ' ')
				}

				copy(buf[actualStart:], ul.message)
			}
			cur.underlines = append(cur.underlines, strings.TrimRight(prefix+string(buf), " "))
		}
	}

	// Now that we've laid out the underlines, we can add the ends of all of the multilines.
	// The result is that a multiline will look like this:
	//
	// / code
	// | code code code
	// |______________^ message
	for i := range info {
		cur := &info[i]
		var line strings.Builder
		for j, ml := range cur.sidebar {
			line.Reset()
			if ml.end-w.start != i {
				continue
			}

			for _, ml := range cur.sidebar[:j+1] {
				if ml == nil {
					line.WriteByte(' ')
					continue
				}
				line.WriteString(color.BoldForLevel(ml.level))
				line.WriteByte('|')
			}

			line.WriteString(color.BoldForLevel(ml.level))
			for range len(cur.sidebar) - j - 1 {
				line.WriteByte('_')
			}
			for range ml.width {
				line.WriteByte('_')
			}

			if ml.level == note {
				line.WriteString("^ ")
			} else {
				line.WriteString("- ")
			}
			line.WriteString(ml.message)
			cur.underlines = append(cur.underlines, line.String())
		}
	}

	// Make sure to emit any lines adjacent to another line we want to emit, so long as that
	// line contains printable characters.
	for i := range info {
		cur := &info[i]

		containsPrintable := func(s string) bool {
			for _, r := range s {
				if unicode.IsGraphic(r) {
					return true
				}
			}
			return false
		}

		if i != 0 && info[i-1].shouldEmit && containsPrintable(lines[i]) {
			cur.shouldEmit = true
		} else if i+1 < len(info) && info[i+1].shouldEmit && containsPrintable(lines[i]) {
			cur.shouldEmit = true
		}
	}

	for i, line := range lines {
		cur := &info[i]
		lineno := i + w.start

		var sidebar strings.Builder
		for _, ml := range cur.sidebar {
			sidebar.WriteString(color.BoldForLevel(ml.level))
			if lineno == ml.start {
				sidebar.WriteByte('/')
			} else {
				sidebar.WriteByte('|')
			}
		}
		if sidebar.Len() > 0 {
			sidebar.WriteByte(' ')
		}

		if !cur.shouldEmit {
			continue
		}

		if i > 0 && !info[i-1].shouldEmit {
			// Generate a visual break if this is right after a real line.
			out.WriteByte('\n')
			out.WriteString(color.bBlue)
			for range lineBarWidth {
				out.WriteByte(' ')
			}
			out.WriteString(" ~ ")
			out.WriteString(sidebar.String())
		}

		// Ok, we are definitely printing this line out.
		fmt.Fprintf(out, "\n%s%*d | %s%s", color.nBlue, lineBarWidth, lineno, sidebar.String(), color.reset)

		// Print out runes one by one, so we account for tabs correctly.
		var ruler width.Ruler
		for _, r := range line {
			if r == '\t' {
				out.WriteByte(' ')
				for ruler.Measure(' ')%TabstopWidth != 0 {
					out.WriteByte(' ')
				}
			} else {
				ruler.Measure(r)
				out.WriteRune(r)
			}
		}

		// If this happens to be an annotated line, this is when it gets annotated.
		for _, line := range cur.underlines {
			out.WriteByte('\n')
			out.WriteString(color.bBlue)
			for range lineBarWidth {
				out.WriteByte(' ')
			}
			out.WriteString(" | ")
			out.WriteString(line)
		}
	}
}

type underline struct {
	line       int
	start, end int
	level      Level
	message    string
}

func (u underline) Len() int {
	return u.end - u.start
}

func cmpUnderlines(a, b underline) int {
	if diff := a.line - b.line; diff != 0 {
		return diff
	}
	if diff := a.level - b.level; diff != 0 {
		return int(diff)
	}
	if diff := a.Len() - b.Len(); diff != 0 {
		return diff
	}
	return a.start - b.start
}

type multiline struct {
	start, end int
	width      int
	level      Level
	message    string
}

// color is the colors used for pretty-rendering diagnostics.
type color struct {
	reset string
	// Normal colors.
	nRed, nYellow, nCyan, nBlue string
	// Bold colors.
	bRed, bYellow, bCyan, bBlue string
}

func ansiColor() color {
	return color{
		reset:   "\033[0m",
		nRed:    "\033[0;31m",
		nYellow: "\033[0;33m",
		nCyan:   "\033[0;36m",
		nBlue:   "\033[0;34m",
		bRed:    "\033[1;31m",
		bYellow: "\033[1;33m",
		bCyan:   "\033[1;36m",
		bBlue:   "\033[1;34m",
	}
}

func (c color) ColorForLevel(l Level) string {
	switch l {
	case Error:
		return c.nRed
	case Warning:
		return c.nYellow
	case Remark:
		return c.nCyan
	case note:
		return c.nBlue
	default:
		return ""
	}
}

func (c color) BoldForLevel(l Level) string {
	switch l {
	case Error:
		return c.bRed
	case Warning:
		return c.bYellow
	case Remark:
		return c.bCyan
	case note:
		return c.bBlue
	default:
		return ""
	}
}

// partition returns an iterator of subslices of s such that each yielded
// slice is delimited according to delimit. Also yields the starting index of
// the subslice.
//
// In other words, suppose delimit is !=. Then, the slice [a a a b c c] is yielded
// as the subslices [a a a], [b], and [c c c].
//
// Will never yield an empty slice.
func partition[T any](s []T, delimit func(a, b *T) bool) iter.Seq2[int, []T] {
	return func(yield func(int, []T) bool) {
		var start int
		for i := 1; i < len(s); i++ {
			if delimit(&s[i-1], &s[i]) {
				if !yield(start, s[start:i]) {
					break
				}
				start = i
			}
		}
		rest := s[start:]
		if len(rest) > 0 {
			yield(start, rest)
		}
	}
}
