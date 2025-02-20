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
)

const (
	space = " "
)

// Dom represents a block of text with formatting information. It is a tree of [Chunk]s.
type Dom struct {
	chunks []*Chunk
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
	if len(d.chunks) > 0 {
		return d.chunks[len(d.chunks)-1].SplitKind()
	}
	return SplitKindUnknown
}

// Output returns the output string of the Dom.
// If format is set to true, then the output will be formatted based on the given line limit
// and indent size.
func (d *Dom) Output(format bool, lineLimit, indentSize int) string {
	var buf bytes.Buffer
	for _, c := range d.chunks {
		if format {
			c.format(lineLimit, indentSize)
		}
		buf.WriteString(c.output(indentSize))
	}
	return buf.String()
}
