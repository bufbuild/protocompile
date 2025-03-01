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
	"fmt"
	"strings"

	"github.com/rivo/uniseg"
)

const (
	space = " "
)

// TODO: considering whether the panics in Chunk are too strict... I am of the opinion that
// we should be inflexible with this behaviour because we want this API to inform the use
// patterns strictly, but open to the opinions of others.

// Chunk represents a line of text with some configurations around indendation and splitting
// (what whitespace should follow, if any).
type Chunk struct {
	rawText          string
	whitespaceOnly   bool
	hasText          bool
	child            *Dom
	indented         bool
	indentDepth      int
	indentStr        string
	formatted        bool
	splitKind        SplitKind
	spaceIfUnsplit   bool
	splitKindIfSplit SplitKind // Restricted to SplitKindHard or SplitKindDouble
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

// SetText sets the text of the Chunk.
func (c *Chunk) SetText(rawText string) {
	c.rawText = rawText
	c.hasText = true
	c.whitespaceOnly = strings.TrimSpace(c.rawText) == ""
}

// WhitespaceOnly returns whether the Chunk's raw text is whitespace-only.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) WhitespaceOnly() bool {
	if !c.hasText {
		panic("protocompile/printer/dom: called WhitespaceOnly() on unset Chunk")
	}
	return c.whitespaceOnly
}

// SplitKind returns the Chunk's current SplitKind.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) SplitKind() SplitKind {
	if !c.hasText {
		panic("protocompile/printer/dom: called SplitKind() on unset Chunk")
	}
	return c.splitKind
}

// SetSplitKind sets the SplitKind of the Chunk.
func (c *Chunk) SetSplitKind(splitKind SplitKind) {
	c.splitKind = splitKind
}

// SetChild sets the Chunk's child.
func (c *Chunk) SetChild(child *Dom) {
	c.child = child
}

// SetIndentDepth sets the indent depth of the Chunk.
func (c *Chunk) SetIndentDepth(indentDepth int) {
	c.indentDepth = indentDepth
}

// SetSpaceIfUnsplit sets whether the Chunk will output a space if it is not split.
func (c *Chunk) SetSpaceIfUnsplit(spaceIfUnsplit bool) {
	c.spaceIfUnsplit = spaceIfUnsplit
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

// The Chunk's current text. For formatted chunks, we use indenting and splitting logic to
// control whitespaces, so this will be the raw text with leading and trailing whitespaces
// trimmed. If the chunk is not formatted, then the raw text will be returned.
// This will panic if called on a Chunk where text has not been explicitly set.
func (c *Chunk) text() string {
	if c.formatted {
		return strings.TrimSpace(c.rawText)
	}
	return c.rawText
}

// The Chunk's indent string. Unformatted chunks will not have an indent string. For formatted
// chunks, if the text is whitespace-only, then there will be no indent string. The indent
// string is based on the indent depth, and indent character set on the Chunk.
func (c *Chunk) indent() string {
	if c.indented && c.formatted && !c.whitespaceOnly {
		return strings.Repeat(c.indentStr, c.indentDepth)
	}
	return ""
}

// The Chunk's split string. Unformatted chunks will not have a split string. For formatted
// chunks, if the text is whitespace-only, then there will be no split string. A hard split
// returns a newline, a double hard split returns a double newline, and soft/never split will
// check if a space should follow.
func (c *Chunk) splitString() string {
	if c.formatted && !c.whitespaceOnly {
		switch c.splitKind {
		case SplitKindHard:
			return "\n"
		case SplitKindDouble:
			return "\n\n"
		case SplitKindSoft, SplitKindNever:
			if c.spaceIfUnsplit {
				return space
			}
		}
	}
	return ""
}

// The Chunk's output string.
func (c *Chunk) output() string {
	output := c.indent() + c.text() + c.splitString()
	if c.child != nil {
		for _, chunk := range c.child.chunks {
			output += chunk.output()
		}
	}
	return output
}

// This sets the indent string on the chunk and its children, and measures the first output
// from the chunk and its child, defined as the first contiguous length of text until a line
// break. If a hard or double split is encountered, we cut the measurement and return.
func (c *Chunk) setIndentStrAndMeasure(indentStr string) int {
	c.indentStr = indentStr
	cost := uniseg.StringWidth(c.indent())
	cost += uniseg.StringWidth(c.text())
	switch c.splitKind {
	case SplitKindSoft, SplitKindNever:
		if c.spaceIfUnsplit {
			cost += uniseg.StringWidth(space)
		}
		if c.child != nil {
			for _, chunk := range c.child.chunks {
				if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
					break
				}
				cost += chunk.setIndentStrAndMeasure(indentStr)
			}
		}
	}
	return cost
}

// Split the Chunk. This sets the splitKind to the splitKind when Split and indents the
// chunks of the child Dom based on the split behaviour currently set.
func (c *Chunk) split() {
	switch c.splitKind {
	case SplitKindHard, SplitKindDouble, SplitKindNever:
		return
	}
	c.splitKind = c.splitKindIfSplit
	if c.child != nil {
		last := c.child.lastNonWhitespaceChunk()
		if last != nil && last.splitKind == SplitKindSoft {
			last.splitKind = last.splitKindIfSplit
		}
	}
}
