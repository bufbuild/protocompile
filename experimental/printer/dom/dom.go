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

package dom

import (
	"bytes"
	"fmt"
	"slices"
)

// Dom represents a block of text with formatting information. It is a tree of [Chunk]s.
type Dom struct {
	chunks []*Chunk
	// Maintain a pointer to the last non-whitespace-only chunk inserted in the Dom.
	lastNonWhitespaceChunkIdx int
	lastNonWhitespaceChunkSet bool
	formatting                *formatting
}

// NewDom constructs a new Dom.
func NewDom() *Dom {
	return &Dom{}
}

// Insert a Chunk into the Dom. If a nil Chunk is passed, it is dropped. If an unset Chunk
// is passed, it is dropped.
func (d *Dom) Insert(chunks ...*Chunk) {
	for _, c := range chunks {
		if c == nil || !c.hasText {
			continue
		}
		if c.hasText {
			// We set the initial indenting based on the last non-whitespace-only chunk's splitKind.
			// This may change after formatting and potential new splits are applied.
			if d.lastNonWhitespaceChunkSet {
				c.indented = indentChunk(d.chunks[d.lastNonWhitespaceChunkIdx].splitKind)
			}
			d.chunks = append(d.chunks, c)
			if !c.whitespaceOnly {
				d.lastNonWhitespaceChunkIdx = len(d.chunks) - 1
				d.lastNonWhitespaceChunkSet = true
			}
		}
	}
}

// Formatting provides the formatting information on the Dom.
// TODO: was thinking this might be a useful API for instrospection, but not entirely sure.
func (d *Dom) Formatting() *formatting {
	return d.formatting
}

// Format the Dom using the given line limit and indent string.
//
// Formatting is done "outside-in"/"breadth first", so we first format the Dom's top-level
// chunks, then we format their children.
// TODO: I think this makes sense, but want to sanity check this. Should probably also expand
// on this information.
func (d *Dom) Format(lineLimit int, indentStr string) {
	var cost int
	d.formatting = &formatting{
		lineLimit: lineLimit,
		indentStr: indentStr,
	}
	for _, c := range d.chunks {
		c.formatted = true
		cost += c.setIndentStrAndMeasure(indentStr)
		if cost > d.formatting.lineLimit {
			c.split()
		}
		if c.splitKind == SplitKindHard || c.splitKind == SplitKindDouble {
			cost = 0
		}
	}
	var indented bool
	for _, c := range d.chunks {
		if c.text() == "" {
			continue
		}
		c.indented = indented
		indented = indentChunk(c.splitKind)
		if c.child != nil {
			c.child.Format(lineLimit, indentStr)
			for _, c := range c.child.chunks {
				if c.text() == "" {
					continue
				}
				c.indented = indented
				indented = indentChunk(c.splitKind)
			}
		}
	}
	d.formatting.formatted = true
}

// Output returns the output string of the Dom.
//
// TODO: should we simply return the bytes.Buffer here?
func (d *Dom) Output() string {
	var buf bytes.Buffer
	for _, c := range d.chunks {
		buf.WriteString(c.output())
	}
	return buf.String()
}

// Returns the last non-whitespace-only chunk from the Dom.
func (d *Dom) lastNonWhitespaceChunk() *Chunk {
	if len(d.chunks) == 0 && !d.lastNonWhitespaceChunkSet {
		return nil
	}
	return d.chunks[d.lastNonWhitespaceChunkIdx]
}

type formatting struct {
	fmt.Stringer

	lineLimit int
	indentStr string
	formatted bool
}

func (f *formatting) String() string {
	state := "unformatted"
	if f.formatted {
		state = "formatted"
	}
	return fmt.Sprintf("state: %s, line limit %d, indent string %q", state, f.lineLimit, f.indentStr)
}

// Formatted returns if the formatting has been applied.
func (f *formatting) Formatted() bool {
	return f.formatted
}

// LineLimit returns the line limit of the formatting.
func (f *formatting) LineLimit() int {
	return f.lineLimit
}

// IndentString returns the indent string of the formatting.
func (f *formatting) IndentString() string {
	return f.indentStr
}

func indentChunk(splitKind SplitKind) bool {
	return slices.Contains([]SplitKind{SplitKindHard, SplitKindDouble}, splitKind)
}
