package dom

import (
	"iter"
)

const (
	kindNone kind = iota
	kindText
	kindSpace
	kindBreak
	kindGroup
	kindIndent
	kindUnindent
)

// kind is a kind of [tag].
type kind byte

// doc is is a source code DOM that contains formatting tags.
type doc []tag

// cursor is a recursive iterator.
type cursor iter.Seq2[*tag, cursor]

// tag is a single tag within a [doc].
type tag struct {
	kind  kind
	cond  Cond
	limit int // used by tagGroup
	text  string

	children      int
	width, column int
	broken        bool
}

// add applies a set of tag funcs to this doc.
func (d *doc) add(tags ...Tag) {
	for _, tag := range tags {
		if tag != nil {
			tag(d)
		}
	}
}

// push appends a tag with children.
func (d *doc) push(tag tag, body func(Sink)) {
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
func (d *doc) cursor() cursor {
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

// check returns whether a condition is true.
func (t *tag) check(cond Cond) bool {
	return t.cond == Always || t.cond == cond
}

// shouldMerge calculates whether adjacent tags should be merged together.
//
// Returns which of the tags should be kept based on whitespace merge semantics.
//
// Never returns false, false.
func shouldMerge(a, b *tag) (keepA, keepB bool) {
	// Merge with the most recent text tag if necessary.
	switch {
	case a.kind == kindSpace && b.kind == kindSpace:
		return false, true

	case a.kind == kindBreak && b.kind == kindSpace:
		return true, false

	case a.kind == kindSpace && b.kind == kindSpace,
		a.kind == kindBreak && b.kind == kindBreak:
		bWider := len(a.text) < len(b.text)
		return !bWider, bWider
	}

	return true, true
}
