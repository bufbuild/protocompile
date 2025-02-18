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
	"strings"

	"github.com/rivo/uniseg"
)

const (
	space = " "
)

// Chunk represents a line of text with some configurations around indendation and splitting
// (what whitespace should follow, if any).
type Chunk struct {
	text             string
	indent           uint32
	indented         bool
	splitKind        SplitKind
	spaceWhenUnsplit bool
	splitKindIfSplit SplitKind
	children         *Doms
}

// NewChunk constructs a new Chunk.
func NewChunk(text string, indent uint32, indented bool, splitKind SplitKind, spaceWhenUnsplit bool) *Chunk {
	return &Chunk{
		text:             text,
		indent:           indent,
		indented:         indented,
		splitKind:        splitKind,
		spaceWhenUnsplit: spaceWhenUnsplit,
		children:         NewDoms(),
	}
}

func (c *Chunk) SplitKind() SplitKind {
	return c.splitKind
}

func (c *Chunk) Text() string {
	return c.text
}

func (c *Chunk) Indent() uint32 {
	if c.indented {
		return c.indent
	}
	return 0
}

func (c *Chunk) SetChildren(children *Doms) {
	c.children.Insert(*children...)
}

// Measures the length of the chunk.
func (c *Chunk) measure() int {
	cost := uniseg.StringWidth(c.text + strings.Repeat(space, int(c.indent)))
	// If the chunk is soft split, we need to account for whether a space is added also.
	if (c.splitKind == SplitKindSoft || c.splitKind == SplitKindNever) && c.spaceWhenUnsplit {
		cost += uniseg.StringWidth(strings.Repeat(space, 1))
	}
	// We must also add the length of any children that are not split
out:
	for _, child := range *c.children {
		for _, chunk := range child.chunks {
			if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
				break out
			}
			cost += chunk.measure()
		}
	}

	return cost
}
