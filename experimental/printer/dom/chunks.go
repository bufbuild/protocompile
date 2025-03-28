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

// TODO: do we need some kind of Zero concept for Chunks (rather than just relying on nil checks)?

// Chunks represents a group of [Chunk]s that are formatted together.
//
// To facilitate splitting logic in Chunks, we keep track of the last non whitespace-only
// [Chunk] inserted.
type Chunks struct {
	chunks                 []*Chunk
	lastNonWhitespaceChunk *Chunk
}

// NewChunks creates a new empty Chunks.
func NewChunks() *Chunks {
	return &Chunks{}
}

// Insert inserts the given [Chunk]s.
// Empty/nil [Chunk]s are dropped.
func (c *Chunks) Insert(chunks ...*Chunk) {
	for _, chunk := range chunks {
		if chunk != nil {
			c.chunks = append(c.chunks, chunk)
			if !chunk.whitespaceOnly {
				c.lastNonWhitespaceChunk = chunk
			}
			if chunk.child != nil {
				// If there is a child, then we must walk down the tree to get the last non whitespace-only [Chunk].
				if chunk.child.LastNonWhitespaceOnlyChunk() != nil {
					c.lastNonWhitespaceChunk = chunk.child.LastNonWhitespaceOnlyChunk()
				}
			}
		}
	}
}

// LastNonWhitespaceOnlyChunk is the last non whitespace-only [Chunk] in the group of chunks.
//
// TODO: would be useful not having to check nil when this is called with a Zero concept.
// TODO: this is currently being called in dom_test for a small toy parser implementation,
// however, I'm not sure we need to expose this. Otherwise, this is mostly used to handle
// splitting behaviour on [Chunk].
func (c *Chunks) LastNonWhitespaceOnlyChunk() *Chunk {
	return c.lastNonWhitespaceChunk
}

// Formats the group of chunks based on the given line limit and indent string.
func (c *Chunks) format(lineLimit int, indentStr string) {
	var cost int
	for _, chunk := range c.chunks {
		cost += chunk.setIndentStrAndMeasure(indentStr)
		if cost > lineLimit {
			c.split()
		}
		if chunk.splitKind == SplitKindHard || chunk.splitKind == SplitKindDouble {
			cost = 0
		}
	}
	for _, c := range c.chunks {
		c.formatted = true
		if c.child != nil {
			c.child.format(lineLimit, indentStr)
		}
	}
}

// TODO: same question as on [Chunk], should this be named "render"?
func (c *Chunks) output() string {
	var output string
	var indented bool
	for _, c := range c.chunks {
		var text string
		text, indented = c.output(indented)
		output += text
	}
	return output
}

func (c *Chunks) split() {
	for _, c := range c.chunks {
		c.split()
	}
}
