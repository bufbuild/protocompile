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

// TODO list
//
// - improve docs
// - maybe refactor with report/width.go

type Doms struct {
	doms []*Dom
}

// NewDoms constructs a new Doms.
func NewDoms() *Doms {
	return &Doms{}
}

// Insert will only add the Dom if it contains chunks. A Dom without chunks will not be inserted.
func (d *Doms) Insert(doms ...*Dom) {
	for _, dom := range doms {
		if len(dom.chunks) == 0 {
			continue
		}
		d.doms = append(d.doms, dom)
	}
}

func (d *Doms) Contents() []*Dom {
	return d.doms
}

// TODO: remove, for debugging right now only
func (d *Doms) What() string {
	var what string
	for _, dom := range d.doms {
		what += dom.What() + " "
	}
	return what
}

// dom represents a "block" of source code that can be formatted.
// It is made up of an ordered slice of chunks.
//
// When rendering a dom, we calculate...
//
// We must denote whether this is a formatted Dom or if this is for printing without formatting.
type Dom struct {
	chunks []*Chunk
	format bool
}

// NewDom constructs a new Dom.
func NewDom(chunks []*Chunk, format bool) *Dom {
	return &Dom{
		chunks: chunks,
		format: format,
	}
}

// TODO: remove, for debugging right now only
func (d *Dom) What() string {
	var what string
	for _, c := range d.chunks {
		what += c.What()
	}
	return what
}

// Format the Dom.
func (d *Dom) Format(lineLimit int) {
	if d.format {
		if d.longestLen() > lineLimit {
			d.split()
		}
		for _, c := range d.chunks {
			if c.children != nil {
				for _, child := range c.children.Contents() {
					child.Format(lineLimit)
				}
			}
		}
	}
}

// Chunks returns the Dom's chunks.
func (d *Dom) Chunks() []*Chunk {
	return d.chunks
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
	if d.format {
		for _, c := range d.chunks {
			// This chunk has already been split or can never be split, simply move on.
			if c.splitKind == SplitKindHard || c.splitKind == SplitKindNever {
				continue
			}
			c.splitKind = SplitKindHard
			c.indent++
			for _, child := range c.children.Contents() {
				child.split()
			}
		}
	}
}
