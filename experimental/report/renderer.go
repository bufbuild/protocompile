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
	"fmt"
	"io"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/stringsx"
)

// Renderer configures a diagnostic rendering operation.
type Renderer struct {
	// If set, uses a compact one-line format for each diagnostic.
	Compact bool

	// If set, rendering results are enriched with ANSI color escapes.
	Colorize bool

	// Upgrades all warnings to errors.
	WarningsAreErrors bool

	// If set, remark diagnostics will be printed.
	ShowRemarks bool

	// If set, rendering a diagnostic will show the debug footer.
	ShowDebug bool
}

// renderer contains shared state for a rendering operation, allowing e.g.
// allocations to be re-used and simplifying function signatures.
type renderer struct {
	Renderer
	writer

	ss styleSheet

	// The width, in columns, of the line number margin in the diagnostic
	// currently being rendered.
	margin int
}

// Render renders a diagnostic report.
//
// In addition to returning the rendering result, returns whether the report
// contains any errors.
//
// On the other hand, the actual error-typed return is an error when writing to
// the writer.
func (r Renderer) Render(report *Report, out io.Writer) (errorCount, warningCount int, err error) {
	state := &renderer{
		Renderer: r,
		writer:   writer{out: out},
		ss:       newStyleSheet(r),
	}
	return state.render(report)
}

// RenderString is a helper for calling [Renderer.Render] with a [strings.Builder].
func (r Renderer) RenderString(report *Report) (text string, errorCount, warningCount int) {
	var buf strings.Builder
	e, w, _ := r.Render(report, &buf)
	return buf.String(), e, w
}

func (r *renderer) render(report *Report) (errorCount, warningCount int, err error) {
	for _, diagnostic := range report.Diagnostics {
		if !r.ShowRemarks && diagnostic.level == Remark {
			continue
		}

		r.diagnostic(report, diagnostic)
		if err := r.Flush(); err != nil {
			return errorCount, warningCount, err
		}

		switch {
		case diagnostic.level <= Error:
			errorCount++
		case diagnostic.level <= Warning:
			if r.WarningsAreErrors {
				errorCount++
			} else {
				warningCount++
			}
		}
	}
	if r.Compact {
		return errorCount, warningCount, err
	}

	switch {
	case errorCount > 0 && warningCount > 0:
		fmt.Fprintf(r, "%sencountered %d error%v and %d warning%v\n%s",
			r.ss.bError,
			errorCount, plural(errorCount), warningCount, plural(warningCount),
			r.ss.reset,
		)
	case errorCount > 0:
		fmt.Fprintf(r, "%sencountered %d error%v\n%s",
			r.ss.bError,
			errorCount, plural(errorCount), r.ss.reset,
		)
	case warningCount > 0:
		fmt.Fprintf(r, "%sencountered %d warning%v\n%s",
			r.ss.bWarning,
			warningCount, plural(warningCount), r.ss.reset,
		)
	}
	return errorCount, warningCount, r.Flush()
}

