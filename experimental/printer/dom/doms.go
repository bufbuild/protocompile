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
	"strings"
)

// Doms...
type Doms []*Dom

// NewDoms constructs a new Doms.
func NewDoms() *Doms {
	// TODO: cap/len for performance?
	doms := Doms([]*Dom{})
	return &doms
}

// Insert will only add the Dom if it contains chunks. A Dom without chunks will not be inserted.
func (d *Doms) Insert(doms ...*Dom) {
	for _, dom := range doms {
		if len(dom.chunks) == 0 {
			continue
		}
		*d = append(*d, dom)
	}
}

// Output returns the output string of Doms.
// If format is set to true, then the output will be formatted based on the given line limit
// and indent size.
func (d *Doms) Output(format bool, lineLimit, indent int) string {
	var buf bytes.Buffer
	if format {
		for _, dom := range *d {
			dom.format(lineLimit)
			for _, c := range dom.chunks {
				buf.WriteString(strings.Repeat(strings.Repeat(" ", indent), int(c.Indent())))
				buf.WriteString(c.Text())
				switch c.SplitKind() {
				case SplitKindHard:
					buf.WriteString("\n")
				case SplitKindDouble:
					buf.WriteString("\n\n")
				case SplitKindSoft, SplitKindNever:
					if c.spaceWhenUnsplit {
						buf.WriteString(" ")
					}
				}
				buf.WriteString(c.children.Output(format, lineLimit, indent))
			}
		}
	} else {
		for _, dom := range *d {
			for _, c := range dom.chunks {
				buf.WriteString(c.Text())
				buf.WriteString(c.children.Output(format, lineLimit, indent))
			}
		}
	}
	return buf.String()
}

// TODO: remove?
func (d *Doms) Contents() []*Dom {
	return *d
}

// First chunk in Doms.
func (d *Doms) first() *Chunk {
	if len(*d) > 0 {
		doms := *d
		return doms[0].chunks[0]
	}
	return nil
}

// Last chunk in Doms.
func (d *Doms) last() *Chunk {
	if len(*d) > 0 {
		doms := *d
		return doms[len(doms)-1].chunks[len(doms[len(doms)-1].chunks)-1]
	}
	return nil
}
