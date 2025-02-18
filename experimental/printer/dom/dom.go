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

// Dom represents a "block" of source code that can be formatted.
// It is made up of an ordered slice of chunks.
//
// When rendering a dom, we calculate...
//
// We must denote whether this is a formatted Dom or if this is for printing without formatting.
type Dom struct {
	chunks    []*Chunk
	formatted bool
}

// NewDom constructs a new Dom.
func NewDom(chunks []*Chunk) *Dom {
	return &Dom{
		chunks: chunks,
	}
}

func (d *Dom) LastSplitKind() SplitKind {
	if len(d.chunks) > 0 {
		return d.chunks[len(d.chunks)-1].SplitKind()
	}
	return SplitKindUnknown
}

func (d *Dom) format(lineLimit int) {
	if !d.formatted {
		if d.longestLen() > lineLimit {
			d.split()
		}
		for _, c := range d.chunks {
			if c.children != nil {
				for _, child := range *c.children {
					child.format(lineLimit)
				}
			}
		}
	}
}

func (d *Dom) longestLen() int {
	var max int
	var cost int
	for _, c := range d.chunks {
		// Reset the cost if the chunk is already hard split
		if c.splitKind == SplitKindHard || c.splitKind == SplitKindDouble {
			cost = 0
		}
		cost += c.measure()
		if cost > max {
			max = cost
		}
	}
	return max
}

func (d *Dom) split() {
	for _, c := range d.chunks {
		if c.splitKind == SplitKindHard || c.splitKind == SplitKindDouble || c.splitKind == SplitKindNever {
			continue
		}
		c.splitKind = SplitKindHard
		if c.children.first() != nil {
			c.children.first().indented = true
		}
		if c.children.last() != nil {
			c.children.last().splitKind = SplitKindHard
		}
	}
}
