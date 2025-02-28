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
	"strings"
)

// Dom represents a block of text with formatting information. It is a tree of [Chunk]s.
type Dom struct {
	chunks     []*Chunk
	formatting *formatting
}

// NewDom constructs a new Dom.
func NewDom() *Dom {
	return &Dom{}
}

// Insert a Chunk into the Dom. Only Chunks that have text set will be inserted, any Chunks
// that have not had text set will be dropped.
func (d *Dom) Insert(chunks ...*Chunk) {
	for _, c := range chunks {
		if c == nil {
			continue
		}
		if c.hasText {
			d.chunks = append(d.chunks, c)
		}
	}
}

// First returns the first chunk in the Dom. Returns nil if the Dom is empty.
func (d *Dom) First() *Chunk {
	if len(d.chunks) == 0 {
		return nil
	}
	return d.chunks[0]
}

// TODO: track at insertion time
func (d *Dom) FirstNonWhitespaceChunk() *Chunk {
	if len(d.chunks) == 0 {
		return nil
	}
	for _, c := range d.chunks {
		if strings.TrimSpace(c.text) != "" {
			return c
		}
	}
	return nil
}

// Last returns the last chunk in the Dom. Returns nil if the Dom is empty.
func (d *Dom) Last() *Chunk {
	if len(d.chunks) == 0 {
		return nil
	}
	return d.chunks[len(d.chunks)-1]
}

// TODO: track at insertion time
func (d *Dom) LastNonWhitespaceChunk() *Chunk {
	if len(d.chunks) == 0 {
		return nil
	}
	for i := len(d.chunks) - 1; i >= 0; i-- {
		if strings.TrimSpace(d.chunks[i].text) != "" {
			return d.chunks[i]
		}
	}
	return nil
}

// Formatting returns the formatting used for the current Dom. This is nil if no formatting
// has been set on the Dom.
func (d *Dom) Formatting() *formatting {
	return d.formatting
}

// SetFormatting sets the Formatting that the Dom and all of its children should use.
func (d *Dom) SetFormatting(lineLimit, indentSize int) {
	d.formatting = &formatting{
		lineLimit:  lineLimit,
		indentSize: indentSize,
	}
	for _, c := range d.chunks {
		if c.child != nil {
			c.child.SetFormatting(lineLimit, indentSize)
		}
	}
}

// Format the Dom. If this is called and no formatting has been set, then this will panic.
func (d *Dom) Format() {
	if d.formatting == nil {
		panic("protocompile/printer/dom: attempted to format Dom with no formatting set")
	}
	if !d.formatting.formatted {
		var cost int
		var indentNext bool
		for _, c := range d.chunks {
			if indentNext {
				c.indented = true
				indentNext = false
			}
			cost += c.length(d.formatting.indentSize)
			if cost > d.formatting.lineLimit {
				c.split()
				// If you're splitting, you always indent the next thing
				indentNext = true
			}
			if c.splitKind == SplitKindHard || c.splitKind == SplitKindDouble {
				cost = 0
				// If something is already split, the next thing is always indented.
				indentNext = true
			}
		}
		// TODO: we are currently splitting from "outside inwards"/"breadth first",
		// so we iterate through the children after we iterate through all the outer chunks.
		// I think this is correct, but want to sanity check this.
		for _, c := range d.chunks {
			if c.child != nil {
				c.child.Format()
			}
		}
		d.formatting.formatted = true
	}
}

// Output returns the output string of the Dom.
func (d *Dom) Output() string {
	var buf bytes.Buffer
	for _, c := range d.chunks {
		buf.WriteString(c.output(d.formatting))
	}
	return buf.String()
}

type formatting struct {
	lineLimit  int
	indentSize int
	formatted  bool
}
