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

	"github.com/bufbuild/protocompile/experimental/source"
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
func hunkDiff(span source.Span, edits []Edit) (source.Span, []hunk) {
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
func unifiedDiff(span source.Span, edits []Edit) (source.Span, []hunk) {
	// Sort the edits such that they are ordered by starting offset.
	span, edits = offsetsForDiffing(span, edits)
	src := span.Text()
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].End
	})

	// Partition offsets into overlapping lines. That is, this connects together
	// all edit spans whose end and start are not separated by a newline.
	parts := slicesx.SplitAfterFunc(edits, func(i int, edit Edit) bool {
		next, ok := slicesx.Get(edits, i+1)
		return ok && edit.End < next.Start && // Go treats str[x:y] for x > y as an error.
			strings.Contains(src[edit.End:next.Start], "\n")
	})

	var out []hunk //nolint:prealloc // False positive.
	var prevHunk int
	for edits := range parts {
		if len(edits) == 0 {
			continue
		}

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
			buf.WriteString(original[prev:max(prev, edit.Start-start)])
			buf.WriteString(edit.Replace)
			prev = edit.End - start
		}
		buf.WriteString(original[prev:])

		unchanged := src[prevHunk:start]
		deleted := src[start:end]
		added := buf.String()
		if stripped, ok := strings.CutPrefix(added, deleted); ok &&
			strings.HasPrefix(stripped, "\n") {
			// It is possible for deleted to be a line suffix of added; outputting
			// a diff like this doesn't look good, so we should fix it up here.
			unchanged = src[prevHunk:end]
			deleted = ""
			added = strings.TrimPrefix(stripped, "\n")
		}

		trim := func(s string) string {
			s = strings.TrimPrefix(s, "\n")
			s = strings.TrimSuffix(s, "\n")
			return s
		}

		// Dump the result into the output.
		out = append(out,
			hunk{hunkUnchanged, trim(unchanged)},
			hunk{hunkDelete, deleted},
			hunk{hunkAdd, added},
		)

		prevHunk = end
	}
	return span, append(out, hunk{hunkUnchanged, src[prevHunk:]})
}

// offsetsForDiffing pre-calculates information needed for diffing:
// the line-snapped span, and edits which are adjusted to conform to that
// span.
func offsetsForDiffing(span source.Span, edits []Edit) (source.Span, []Edit) {
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
