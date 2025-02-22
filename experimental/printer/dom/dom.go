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

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

const (
	space = " "
)

// Dom represents a block of text with formatting information. It is a tree of [Chunk]s.
type Dom struct {
	chunks    []*Chunk
	formatted bool
}

// NewDom constructs a new Dom.
func NewDom() *Dom {
	return &Dom{}
}

// Insert a Chunk into the Dom. Only Chunks that have text set will be inserted, any Chunks
// that have not had text set will be dropped.
func (d *Dom) Insert(chunks ...*Chunk) {
	for _, c := range chunks {
		if c.hasText {
			d.chunks = append(d.chunks, c)
		}
	}
}

// LastSplitKind returns the SplitKind of the last chunk in the Dom.
func (d *Dom) LastSplitKind() SplitKind {
	chunk, ok := slicesx.Last(d.chunks)
	if ok {
		return chunk.SplitKind()
	}
	return SplitKindUnknown
}

// TODO: is the behaviour of Output with respect to formatting too strict?

// Output returns the output string of the Dom.
// If format is set to true, then the output will be formatted based on the given line limit
// and indent size.
//
// Once Output has been called with format=true, then the Dom will be formatted forever, and
// calling Output with format=false will result in a panic.
func (d *Dom) Output(format bool, lineLimit, indentSize int) string {
	var buf bytes.Buffer
	// We need to format the dom -- since there could still be a contiguous line across chunks.
	if format {
		d.format(lineLimit, indentSize)
	} else {
		if d.formatted {
			panic("protocompile/printer/dom: called Output with format=falses on a formatted Dom")
		}
	}
	for _, c := range d.chunks {
		buf.WriteString(c.output(format, indentSize))
	}
	return buf.String()
}

func (d *Dom) format(lineLimit, indentSize int) {
	if !d.formatted {
		// Measure contiguous chunks
		var cost int
		var remove []int
		for i, c := range d.chunks {
			if c.onlyOutputUnformatted {
				remove = append(remove, i)
				continue
			}
			cost += c.length(indentSize)
			if cost > lineLimit {
				c.split(false)
			}
			if c.splitKind == SplitKindHard || c.splitKind == SplitKindDouble {
				// Reset the cost for any non-contiguous chunk
				cost = 0
			}
		}
		if len(remove) > 0 {
			var last int
			var cleanedChunks []*Chunk
			for _, i := range remove {
				cleanedChunks = append(cleanedChunks, d.chunks[last:i]...)
				last = i + 1
				if last > len(d.chunks) {
					panic("protocompile/printer/dom: attempted to remove an out-of-range chunk while formatting")
				}
			}
			d.chunks = cleanedChunks
		}
		d.formatted = true
	}
}
