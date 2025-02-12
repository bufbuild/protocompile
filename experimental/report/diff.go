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
	"slices"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

const (
	hunkUnchanged = ' '
	hunkAdd       = '+'
	hunkDelete    = '-'
)

// hunk is a render-able piece of a diff.
type hunk struct {
	kind    rune
	content string
}

func (h hunk) color(ss *styleSheet) string {
	switch h.kind {
	case hunkAdd:
		return ss.nAdd
	case hunkDelete:
		return ss.nDelete
	default:
		return ss.reset
	}
}

func (h hunk) bold(ss *styleSheet) string {
	switch h.kind {
	case hunkAdd:
		return ss.bAdd
	case hunkDelete:
		return ss.bDelete
	default:
		return ss.reset
	}
}

// hunkDiff computes edit hunks for a diff.
func hunkDiff(span Span, edits []Edit) (Span, []hunk) {
	out := make([]hunk, 0, len(edits)*3+1)
	var prev int
	span, edits = offsetsForDiffing(span, edits)
	src := span.Text()
	for _, edit := range edits {
		out = append(out,
			hunk{hunkUnchanged, src[prev:edit.Start]},
			hunk{hunkDelete, src[edit.Start:edit.End]},
			hunk{hunkAdd, edit.Replace},
		)
		prev = edit.End
	}
	return span, append(out, hunk{hunkUnchanged, src[prev:]})
}

// unifiedDiff computes whole-line hunks for this diff, for producing a unified
// edit.
//
// Each slice will contain one or more lines that should be displayed together.
func unifiedDiff(span Span, edits []Edit) (Span, []hunk) {
	// Sort the edits such that they are ordered by starting offset.
	span, edits = offsetsForDiffing(span, edits)
	src := span.Text()
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].End
	})

	// Partition offsets into overlapping lines. That is, this connects together
	// all edit spans whose end and start are not separated by a newline.
	prev := 0
	parts := slicesx.SplitFunc(edits, func(i int, next Edit) bool {
		if i == prev {
			return false
		}

		chunk := src[edits[i-1].End:next.Start]
		if !strings.Contains(chunk, "\n") {
			return false
		}

		prev = i
		return true
	})

	var out []hunk
	var prevHunk int
	parts(func(edits []Edit) bool {
		// First, figure out the start and end of the modified region.
		start, end := edits[0].Start, edits[0].End
		for _, edit := range edits[1:] {
			start = min(start, edit.Start)
			end = max(end, edit.End)
		}
		// Then, snap the region to be newline delimited. This is the unedited
		// lines.
		start, end = adjustLineOffsets(src, start, end)
		original := src[start:end]

		// Now, apply the edits to original to produce the modified result.
		var buf strings.Builder
		prev := 0
		for _, edit := range edits {
			buf.WriteString(original[prev : edit.Start-start])
			buf.WriteString(edit.Replace)
			prev = edit.End - start
		}
		buf.WriteString(original[prev:])

		// Dump the result into the output.
		out = append(out,
			hunk{hunkUnchanged, src[prevHunk:start]},
			hunk{hunkDelete, src[start:end]},
			hunk{hunkAdd, buf.String()},
		)

		prevHunk = end
		return true
	})
	return span, append(out, hunk{hunkUnchanged, src[prevHunk:]})
}

// offsetsForDiffing pre-calculates information needed for diffing:
// the line-snapped span, and edits which are adjusted to conform to that
// span.
func offsetsForDiffing(span Span, edits []Edit) (Span, []Edit) {
	edits = slices.Clone(edits)
	var start, end int
	for i := range edits {
		e := &edits[i]
		e.Start += span.Start
		e.End += span.Start
		if i == 0 {
			start, end = e.Start, e.End
		} else {
			start, end = min(e.Start, start), max(e.End, end)
		}
	}

	start, end = adjustLineOffsets(span.File.Text(), start, end)
	for i := range edits {
		edits[i].Start -= start
		edits[i].End -= start
	}

	return span.File.Span(start, end), edits
}
