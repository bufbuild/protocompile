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
	"sort"
	"strings"

	"github.com/bufbuild/protocompile/internal/iters"
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
func hunkDiff(span Span, edits []Edit) []hunk {
	out := make([]hunk, 0, len(edits)*3+1)
	var prev int
	src, offsets := offsetsForDiffing(span, edits)
	for i, edit := range edits {
		start := offsets[i][0]
		end := offsets[i][1]

		out = append(out,
			hunk{hunkUnchanged, src[prev:start]},
			hunk{hunkDelete, src[start:end]},
			hunk{hunkAdd, edit.Replace},
		)
		prev = end
	}
	return append(out, hunk{hunkUnchanged, src[prev:]})
}

// unifiedDiff computes whole-line hunks for this diff, for producing a unified
// edit.
//
// Each slice will contain one or more lines that should be displayed together.
func unifiedDiff(span Span, edits []Edit) []hunk {
	// Sort the edits such that they are ordered by starting offset.
	src, offsets := offsetsForDiffing(span, edits)
	sort.Slice(edits, func(i, j int) bool {
		return offsets[i][0] < offsets[j][0]
	})

	// Partition offsets into overlapping lines. That is, this connects together
	// all edit spans whose end and start are not separated by a newline.
	prev := &offsets[0]
	parts := iters.Partition(offsets, func(_, next *[2]int) bool {
		if next == prev || !strings.Contains(src[prev[1]:next[0]], "\n") {
			return false
		}

		prev = next
		return true
	})

	var out []hunk
	var prevHunk int
	parts(func(i int, offsets [][2]int) bool {
		// First, figure out the start and end of the modified region.
		start, end := offsets[0][0], offsets[0][1]
		for _, offset := range offsets[1:] {
			start = min(start, offset[0])
			end = max(end, offset[1])
		}
		// Then, snap the region to be newline delimited. This is the unedited
		// lines.
		start, end = adjustLineOffsets(src, start, end)
		original := src[start:end]

		// Now, apply the edits to original to produce the modified result.
		var buf strings.Builder
		prev := 0
		for j, offset := range offsets {
			buf.WriteString(original[prev:offset[0]])
			buf.WriteString(edits[i+j].Replace)
			prev = offset[1]
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
	return append(out, hunk{hunkUnchanged, src[prevHunk:]})
}

// offsetsForDiffing pre-calculates information needed for diffing:
// the line-snapped span, and the offsetsForDiffing of each edit as indices into
// that span.
func offsetsForDiffing(span Span, edits []Edit) (string, [][2]int) {
	start, end := adjustLineOffsets(span.File.Text(), span.Start, span.End)
	delta := span.Start - start

	offsets := make([][2]int, len(edits))
	for i, edit := range edits {
		offsets[i] = [2]int{edit.Start + delta, edit.End + delta}
	}

	return span.File.Text()[start:end], offsets
}
