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
	"iter"
)

const (
	kindNone kind = iota //nolint:unused

	kindText     // Ordinary text.
	kindSpace    // All spaces (U+0020).
	kindBreak    // All newlines (U+000A).
	kindGroup    // See [Group].
	kindIndent   // See [Indent].
	kindUnindent // See [Unindent].
)

// kind is a kind of [tag].
type kind byte

// dom is a source code DOM that contains formatting tags.
type dom []tag

// cursor is a recursive iterator over a [dom].
//
// See [dom.cursor].
type cursor iter.Seq2[*tag, cursor]

// tag is a single tag within a [doc].
type tag struct {
	text  string
	limit int // Used by kind == tagGroup.

	kind   kind
	cond   Cond
	broken bool

	width, column int // See layout.go.
	children      int // Number of children that follow in a [dom].
}

// add applies a set of tag funcs to this doc.
func (d *dom) add(tags ...Tag) {
	for _, tag := range tags {
		if tag != nil {
			tag(d)
		}
	}
}

// push appends a tag with children.
func (d *dom) push(tag tag, body func(Sink)) {
	*d = append(*d, tag)

	if body != nil {
		n := len(*d)
		body(d.add)
		(*d)[n-1].children = len(*d) - n
	}
}

// cursor returns an iterator over the top-level tags of this doc.
//
// The iterator yields tags along with another iterator over that tag's
// children.
func (d *dom) cursor() cursor {
	return func(yield func(*tag, cursor) bool) {
		d := *d
		for i := 0; i < len(d); i++ {
			tag := &d[i]
			children := d[i+1 : i+tag.children+1]
			i += len(children)

			if !yield(tag, children.cursor()) {
				return
			}
		}
	}
}

// renderIf returns whether a condition is true.
func (t *tag) renderIf(cond Cond) bool {
	return t.cond == Always || t.cond == cond
}

// shouldMerge calculates whether adjacent tags should be merged together.
//
// Returns which of the tags should be kept based on whitespace merge semantics.
//
// Never returns false, false.
func shouldMerge(a, b *tag) (keepA, keepB bool) {
	switch {
	case a.kind == kindSpace && b.kind == kindBreak:
		return false, true
	case a.kind == kindBreak && b.kind == kindSpace:
		return true, false

	case a.kind == kindSpace && b.kind == kindSpace,
		a.kind == kindBreak && b.kind == kindBreak:
		bIsWider := len(a.text) < len(b.text)
		return !bIsWider, bIsWider
	}

	return true, true
}
