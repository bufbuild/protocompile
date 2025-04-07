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
	"math"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// printer holds state for converting a laid-out [dom] into a string.
type printer struct {
	Options

	out strings.Builder
	// Buffered spaces and newlines, for whitespace merging in write().
	spaces, newlines int

	// Indentation state. See indentBy() for usage.
	indent  []byte
	indents []string

	// Whether a value has been popped from indent. This is used to handle the
	// relatively rare case where indentBy is called inside of unindentBy.
	popped bool
}

// render renders a dom with the given options.
func render(options Options, doc *dom) string {
	options = options.WithDefaults()
	l := layout{Options: options}
	l.layout(*doc)

	p := printer{Options: options}
	if options.HTML {
		p.html(doc.cursor())
	} else {
		// Top level group is always broken.
		p.print(Broken, doc.cursor())
	}

	if !strings.HasSuffix(p.out.String(), "\n") {
		p.out.WriteByte('\n')
	}

	return p.out.String()
}

// print prints all of the elements of a cursor that are conditioned on cond.
//
// In other words, this function is called with cond set to whether the
// containing group is broken.
func (p *printer) print(cond Cond, cursor cursor) {
	for tag, cursor := range cursor {
		if !tag.renderIf(cond) {
			continue
		}

		switch tag.kind {
		case kindText:
			p.write(tag.text)
			p.spaces = 0
			p.newlines = 0

		case kindSpace:
			p.spaces = max(p.spaces, len(tag.text))

		case kindBreak:
			p.newlines = max(p.newlines, len(tag.text))

		case kindGroup:
			ourCond := Flat
			if tag.broken {
				ourCond = Broken
			}
			p.print(ourCond, cursor)

		case kindIndent:
			p.withIndent(tag.text, func(p *printer) {
				p.print(cond, cursor)
			})

		case kindUnindent:
			p.withUnindent(func(p *printer) {
				p.print(cond, cursor)
			})
		}
	}
}

// html renders the contents of cursor as pseudo-HTML.
func (p *printer) html(cursor cursor) {
	for tag, cursor := range cursor {
		var cond string
		switch tag.cond {
		case Flat:
			cond = " if=flat"
		case Broken:
			cond = " if=broken"
		}

		switch tag.kind {
		case kindText:
			if cond != "" {
				fmt.Fprintf(&p.out, "<p%v>%q</p>", cond, tag.text)
			} else {
				fmt.Fprintf(&p.out, "%q", tag.text)
			}

		case kindSpace:
			fmt.Fprintf(&p.out, "<sp count=%v%v>", len(tag.text), cond)

		case kindBreak:
			fmt.Fprintf(&p.out, "<br count=%v%v>", len(tag.text), cond)

		case kindGroup:
			name := "span"
			if tag.broken {
				name = "div"
			}

			var limit string
			if tag.limit != math.MaxInt {
				limit = fmt.Sprintf(" limit=%v", tag.limit)
			}

			fmt.Fprintf(&p.out,
				"<%v%v width=%v col=%v%v>",
				name, limit, tag.width, tag.column, cond)
			p.newlines++
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			fmt.Fprintf(&p.out, "</%v>", name)

		case kindIndent:
			fmt.Fprintf(&p.out, "<indent by=%q%v>", tag.text, cond)
			p.newlines++
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			fmt.Fprintf(&p.out, "</indent>")

		case kindUnindent:
			fmt.Fprintf(&p.out, "<unindent%v>", cond)
			p.newlines++
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			fmt.Fprintf(&p.out, "</unindent>")
		}
		p.newlines++
	}
}

// write appends data to the output buffer.
//
// This function automatically handles newline/space merging and indentation.
func (p *printer) write(data string) {
	if p.newlines > 0 {
		for range p.newlines {
			p.out.WriteByte('\n')
		}
		p.newlines = 0
		p.spaces = 0

		p.out.Write(p.indent)
	}

	for range p.spaces {
		p.out.WriteByte(' ')
	}
	p.spaces = 0

	p.out.WriteString(data)
}

// withIndent pushes an indentation string onto the indentation stack for
// the duration of body.
func (p *printer) withIndent(by string, body func(*printer)) {
	prev := p.indent

	if p.popped {
		p.popped = false

		// Need to make a defensive copy here to avoid clobbering any
		// indent popped by withUnindent. Doing this here avoids needing to
		// do the copy except in the case of an indent/unindent/indent sequence.
		//
		// Force a copy in append() below by clipping the slice.
		p.indent = slices.Clip(p.indent)
	}

	p.indent = append(p.indent, by...)
	p.indents = append(p.indents, by)
	body(p)
	if slicesx.PointerEqual(prev, p.indent) {
		// Retain any capacity added by downstream indent calls.
		p.indent = p.indent[:len(prev)]
	} else {
		p.indent = prev
	}
	slicesx.Pop(&p.indents)
}

// withUnindent undoes the most recent call to [printer.withIndent] for the
// duration of body.
func (p *printer) withUnindent(body func(*printer)) {
	if len(p.indents) == 0 {
		body(p)
		return
	}

	prev := p.indent
	popped, _ := slicesx.Pop(&p.indents)
	p.indent = p.indent[:len(p.indent)-len(popped)]
	p.popped = true
	body(p)
	p.popped = false
	p.indent = prev
	p.indents = append(p.indents, popped)
}
