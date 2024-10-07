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

package report

import (
	"bytes"
	"fmt"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

// Renderer configures a diagnostic rendering operation.
type Renderer struct {
	// If set, uses a compact one-line format for each diagnostic.
	Austere bool

	// If set, rendering results are enriched with ANSI color escapes.
	Colorize bool

	// Upgrades all warnings to errors.
	WarningsAreErrors bool

	// If set, remark diagnostics will be printed.
	//
	// Ignored by [Renderer.RenderDiagnostic].
	ShowRemarks bool

	// If set, rendering a diagnostic will show the debug footer.
	ShowDebug bool
}

// Render renders a diagnostic report.
//
// In addition to returning the rendering result, returns whether any
// errors occurred.
func (r Renderer) Render(report *Report) (text string, haveErrors bool) {
	var out strings.Builder
	var errors, warnings int
	for _, diagnostic := range report.Diagnostics {
		if !r.ShowRemarks && diagnostic.Level == Remark {
			continue
		}

		out.WriteString(r.Diagnostic(diagnostic))
		out.WriteString("\n")
		if !r.Austere {
			out.WriteString("\n")
		}
		if diagnostic.Level == Error {
			errors++
		}
		if diagnostic.Level == Warning {
			if r.WarningsAreErrors {
				errors++
			} else {
				warnings++
			}
		}
	}
	if r.Austere {
		return out.String(), errors > 0
	}

	c := r.colors()

	pluralize := func(count int, what string) string {
		if count == 1 {
			return "1 " + what
		}
		return fmt.Sprint(count, " ", what, "s")
	}

	if errors > 0 {
		fmt.Fprint(&out, c.bRed, "encountered ", pluralize(errors, "error"))
		if warnings > 0 {
			fmt.Fprint(&out, " and ", pluralize(warnings, "warning"))
		}
		fmt.Fprintln(&out, c.reset)
	} else if warnings > 0 {
		fmt.Fprintln(&out, c.bYellow, "encountered ", pluralize(warnings, "warning"))
	}

	out.WriteString(c.reset)
	return out.String(), errors > 0
}

// Diagnostic renders a single diagnostic to a string.
func (r *Renderer) Diagnostic(d Diagnostic) string {
	var level string
	switch d.Level {
	case Error:
		level = "error"
	case Warning:
		if r.WarningsAreErrors {
			level = "error"
		} else {
			level = "warning"
		}
	case Remark:
		level = "remark"
	}

	c := r.colors()

	// For the simple style, we imitate the Go compiler.
	if r.Austere {
		annotation := d.Primary()

		if annotation.Start.Line == 0 {
			if annotation.File.Path == "" {
				return fmt.Sprintf(
					"%s%s: %s%s",
					c.ColorForLevel(d.Level),
					level,
					d.Err.Error(),
					c.reset,
				)
			}

			return fmt.Sprintf(
				"%s%s: %s: %s%s",
				c.ColorForLevel(d.Level),
				level,
				annotation.File.Path,
				d.Err.Error(),
				c.reset,
			)
		}

		return fmt.Sprintf(
			"%s%s: %s:%d:%d: %s%s",
			c.ColorForLevel(d.Level),
			level,
			annotation.File.Path,
			annotation.Start.Line,
			annotation.Start.Column,
			d.Err.Error(),
			c.reset,
		)
	}

	// For the other styles, we imitate the Rust compiler. See
	// https://github.com/rust-lang/rustc-dev-guide/blob/master/src/diagnostics.md

	var out strings.Builder
	fmt.Fprint(&out, c.BoldForLevel(d.Level), level, ": ", d.Err.Error(), c.reset)

	// Figure out how wide the line bar needs to be. This is given by
	// the width of the largest line value among the annotations.
	var greatestLine int
	for _, snip := range d.Annotations {
		greatestLine = max(greatestLine, snip.End.Line)
	}
	lineBarWidth := len(strconv.Itoa(greatestLine)) // Easier than messing with math.Log10()
	lineBarWidth = max(2, lineBarWidth)

	// Render all the diagnostic windows.
	parts := partition(d.Annotations, func(a, b *Annotation) bool { return a.File.Path != b.File.Path })
	parts(func(i int, annotations []Annotation) bool {
		out.WriteByte('\n')
		out.WriteString(c.nBlue)
		padBy(&out, lineBarWidth)

		if i == 0 {
			primary := d.Annotations[0]
			fmt.Fprintf(&out, "--> %s:%d:%d", primary.File.Path, primary.Start.Line, primary.Start.Column)
		} else {
			primary := annotations[0]
			fmt.Fprintf(&out, "::: %s:%d:%d", primary.File.Path, primary.Start.Line, primary.Start.Column)
		}

		// Add a blank line after the file. This gives the diagnostic window some
		// visual breathing room.
		out.WriteByte('\n')
		out.WriteString(c.nBlue)
		padBy(&out, lineBarWidth)
		out.WriteString(" | ")

		window := buildWindow(d.Level, annotations)
		window.Render(lineBarWidth, &c, &out)
		return true
	})

	// Render a remedial file name for spanless errors.
	if len(d.Annotations) == 0 && d.InFile != "" {
		out.WriteByte('\n')
		out.WriteString(c.nBlue)
		padBy(&out, lineBarWidth-1)

		fmt.Fprintf(&out, "--> %s", d.InFile)
	}

	// Render the footers. For simplicity we collect them into an array first.
	footers := make([][3]string, 0, len(d.Notes)+len(d.Help)+len(d.Debug))
	for _, note := range d.Notes {
		footers = append(footers, [3]string{c.bCyan, "note", note})
	}
	for _, help := range d.Help {
		footers = append(footers, [3]string{c.bCyan, "help", help})
	}
	for _, debug := range d.Debug {
		footers = append(footers, [3]string{c.bRed, "debug", debug})
	}
	for _, footer := range footers {
		out.WriteByte('\n')
		out.WriteString(c.nBlue)
		padBy(&out, lineBarWidth)
		out.WriteString(" = ")
		fmt.Fprint(&out, footer[0], footer[1], ": ", c.reset)
		for i, line := range strings.Split(footer[2], "\n") {
			if i > 0 {
				out.WriteByte('\n')
				margin := lineBarWidth + 3 + len(footer[1]) + 2
				padBy(&out, margin)
			}
			out.WriteString(line)
		}
	}

	out.WriteString(c.reset)
	return out.String()
}

func (r *Renderer) colors() color {
	if !r.Colorize {
		return color{r: r}
	}

	return color{
		r:       r,
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

// color is the colors used for pretty-rendering diagnostics.
type color struct {
	r *Renderer

	reset string
	// Normal colors.
	nRed, nYellow, nCyan, nBlue string
	// Bold colors.
	bRed, bYellow, bCyan, bBlue string
}

func (c color) ColorForLevel(l Level) string {
	switch l {
	case Error:
		return c.nRed
	case Warning:
		if c.r.WarningsAreErrors {
			return c.nRed
		}
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
		if c.r.WarningsAreErrors {
			return c.nRed
		}
		return c.bYellow
	case Remark:
		return c.bCyan
	case note:
		return c.bBlue
	default:
		return ""
	}
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

// buildWindow builds a diagnostic window for the given annotations, which must all have
// the same file.
func buildWindow(level Level, annotations []Annotation) *window {
	w := new(window)
	w.file = annotations[0].File

	// Calculate the range of the file we will be printing. This is given
	// by every line that has a piece of diagnostic in it. To find this, we
	// calculate the join of all of the spans in the window, and find the
	// nearest \n runes in the text.
	w.start = annotations[0].Start.Line
	w.offsets[0] = annotations[0].Start.Offset
	for _, snip := range annotations {
		w.start = min(w.start, snip.Start.Line)
		w.offsets[0] = min(w.offsets[0], snip.Start.Offset)
		w.offsets[1] = max(w.offsets[1], snip.End.Offset)
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
	for _, snippet := range annotations {
		if snippet.Start.Line != snippet.End.Line {
			w.multilines = append(w.multilines, multiline{
				start:      snippet.Start.Line,
				end:        snippet.End.Line,
				startWidth: snippet.Start.Column,
				endWidth:   snippet.End.Column,
				level:      note,
				message:    snippet.Message,
			})
			ml := &w.multilines[len(w.multilines)-1]

			// Calculate whether this snippet starts on the first non-space rune of
			// the line.
			if snippet.Start.Offset != 0 {
				firstLineStart := strings.LastIndexByte(w.file.Text[:snippet.Start.Offset-1], '\n')
				if !strings.ContainsFunc(
					w.file.Text[firstLineStart+1:snippet.Start.Offset-1],
					func(r rune) bool { return !unicode.IsSpace(r) },
				) {
					ml.startWidth = 0
				}
			}

			if snippet.Primary {
				ml.level = level
			}
			continue
		}

		w.underlines = append(w.underlines, underline{
			line:    snippet.Start.Line,
			start:   snippet.Start.Column,
			end:     snippet.End.Column,
			level:   note,
			message: snippet.Message,
		})

		ul := &w.underlines[len(w.underlines)-1]
		if snippet.Primary {
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

func (w *window) Render(lineBarWidth int, c *color, out *strings.Builder) {
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
	parts := partition(w.underlines, func(a, b *underline) bool { return a.line != b.line })
	parts(func(_ int, part []underline) bool {
		cur := &info[part[0].line-w.start]
		cur.shouldEmit = true

		// Arrange for a "sidebar prefix" for this line. This is determined by any sidebars that are
		// active on this line, even if they end on it.
		sidebar := renderSidebar(sidebarLen, -1, -1, c, cur.sidebar)

		// Lay out the physical underlines in reverse order. This will cause longer lines to be
		// laid out first, which will be overwritten by shorter ones.
		//
		// We use a slice instead of a strings.Builder so we can overwrite parts
		// as we render different "layers".
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
		parts := partition(buf, func(a, b *byte) bool { return *a != *b })
		parts(func(_ int, line []byte) bool {
			level := Level(line[0])
			if line[0] == 0 {
				out.WriteString(c.reset)
			} else {
				out.WriteString(c.BoldForLevel(level))
			}
			for range line {
				switch level {
				case 0:
					out.WriteByte(' ')
				case note:
					out.WriteByte('-')
				default:
					out.WriteByte('^')
				}
			}
			return true
		})

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
		cur.underlines = []string{sidebar + underlines + " " + c.BoldForLevel(rightmost.level) + rightmost.message}

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

			// First, lay out the pipes. Note that rest is not necessarily
			// ordered from right to left, so we need to sort the pipes first.
			// To deal with this, we make a copy of rest[idx:], sort it appropriately,
			// and then lay things out.
			//
			// This is quadratic, but no one is going to put more than like. five snippets
			// in a line, so it's fine.
			restSorted := slices.Clone(rest[idx:])
			slices.SortFunc(restSorted, func(a, b *underline) int {
				return a.start - b.start
			})

			var nonColorLen int
			for _, ul := range restSorted {
				col := ul.start - 1
				for nonColorLen < col {
					buf = append(buf, ' ')
					nonColorLen++
				}

				if nonColorLen == col {
					// Two pipes may appear on the same column!
					// This is why this is in a conditional.
					buf = append(buf, c.BoldForLevel(ul.level)...)
					buf = append(buf, '|')
					nonColorLen++
				}
			}

			// Spat in the one with all the pipes in it as-is.
			cur.underlines = append(cur.underlines, strings.TrimRight(sidebar+string(buf), " "))

			// Then, splat in the message. having two rows like this ensures that
			// each message has one pipe directly above it.
			if idx >= 0 {
				ul := rest[idx]

				actualStart := ul.start - 1
				for _, other := range rest[idx:] {
					if other.start <= ul.start {
						actualStart += len(c.BoldForLevel(ul.level))
					}
				}
				for len(buf) < actualStart+len(ul.message)+1 {
					buf = append(buf, ' ')
				}

				// Make sure we don't crop *part* of an escape. To do this, we look for
				// the last ESC in the region we're going to replace. If it is not
				// followed by an m, we need to insert that many spaces into buf to avoid
				// overwriting it.
				writeTo := buf[actualStart:][:len(ul.message)]
				lastEsc := bytes.LastIndexByte(writeTo, 033)
				if lastEsc != -1 && !bytes.ContainsRune(writeTo[lastEsc:], 'm') {
					// If we got here, it means we're going to crop an escape if
					// we don't do something about it.
					spaceNeeded := len(writeTo) - lastEsc
					for i := 0; i < spaceNeeded; i++ {
						buf = append(buf, 0)
					}
					copy(buf[actualStart+lastEsc+spaceNeeded:], buf[actualStart+lastEsc:])
				}

				copy(buf[actualStart:], ul.message)
			}
			cur.underlines = append(cur.underlines, strings.TrimRight(sidebar+string(buf), " "))
		}

		return true
	})

	//nolint:dupword
	// Now that we've laid out the underlines, we can add the starts and ends of all
	// of the multilines, which go after the underlines.
	//
	// The result is that a multiline will look like this:
	//
	//   code
	//  ____^
	// | code code code
	// \______________^ message
	var line strings.Builder
	for i := range info {
		cur := &info[i]
		prevStart := -1
		for j, ml := range cur.sidebar {
			if ml == nil {
				continue
			}

			line.Reset()
			var isStart bool
			switch w.start + i {
			case ml.start:
				if ml.startWidth == 0 {
					continue
				}

				isStart = true
				fallthrough
			case ml.end:
				// We need to be flush with the sidebar here, so we trim the trailing space.
				sidebar := []byte(strings.TrimRight(renderSidebar(0, -1, prevStart, c, cur.sidebar[:j+1]), " "))

				// We also need to erase the bars of any multis that are before this multi
				// and start/end on the same line.
				if !isStart {
					for i, otherML := range cur.sidebar[:j+1] {
						if otherML != nil && otherML.end == ml.end {
							// We assume all the color codes have the same byte length.
							codeLen := len(c.bBlue)
							idx := i*(2+codeLen) + codeLen
							if idx < len(sidebar) {
								sidebar[idx] = ' '
							}
						}
					}
				}

				// Delete the last pipe and replace it with a slash or space, depending.
				// on orientation.
				line.Write(sidebar[:len(sidebar)-1])
				if isStart {
					line.WriteByte(' ')
				} else {
					line.WriteByte('\\')
				}

				// Pad out to the gutter of the code block.
				remaining := sidebarLen - (j + 1)
				padByRune(&line, remaining*2, '_')

				// Pad to right before we need to insert a ^ or -
				if isStart {
					padByRune(&line, ml.startWidth-1, '_')
				} else {
					padByRune(&line, ml.endWidth-1, '_')
				}

				if ml.level == note {
					line.WriteByte('-')
				} else {
					line.WriteByte('^')
				}
				if !isStart && ml.message != "" {
					line.WriteByte(' ')
					line.WriteString(ml.message)
				}
				cur.underlines = append(cur.underlines, line.String())
			}

			if isStart {
				prevStart = j
			} else {
				prevStart = -1
			}
		}
	}

	// Make sure to emit any lines adjacent to another line we want to emit, so long as that
	// line contains printable characters.
	//
	// We copy a set of all the lines we plan to emit before this transformation;
	// otherwise, doing it in-place will cause every nonempty line after a must-emit line
	// to be shown, which we don't want.
	mustEmit := make(map[int]bool)
	for i := range info {
		if info[i].shouldEmit {
			mustEmit[i] = true
		}
	}
	for i := range info {
		// At least two of the below conditions must be true for
		// this line to be shown. Annoyingly, go does not have a conversion
		// from bool to int...
		var score int
		if strings.IndexFunc(lines[i], unicode.IsGraphic) != 0 {
			score++
		}
		if mustEmit[i-1] {
			score++
		}
		if mustEmit[i+1] {
			score++
		}
		if score >= 2 {
			info[i].shouldEmit = true
		}
	}

	lastEmit := w.start
	for i, line := range lines {
		cur := &info[i]
		lineno := i + w.start

		if !cur.shouldEmit {
			continue
		}

		// If the last multi of the previous line starts on that line, make its
		// pipe here a slash so that it connects properly.
		slashAt := -1
		if i > 0 {
			prevSidebar := info[i-1].sidebar
			if len(prevSidebar) > 0 &&
				prevSidebar[len(prevSidebar)-1].start == lineno-1 &&
				prevSidebar[len(prevSidebar)-1].startWidth > 0 {
				slashAt = len(prevSidebar) - 1
			}
		}
		sidebar := renderSidebar(sidebarLen, lineno, slashAt, c, cur.sidebar)

		if i > 0 && !info[i-1].shouldEmit {
			// Generate a visual break if this is right after a real line.
			out.WriteByte('\n')
			out.WriteString(c.nBlue)
			padBy(out, lineBarWidth-2)
			out.WriteString("...  ")

			// Generate a sidebar as before but this time we want to look at the
			// last line that was actually emitted.
			slashAt := -1
			prevSidebar := info[lastEmit].sidebar
			if len(prevSidebar) > 0 &&
				prevSidebar[len(prevSidebar)-1].start == lastEmit &&
				prevSidebar[len(prevSidebar)-1].startWidth > 0 {
				slashAt = len(prevSidebar) - 1
			}

			out.WriteString(renderSidebar(sidebarLen, lineno, slashAt, c, cur.sidebar))
		}

		// Ok, we are definitely printing this line out.
		fmt.Fprintf(out, "\n%s%*d | %s%s", c.nBlue, lineBarWidth, lineno, sidebar, c.reset)
		lastEmit = lineno

		// Replace tabstops with spaces.
		var column int
		// We can't just use StringWidth, because that doesn't respect tabstops
		// correctly.
		for {
			nextTab := strings.IndexByte(line, '\t')
			if nextTab != -1 {
				column += uniseg.StringWidth(line[:nextTab])
				out.WriteString(line[:nextTab])

				tab := TabstopWidth - (column % TabstopWidth)
				column += tab
				padBy(out, tab)

				line = line[nextTab+1:]
			} else {
				out.WriteString(line)
				break
			}
		}

		// If this happens to be an annotated line, this is when it gets annotated.
		for _, line := range cur.underlines {
			out.WriteByte('\n')
			out.WriteString(c.nBlue)
			padBy(out, lineBarWidth)
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
	start, end           int
	startWidth, endWidth int
	level                Level
	message              string
}

func renderSidebar(bars, lineno, slashAt int, c *color, multis []*multiline) string {
	var sidebar strings.Builder
	for i, ml := range multis {
		if ml == nil {
			sidebar.WriteString("  ")
			continue
		}

		sidebar.WriteString(c.BoldForLevel(ml.level))

		switch {
		case slashAt == i:
			sidebar.WriteByte('/')
		case lineno != ml.start:
			sidebar.WriteByte('|')
		case ml.startWidth == 0:
			sidebar.WriteByte('/')
		default:
			sidebar.WriteByte(' ')
		}
		sidebar.WriteByte(' ')
	}
	for sidebar.Len() < bars*2 {
		sidebar.WriteByte(' ')
	}
	return sidebar.String()
}

// partition returns an iterator of subslices of s such that each yielded
// slice is delimited according to delimit. Also yields the starting index of
// the subslice.
//
// In other words, suppose delimit is !=. Then, the slice [a a a b c c] is yielded
// as the subslices [a a a], [b], and [c c c].
//
// Will never yield an empty slice.
//
//nolint:dupword
func partition[T any](s []T, delimit func(a, b *T) bool) func(func(int, []T) bool) {
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

func padBy(out *strings.Builder, spaces int) {
	for i := 0; i < spaces; i++ {
		out.WriteByte(' ')
	}
}

func padByRune(out *strings.Builder, spaces int, r rune) {
	for i := 0; i < spaces; i++ {
		out.WriteRune(r)
	}
}
