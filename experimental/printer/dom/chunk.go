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

// Chunk represents a line of text with some configurations around indendation and splitting
// (what whitespace should follow, if any).
type Chunk struct {
	text                      string
	indent                    uint32
	indented                  bool
	splitKind                 SplitKind
	spaceWhenUnsplit          bool
	splitKindIfSplit          SplitKind // Restricted to SplitKindHard or SplitKindDouble
	splitWithParent           bool
	indentWhenSplitWithParent bool
	children                  *Doms
}

// NewChunk constructs a new Chunk.
func NewChunk(text string) *Chunk {
	return &Chunk{text: text, children: NewDoms()}
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

func (c *Chunk) Children() *Doms {
	return c.children
}

// Setters

func (c *Chunk) SetIndent(indent uint32) {
	c.indent = indent
}

func (c *Chunk) SetIndented(indented bool) {
	//switch c.splitKind {
	//case SplitKindSoft, SplitKindNever:
	//	panic("invalid SplitKind for indented")
	//}
	c.indented = indented
}

func (c *Chunk) SetSplitKind(splitKind SplitKind) {
	c.splitKind = splitKind
}

func (c *Chunk) SetSpaceWhenUnsplit(spaceWhenUnsplit bool) {
	c.spaceWhenUnsplit = spaceWhenUnsplit
}

func (c *Chunk) SetSplitKindIfSplit(splitKindIfSplit SplitKind) {
	if splitKindIfSplit != SplitKindHard && splitKindIfSplit != SplitKindDouble {
		panic("invalid splitKindIfSplit")
	}
	c.splitKindIfSplit = splitKindIfSplit
}

func (c *Chunk) SetSplitWithParent(splitWithParent bool) {
	c.splitWithParent = splitWithParent
}

func (c *Chunk) SetIndentWhenSplitWithParent(indentWhenSplitWithParent bool) {
	if !c.splitWithParent {
		panic("can only set indentWhenSplitWithParent if splitWithParent is true")
	}
	c.indentWhenSplitWithParent = indentWhenSplitWithParent
}

func (c *Chunk) SetChildren(children *Doms) {
	c.children = children
}

// Private

// Measure the length of the chunk text. Requires an indentSize.
func (c *Chunk) measure(indentSize int) int {
	cost := uniseg.StringWidth(c.text)
	// Only count indent if indented
	if c.indented {
		cost += uniseg.StringWidth(strings.Repeat(strings.Repeat(" ", indentSize), int(c.indent)))
	}
	// If the chunk is soft split, we need to account for whether a space is added also.
	if (c.splitKind == SplitKindSoft || c.splitKind == SplitKindNever) && c.spaceWhenUnsplit {
		cost += uniseg.StringWidth(strings.Repeat(" ", 1))
	}
	// We must also add the length of any children that are not split
out:
	for _, child := range *c.children {
		for _, chunk := range child.chunks {
			if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
				break out
			}
			cost += chunk.measure(indentSize)
		}
	}
	return cost
}

func (c *Chunk) split(child bool) {
	switch c.splitKind {
	case SplitKindHard, SplitKindDouble, SplitKindNever:
		return
	}
	if child {
		c.indented = true
		if c.splitWithParent {
			c.splitKind = c.splitKindIfSplit
			c.indented = c.indentWhenSplitWithParent
		}
	} else {
		c.splitKind = c.splitKindIfSplit
		c.indented = true
		for _, child := range *c.children {
			for _, childChunk := range child.chunks {
				childChunk.split(true)
			}
		}
	}
}