// diagnostic renders a single diagnostic to a string.
func (r *renderer) diagnostic(report *Report, d Diagnostic) {
	if report.Tracing > 0 {
		// If we're debugging diagnostic traces, and we panic, show where this
		// particular diagnostic was generated. This is useful for debugging
		// renderer bugs.
		defer func() {
			if panicked := recover(); panicked != nil {
				stack := strings.Join(d.debug[:min(report.Tracing, len(d.debug))], "\n")
				panic(fmt.Sprintf("protocompile/report: panic in renderer: %v\ndiagnosed at:\n%s", panicked, stack))
			}
		}()
	}

	var level string
	switch d.level {
	case ICE:
		level = "internal compiler error"
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

	// For the simple style, we imitate the Go compiler.
	if r.Compact {
		r.WriteString(r.ss.ColorForLevel(d.level))
		primary := d.Primary()
		switch {
		case primary.File != nil:
			start := primary.StartLoc()
			fmt.Fprintf(r, "%s: %s:%d:%d: %s",
				level, primary.Path(),
				start.Line, start.Column,
				d.message,
			)
		case d.inFile != "":
			fmt.Fprintf(r, "%s: %s: %s",
				level, d.inFile, d.message,
			)
		default:
			fmt.Fprintf(r, "%s: %s", level, d.message)
		}
		r.WriteString(r.ss.reset)
		r.WriteString("\n")
		return
	}

	// For the other styles, we imitate the Rust compiler. See
	// https://github.com/rust-lang/rustc-dev-guide/blob/master/src/diagnostics.md

	fmt.Fprint(r, r.ss.BoldForLevel(d.level), level, ": ")
	r.WriteWrapped(d.message, MaxMessageWidth)

	locations := make([][2]Location, len(d.snippets))
	for i, snip := range d.snippets {
		locations[i][0] = snip.location(snip.Start, false)
		if strings.HasSuffix(snip.Text(), "\n") {
			// If the snippet ends in a newline, don't include the newline in the
			// printed span.
			locations[i][1] = snip.location(snip.End-1, false)
			locations[i][1].Column++
		} else {
			locations[i][1] = snip.location(snip.End, false)
		}
	}

	// Figure out how wide the line bar needs to be. This is given by
	// the width of the largest line value among the snippets.
	var greatestLine int
	for _, loc := range locations {
		greatestLine = max(greatestLine, loc[1].Line)
	}
	r.margin = max(2, len(strconv.Itoa(greatestLine))) // Easier than messing with math.Log10()

	// Render all the diagnostic windows.
	parts := slicesx.PartitionKey(d.snippets, func(snip snippet) any {
		if len(snip.edits) > 0 {
			// Suggestions are always rendered in their own windows.
			// Return a fresh pointer, since that will always compare as
			// distinct.
			return new(int)
		}
		return snip.Path()
	})

	parts(func(i int, snippets []snippet) bool {
		if i == 0 || d.snippets[i-1].Path() != d.snippets[i].Path() {
			r.WriteString("\n")
			r.WriteString(r.ss.nAccent)
			r.WriteSpaces(r.margin)

			primary := snippets[0]
			start := locations[i][0]
			sep := ":::"
			if i == 0 {
				sep = "-->"
			}
			fmt.Fprintf(r, "%s %s:%d:%d\n", sep, primary.Path(), start.Line, start.Column)
		}

		if len(snippets[0].edits) > 0 {
			if i > 0 {
				r.WriteString("\n")
			}
			r.suggestion(snippets[0])
			return true
		}

		// Add a blank line after the file. This gives the diagnostic window some
		// visual breathing room.
		r.WriteSpaces(r.margin)
		r.WriteString(" | ")

		window := buildWindow(d.level, locations[i:i+len(snippets)], snippets)
		r.window(window)
		return true
	})

	// Render a remedial file name for spanless errors.
	if len(d.snippets) == 0 && d.inFile != "" {
		r.WriteString("\n")
		r.WriteString(r.ss.nAccent)
		r.WriteSpaces(r.margin - 1)

		fmt.Fprintf(r, "--> %s", d.inFile)
	}

	type footer struct {
		color, label, text string
	}
	footers := iterx.Chain(
		slicesx.Map(d.notes, func(s string) footer { return footer{r.ss.bRemark, "note", s} }),
		slicesx.Map(d.help, func(s string) footer { return footer{r.ss.bRemark, "help", s} }),
		slicesx.Map(d.debug, func(s string) footer { return footer{r.ss.bError, "debug", s} }),
	)

	footers(func(f footer) bool {
		isDebug := f.label == "debug"
		if isDebug && !r.ShowDebug {
			return true
		}

		r.WriteString("\n")
		r.WriteSpaces(r.margin)
		fmt.Fprintf(r, "%s = %s%s: %s", r.ss.nAccent, f.color, f.label, r.ss.reset)

		if isDebug {
			r.WriteWrapped(f.text, math.MaxInt)
		} else {
			r.WriteWrapped(f.text, MaxMessageWidth)
		}

		return true
	})

	r.WriteString(r.ss.reset)
	r.WriteString("\n\n")
}

const maxMultilinesPerWindow = 8

// window is an intermediate structure for rendering an annotated code snippet
// consisting of multiple spans in the same file.
type window struct {
	file *File
	// The line number at which the text starts in the overall source file.
	start int
	// The byte offset range this window's text occupies in the containing
	// source File.
	offsets [2]int
	// A list of all underline elements in this window. Must be sorted
	// according to cmpUnderlines.
	underlines []underline
	multilines []multiline
}

// buildWindow builds a diagnostic window for the given snippets, which must all have
// the same file.
//
// This is separate from [window.Render] because it performs certain layout
// decisions that cannot happen in the middle of actually rendering the source
// code (well, they could, but the resulting code would be far more complicated).
func buildWindow(level Level, locations [][2]Location, snippets []snippet) *window {
	w := new(window)
	w.file = snippets[0].File

	// Calculate the range of the file we will be printing. This is given
	// by every line that has a piece of diagnostic in it. To find this, we
	// calculate the join of all of the spans in the window, and find the
	// nearest \n runes in the text.
	w.start = locations[0][0].Line
	w.offsets[0] = snippets[0].Start
	for i := range snippets {
		w.start = min(w.start, locations[i][0].Line)
		w.offsets[0] = min(w.offsets[0], locations[i][0].Offset)
		w.offsets[1] = max(w.offsets[1], locations[i][1].Offset)
	}
	w.offsets[0], w.offsets[1] = adjustLineOffsets(w.file.Text(), w.offsets[0], w.offsets[1])

	// Now, convert each span into an underline or multiline.
	for i, snippet := range snippets {
		isMulti := locations[i][0].Line != locations[i][1].Line

		if isMulti && len(w.multilines) < maxMultilinesPerWindow {
			w.multilines = append(w.multilines, multiline{
				start:      locations[i][0].Line,
				end:        locations[i][1].Line,
				startWidth: locations[i][0].Column,
				endWidth:   locations[i][1].Column,
				level:      noteLevel,
				message:    snippet.message,
			})
			ml := &w.multilines[len(w.multilines)-1]

			if ml.startWidth == ml.endWidth {
				ml.endWidth++
			}

			// Calculate whether this snippet starts on the first non-space rune of
			// the line.
			if snippet.Start != 0 {
				firstLineStart := strings.LastIndexByte(w.file.Text()[:snippet.Start], '\n')
				if !strings.ContainsFunc(
					w.file.Text()[firstLineStart+1:snippet.Start],
					func(r rune) bool { return !unicode.IsSpace(r) },
				) {
					ml.startWidth = 0
				}
			}

			if snippet.primary {
				ml.level = level
			}
			continue
		}

		w.underlines = append(w.underlines, underline{
			line:    locations[i][0].Line,
			start:   locations[i][0].Column,
			end:     locations[i][1].Column,
			level:   noteLevel,
			message: snippet.message,
		})

		ul := &w.underlines[len(w.underlines)-1]
		if snippet.primary {
			ul.level = level
		}
		if ul.start == ul.end {
			ul.end++
		}

		if isMulti {
			// This is an "overflow multiline" for diagnostics with too
			// many multilines. In this case, we want to end the underline at
			// the end of the first line.
			lineEnd := strings.Index(w.file.Text()[snippet.Start:], "\n")
			if lineEnd == -1 {
				lineEnd = len(w.file.Text())
			} else {
				lineEnd += snippet.Start
			}
			ul.end = ul.start + stringWidth(ul.start, w.file.Text()[snippet.Start:lineEnd], false, nil)
		}

		// Make sure no empty underlines exist.
		if ul.Len() == 0 {
			ul.start++
		}
	}

	slices.SortFunc(w.underlines, cmpUnderlines)
	slices.SortFunc(w.multilines, cmpMultilines)
	return w
}

func (r *renderer) window(w *window) {
	// lineInfo is layout information for a single line of this window. There
	// is one lineInfo for each line of w.file.Text we intend to render, as
	// given by w.offsets.
	type lineInfo struct {
		// This is the multilines whose pipes intersect with this line.
		sidebar []*multiline
		// This is a set of strings to render verbatim under the actual source
		// code line. This makes it possible to lay out all of the complex
		// underlines ahead of time instead of interleaved with rendering the
		// source code lines.
		underlines []string
		// This is whether this line should be printed in the window. This is
		// used to avoid emitting e.g. lines between the start and end of a
		// 100-line multi.
		shouldEmit bool
	}

	lines := strings.Split(w.file.Text()[w.offsets[0]:w.offsets[1]], "\n")
	// Populate ancillary info for each line.
	info := make([]lineInfo, len(lines))

	// First, lay out the multilines, and compute how wide the sidebar is.
	for i := range w.multilines {
		multi := &w.multilines[i]
		// Find the smallest unused index by every line in the range.
		//
		// We want to assign to each multiline a "sidebar index", which is which
		// column its connecting pipes | are placed on. For each multiline, we
		// want to allocate the leftmost index such that it does not conflict
		// with any previously allocated sidebar pipes that are in the same
		// range as this multiline. We cannot simply take the max of their
		// indices, because it might happen that this multiline only intersects
		// with multis on lines that only use indices 0 and 2. This can happen
		// if the multi on index 2 intersects a *different* range that already
		// has two other multis in it.
		//
		// We achieve this by looking at all already-laid-out multis in this
		// multi's range and using a bitset to detect the least unused index.
		// Note that we artificially limit the number of rendered multis to
		// 8 in the code that builds the window itself.
		var multilineBitset uint
		for i := multi.start; i <= multi.end; i++ {
			for col, ml := range info[i-w.start].sidebar {
				if ml != nil {
					multilineBitset |= 1 << col
				}
			}
		}
		idx := bits.TrailingZeros(^multilineBitset)

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
	parts := slicesx.PartitionKey(w.underlines, func(u underline) int { return u.line })
	parts(func(_ int, part []underline) bool {
		cur := &info[part[0].line-w.start]
		cur.shouldEmit = true

		// Arrange for a "sidebar prefix" for this line. This is determined by any sidebars that are
		// active on this line, even if they end on it.
		sidebar := r.sidebar(sidebarLen, -1, -1, cur.sidebar)

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
				// This comparison ensures that we do not overwrite an error
				// underline with a note underline, regardless of ordering.
				if buf[j] == 0 || buf[j] > byte(element.level) {
					buf[j] = byte(element.level)
				}
			}
		}

		// Now, convert the buffer into a proper string.
		var out strings.Builder
		slicesx.Partition(buf)(func(_ int, line []byte) bool {
			level := Level(line[0])
			if line[0] == 0 {
				out.WriteString(r.ss.reset)
			} else {
				out.WriteString(r.ss.BoldForLevel(level))
			}
			for range line {
				switch level {
				case 0:
					out.WriteByte(' ')
				case noteLevel:
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
		cur.underlines = []string{sidebar + underlines + " " + r.ss.BoldForLevel(rightmost.level) + rightmost.message}

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
			// This is quadratic, but no one is going to put more than like, five snippets
			// in a whole diagnostic, much less five snippets that share a line, so
			// this shouldn't be an issue.
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
					buf = append(buf, r.ss.BoldForLevel(ul.level)...)
					buf = append(buf, '|')
					nonColorLen++
				}
			}

			// Splat in the one with all the pipes in it as-is.
			cur.underlines = append(cur.underlines, strings.TrimRight(sidebar+string(buf), " "))

			// Then, splat in the message. having two rows like this ensures that
			// each message has one pipe directly above it.
			if idx >= 0 {
				ul := rest[idx]

				actualStart := ul.start - 1
				for _, other := range rest[idx:] {
					if other.start <= ul.start {
						actualStart += len(r.ss.BoldForLevel(ul.level))
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
					for range spaceNeeded {
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
	for lineIdx := range info {
		cur := &info[lineIdx]
		prevStart := -1
		for mlIdx, ml := range cur.sidebar {
			if ml == nil {
				continue
			}

			line.Reset()
			var isStart bool
			switch w.start + lineIdx {
			case ml.start:
				if ml.startWidth == 0 {
					continue
				}

				isStart = true
				fallthrough
			case ml.end:
				// We need to be flush with the sidebar here, so we trim the trailing space.
				sidebar := []byte(strings.TrimRight(r.sidebar(0, -1, prevStart, cur.sidebar[:mlIdx+1]), " "))

				// We also need to erase the bars of any multis that are before this multi
				// and start/end on the same line.
				if !isStart {
					for mlIdx, otherML := range cur.sidebar[:mlIdx+1] {
						if otherML != nil && otherML.end == ml.end {
							// All the color escapes have the same byte length, so we can use the length of
							// any of them to measure how far we need to adjust the offset to get to the
							// pipe. We need to account for one escape per multiline, and also need to skip
							// past the color escape on the pipe we want to erase.
							codeLen := len(r.ss.bAccent)
							idx := mlIdx*(2+codeLen) + codeLen
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
				remaining := sidebarLen - (mlIdx + 1)
				padByRune(&line, remaining*2, '_')

				// Pad to right before we need to insert a ^ or -
				if isStart {
					padByRune(&line, ml.startWidth-1, '_')
				} else {
					padByRune(&line, ml.endWidth-1, '_')
				}

				if ml.level == noteLevel {
					line.WriteByte('-')
				} else {
					line.WriteByte('^')
				}

				// TODO: If the source code has extremely long lines, this will cause
				// the message to wind up wrapped crazy far. It may be worth doing
				// wrapping ourselves in some cases (beyond a threshold of, say, 120
				// columns). It is unlikely users will hit this problem with "realistic"
				// inputs, though, and e.g. rustc and clang do not bother to handle this
				// case nicely.
				if !isStart && ml.message != "" {
					line.WriteByte(' ')
					line.WriteString(ml.message)
				}
				cur.underlines = append(cur.underlines, line.String())
			}

			if isStart {
				prevStart = mlIdx
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
		sidebar := r.sidebar(sidebarLen, lineno, slashAt, cur.sidebar)

		if i > 0 && !info[i-1].shouldEmit {
			// Generate a visual break if this is right after a real line.
			r.WriteString("\n")
			r.WriteString(r.ss.nAccent)
			r.WriteSpaces(r.margin - 2)
			r.WriteString("...  ")

			// Generate a sidebar as before but this time we want to look at the
			// last line that was actually emitted.
			slashAt := -1
			prevSidebar := info[lastEmit-w.start].sidebar
			if len(prevSidebar) > 0 &&
				prevSidebar[len(prevSidebar)-1].start == lastEmit &&
				prevSidebar[len(prevSidebar)-1].startWidth > 0 {
				slashAt = len(prevSidebar) - 1
			}

			r.WriteString(r.sidebar(sidebarLen, lineno, slashAt, cur.sidebar))
		}

		// Ok, we are definitely printing this line out.
		//
		// Note that sidebar already includes a trailing ss.reset for us.
		fmt.Fprintf(r, "\n%s%*d | %s%s", r.ss.nAccent, r.margin, lineno, sidebar, r.ss.reset)
		lastEmit = lineno

		// Re-use the logic from width calculation to correctly format a line for
		// showing in a terminal.
		stringWidth(0, line, false, &r.writer)

		// If this happens to be an annotated line, this is when it gets annotated.
		for _, line := range cur.underlines {
			r.WriteString("\n")
			r.WriteString(r.ss.nAccent)
			r.WriteSpaces(r.margin)
			r.WriteString(" | ")
			r.WriteString(line)
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

// cmpUnderliens sorts ascending on line, then level, then length, then
// start column.
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

// cmpMultilines sorts ascending on line, then descending on end. This sort
// order is intended to promote visual nesting of multis from left to right.
func cmpMultilines(a, b multiline) int {
	if diff := a.start - b.start; diff != 0 {
		return diff
	}
	return b.end - a.end
}

func (r *renderer) sidebar(bars, lineno, slashAt int, multis []*multiline) string {
	var sidebar strings.Builder
	for i, ml := range multis {
		if ml == nil {
			sidebar.WriteString("  ")
			continue
		}

		sidebar.WriteString(r.ss.BoldForLevel(ml.level))

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

// suggestion renders a single suggestion window.
func (r *renderer) suggestion(snip snippet) {
	r.WriteString(r.ss.nAccent)
	r.WriteSpaces(r.margin)
	r.WriteString("help: ")
	r.WriteWrapped(snip.message, MaxMessageWidth)

	// Add a blank line after the file. This gives the diagnostic window some
	// visual breathing room.
	r.WriteString("\n")
	r.WriteSpaces(r.margin)
	r.WriteString(" | ")

	// When the suggestion spans multiple lines, we don't bother doing a by-the-rune
	// diff, because the result can be hard for users to understand how to apply
	// to their code. Also, if the suggestion contains deletions, use
	multiline := slices.ContainsFunc(snip.edits, func(e Edit) bool {
		// Prefer multiline suggestions in the case of deletions.
		return e.IsDeletion() || strings.Contains(e.Replace, "\n")
	}) ||
		strings.Contains(snip.Span.Text(), "\n")

	if multiline {
		span, hunks := unifiedDiff(snip.Span, snip.edits)
		aLine := span.StartLoc().Line
		bLine := aLine
		for i, hunk := range hunks {
			// Trim a single newline before and after hunk. This helps deal with
			// cases where a newline gets duplicated across hunks of different
			// type.
			hunk.content, _ = strings.CutPrefix(hunk.content, "\n")
			hunk.content, _ = strings.CutSuffix(hunk.content, "\n")

			if hunk.content == "" {
				continue
			}

			// Skip addition lines that only contain whitespace, if the previous
			// hunk was a deletion. This helps avoid cases where a whole line
			// was deleted and some indentation was left over.
			if prev, _ := slicesx.Get(hunks, i-1); prev.kind == hunkDelete &&
				hunk.kind == hunkAdd &&
				stringsx.EveryFunc(hunk.content, unicode.IsSpace) {
				continue
			}

			for _, line := range strings.Split(hunk.content, "\n") {
				lineno := aLine
				if hunk.kind == '+' {
					lineno = bLine
				}

				// Draw the line as we would for an ordinary window, but prefix
				// each line with a the hunk's kind and color.
				fmt.Fprintf(r, "\n%s%*d | %s%c%s %s",
					r.ss.nAccent, r.margin, lineno,
					hunk.bold(&r.ss), hunk.kind, hunk.color(&r.ss),
					line,
				)

				switch hunk.kind {
				case hunkUnchanged:
					aLine++
					bLine++
				case hunkDelete:
					aLine++
				case hunkAdd:
					bLine++
				}
			}
		}

		r.WriteString("\n")
		r.WriteString(r.ss.nAccent)
		r.WriteSpaces(r.margin)
		r.WriteString(" | ")
		return
	}

	span, hunks := hunkDiff(snip.Span, snip.edits)
	fmt.Fprintf(r, "\n%s%*d | ", r.ss.nAccent, r.margin, span.StartLoc().Line)
	var column int
	for _, hunk := range hunks {
		if hunk.content == "" {
			continue
		}

		r.WriteString(hunk.color(&r.ss))
		// Re-use the logic from width calculation to correctly format a line for
		// showing in a terminal.
		column = stringWidth(column, hunk.content, false, &r.writer)
	}

	// Draw underlines for each modified segment, using + and - as the
	// underline characters.
	r.WriteString("\n")
	r.WriteString(r.ss.nAccent)
	r.WriteSpaces(r.margin)
	r.WriteString(" | ")
	column = 0
	for _, hunk := range hunks {
		if hunk.content == "" {
			continue
		}

		prev := column
		column = stringWidth(column, hunk.content, false, nil)
		r.WriteString(hunk.bold(&r.ss))
		for range column - prev {
			r.WriteString(string(hunk.kind))
		}
	}
}

func adjustLineOffsets(text string, start, end int) (int, int) {
	// Find the newlines before and after the given ranges, respectively.
	// This snaps the range to start immediately after a newline (or SOF) and
	// end immediately before a newline (or EOF).
	start = strings.LastIndexByte(text[:start], '\n') + 1 // +1 gives the byte *after* the newline.
	if offset := strings.IndexByte(text[end:], '\n'); offset != -1 {
		end += offset
	} else {
		end = len(text)
	}
	return start, end
}

func padByRune(out *strings.Builder, spaces int, r rune) {
	for range spaces {
		out.WriteRune(r)
	}
}
