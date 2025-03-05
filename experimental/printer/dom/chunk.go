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
	"slices"
	"strings"

	"github.com/rivo/uniseg"
)

const (
	space = " "
)

// TODO: considering whether the panics in Chunk are too strict... I am of the opinion that
// we should be inflexible with this behaviour because we want this API to inform the use
// patterns strictly, but open to the opinions of others.
//
// TODO: do we need some kind of Zero concept for Chunk (rather than just relying on nil checks)?

// Chunk represents a line of text with configurations around indendation and whitespace.
//
// Unformatted Chunks will always return the raw text that was set on it.
// Formatted Chunks will render their output based on the indentation and splitting
// configurations.
//
// Indentation controls the leading whitespace of a Chunk, and it is based on the indent
// string set, indent depth of the `Chunk`, and the splitting pattern at render time.
// Splitting controls the trailing whitespace for a Chunk. A [SplitKindSoft] means that based
// on the line length set at the time of formatting, the Chunk may have a newline, [SplitKindHard]
// or double newline [SplitKindDouble] following its text. However, if the Chunk remains
// unsplit at render time, then a trailing space will be added based on spaceifUnsplit.
type Chunk struct {
	rawText          string
	child            *Chunks
	indentStr        string
	indentDepth      int
	splitKind        SplitKind
	splitKindIfSplit SplitKind // Restricted to SplitKindHard or SplitKindDouble
	spaceIfUnsplit   bool
	indented         bool
	whitespaceOnly   bool
	hasText          bool
	formatted        bool
}

// NewChunk creates a new empty Chunk.
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
func (c *Chunk) SetChild(child *Chunks) {
	if child != nil && len(child.chunks) > 0 {
		c.child = child
	}
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

// The Chunk's current text.
//
// Unformatted chunks will return the raw text set on the chunk.
// Formatted chunks should rely on the indenting and splitting logic set on the chunk, so
// this will return the text with leading and trailing whitespaces trimmed.
func (c *Chunk) text() string {
	if c.formatted {
		return strings.TrimSpace(c.rawText)
	}
	return c.rawText
}

// The Chunk's indent string.
//
// Unformatted chunks will always return empty string.
// Formatted chunks will return an indent string only if the chunk is indented and only
// if it is not a whitespace-only chunk. The indent is based on the indent depth, and indent
// string set on the chunk..
func (c *Chunk) indent() string {
	if c.formatted && c.indented && !c.whitespaceOnly {
		return strings.Repeat(c.indentStr, c.indentDepth)
	}
	return ""
}

// The Chunk's split string (trailing whitespace).
//
// Unformatted chunks will always return empty string.
// Formatted chunks will return a split string only if the chunk is not whitespace-only.
// The split string is based on the split kind currently set and space if unsplit for
// soft/never split chunks.
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
//
// TODO: should this be named "render", since this is technically rendering the string
// output of the Chunk?
func (c *Chunk) output(indented bool) (string, bool) {
	c.indented = indented
	output := c.indent() + c.text() + c.splitString()
	if !c.whitespaceOnly {
		indented = indentChunk(c.splitKind)
	}
	if c.child != nil {
		for _, chunk := range c.child.chunks {
			var text string
			text, indented = chunk.output(indented)
			output += text
		}
	}
	return output, indented
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

// Splits the chunk. If the chunk has a soft split set, then it will set to the configured
// split kind. It will also split the last non whitespace-only chunk of its child.
func (c *Chunk) split() {
	if c.splitKind == SplitKindSoft {
		c.splitKind = c.splitKindIfSplit
	}
	if c.child != nil && c.child.LastNonWhitespaceOnlyChunk() != nil {
		c.child.LastNonWhitespaceOnlyChunk().split()
	}
}

func indentChunk(splitKind SplitKind) bool {
	return slices.Contains([]SplitKind{SplitKindHard, SplitKindDouble}, splitKind)
}
