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

// Dom represents a block of text with formatting information. It is a tree of [Chunk]s.
type Dom struct {
	chunks     []*Chunks
	formatting *Formatting
}

// NewDom constructs a new Dom.
func NewDom() *Dom {
	return &Dom{}
}

func (d *Dom) AddChunks(chunks *Chunks) {
	// TODO: add a concept of empty to chunks
	if chunks != nil {
		d.chunks = append(d.chunks, chunks)
	}
}

// Formatting provides the formatting information on the Dom.
//
// TODO: was thinking this might be a useful API for instrospection, but not entirely sure.
// It might also make sense to expose this on Chunks/Chunk instead.
func (d *Dom) Formatting() *Formatting {
	return d.formatting
}

// Format the Dom using the given line limit and indent string.
func (d *Dom) Format(lineLimit int, indentStr string) {
	d.formatting = &Formatting{
		lineLimit: lineLimit,
		indentStr: indentStr,
	}
	for _, c := range d.chunks {
		c.format(lineLimit, indentStr)
	}
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
