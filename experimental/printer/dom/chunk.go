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
	"strings"

	"github.com/rivo/uniseg"
)

// TODO: considering whether the panics in Chunk are too strict... I am of the opinion that
// we should be inflexible with this behaviour because we want this API to inform the use
// patterns strictly, but open to the opinions of others.

// Chunk represents a line of text with some configurations around indendation and splitting
// (what whitespace should follow, if any).
type Chunk struct {
	text                string
	hasText             bool
	child               *Dom
	indent              uint32
	indented            bool
	splitKind           SplitKind
	spaceWhenUnsplit    bool
	splitKindIfSplit    SplitKind // Restricted to SplitKindHard or SplitKindDouble
	splitWithParent     bool
	indentOnParentSplit bool
}

func NewChunk() *Chunk {
	return &Chunk{}
}

// HasText returns whether this Chunk is has had text set. A Chunk where text has not been
// set will not be included in the [Dom.Output]. This is to differentiate Chunks that have
// an empty string set to its text.
func (c *Chunk) HasText() bool {
	return c.hasText
}

// Text returns the Chunk's current text.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) Text() string {
	if !c.hasText {
		panic("protocompile/printer/dom: called Text() on unset Chunk")
	}
	return c.text
}

// SplitKind returns the Chunk's current SplitKind.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) SplitKind() SplitKind {
	if !c.hasText {
		panic("protocompile/printer/dom: called SplitKind() on unset Chunk")
	}
	return c.splitKind
}

// Indent returns the Chunk's current number of indents. If the Chunk is not indented, then
// this will always return 0.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) Indent() uint32 {
	if !c.hasText {
		panic("protocompile/printer/dom: called SplitKind() on unset Chunk")
	}
	if c.indented {
		return c.indent
	}
	return 0
}

// Child returns the Chunk's child.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) Child() *Dom {
	if !c.hasText {
		panic("protocompile/printer/dom: called SplitKind() on unset Chunk")
	}
	return c.child
}

// SetText sets the text of the Chunk.
func (c *Chunk) SetText(text string) {
	c.text = text
	c.hasText = true
}

// SetIndent sets the indent of the Chunk.
func (c *Chunk) SetIndent(indent uint32) {
	c.indent = indent
}

// SetIndented sets whether the Chunk's output will be indented.
func (c *Chunk) SetIndented(indented bool) {
	c.indented = indented
}

// SetSplitKind sets the SplitKind of the Chunk.
func (c *Chunk) SetSplitKind(splitKind SplitKind) {
	c.splitKind = splitKind
}

// SetSpaceWhenUnsplit sets whether the Chunk will output a space if it is not split.
func (c *Chunk) SetSpaceWhenUnsplit(spaceWhenUnsplit bool) {
	c.spaceWhenUnsplit = spaceWhenUnsplit
}

// SetSplitKindIfSplit sets the SplitKind of the Chunk if it is split. This will panic if it
// is not called with SplitKindHard or SplitKindDouble.
func (c *Chunk) SetSplitKindIfSplit(splitKindIfSplit SplitKind) {
	if splitKindIfSplit != SplitKindHard && splitKindIfSplit != SplitKindDouble {
		panic(fmt.Sprintf(
			"protocompile/printer/dom: called SetSplitKindIfSplit with %s, this is restricted to %s and %s",
			splitKindIfSplit,
			SplitKindHard,
			SplitKindDouble,
		))
	}
	c.splitKindIfSplit = splitKindIfSplit
}

// SetSplitWithParent sets whether the Chunk will be split when the parent is split.
func (c *Chunk) SetSplitWithParent(splitWithParent bool) {
	c.splitWithParent = splitWithParent
}

// SetNoIndentOnParentSplit sets whether the Chunk will be indented when the parent is split.
func (c *Chunk) SetIndentOnParentSplit(indentOnParentSplit bool) {
	c.indentOnParentSplit = indentOnParentSplit
}

// SetChild sets the Chunk's child.
func (c *Chunk) SetChild(child *Dom) {
	c.child = child
}

func (c *Chunk) format(lineLimit, indent int) {
	if c.length(indent) > lineLimit {
		c.split(false)
	}
}

func (c *Chunk) output(indent int) string {
	var buf bytes.Buffer
	buf.WriteString(strings.Repeat(strings.Repeat(space, indent), int(c.Indent())))
	buf.WriteString(c.Text())
	switch c.SplitKind() {
	case SplitKindHard:
		buf.WriteString("\n")
	case SplitKindDouble:
		buf.WriteString("\n\n")
	case SplitKindSoft, SplitKindNever:
		if c.spaceWhenUnsplit {
			buf.WriteString(space)
		}
	}
	if c.child != nil {
		for _, chunk := range c.child.chunks {
			buf.WriteString(chunk.output(indent))
		}
	}
	return buf.String()
}

// Provides the length of the Chunk's output, defined as the contiguous length of text until
// a line break. So this captures the Chunk's text, indents, and if the Chunk is not split,
// the output of any child chunks until a line break is hit.
func (c *Chunk) length(indentSize int) int {
	cost := uniseg.StringWidth(c.text)
	// Only count indent if indented
	if c.indented {
		cost += uniseg.StringWidth(strings.Repeat(strings.Repeat(" ", indentSize), int(c.indent)))
	}
	switch c.splitKind {
	case SplitKindSoft, SplitKindNever:
		if c.spaceWhenUnsplit {
			cost += uniseg.StringWidth(strings.Repeat(" ", 1))
		}
		if c.child != nil {
			for _, chunk := range c.child.chunks {
				if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
					break
				}
				cost += chunk.length(indentSize)
			}
		}
	}
	return cost
}

// Split the Chunk. This sets the splitKind to the splitKind when Split and then checks the
// splitting behaviour of each child.
// Splitting is not a cascading effect down the tree, we measure and split as needed at
// each Chunk.
func (c *Chunk) split(child bool) {
	switch c.splitKind {
	case SplitKindHard, SplitKindDouble, SplitKindNever:
		return
	}
	if child {
		if c.indentOnParentSplit {
			c.indented = true
		}
		if c.splitWithParent {
			c.splitKind = c.splitKindIfSplit
		}
	} else {
		c.splitKind = c.splitKindIfSplit
		c.indented = true
		if c.child != nil {
			for _, chunk := range c.child.chunks {
				chunk.split(true)
			}
		}
	}
}
