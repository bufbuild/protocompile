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
	splitKind        SplitKind
	spaceWhenUnsplit bool
	children         *Doms
}

// NewChunk constructs a new Chunk.
func NewChunk(text string, indent uint32, splitKind SplitKind, spaceWhenUnsplit bool) *Chunk {
	return &Chunk{
		text:             text,
		indent:           indent,
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
	return c.indent
}

func (c *Chunk) SpaceWhenUnsplit() bool {
	return c.spaceWhenUnsplit
}

func (c *Chunk) SetChildren(children *Doms) {
	c.children.Insert(children.Contents()...)
}

func (c *Chunk) Children() *Doms {
	return c.children
}

// TODO: remove, for debugging right now only
func (c *Chunk) What() string {
	return c.text
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
	for _, child := range c.children.Contents() {
		for _, chunk := range child.chunks {
			if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
				break out
			}
			cost += chunk.measure()
		}
	}

	return cost
}
