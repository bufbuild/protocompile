package dom

import (
	"bytes"
	"fmt"
	"math"
	"slices"

	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/ext/unsafex"
)

type printer struct {
	Options

	// Not strings.Builder so we can delete from the slice.
	out byteWriter

	indent  []byte
	indents []string
	popped  bool

	spaces, newlines int
}

func render(options Options, doc *doc) string {
	if options.MaxWidth == 0 {
		options.MaxWidth = math.MaxInt
	}
	if options.TabstopWidth == 0 {
		options.TabstopWidth = 4
	}

	l := layout{Options: options}
	l.do(*doc)

	p := printer{Options: options}
	if options.HTML {
		p.html(doc.cursor())
	} else {
		p.print(Broken, doc.cursor())
		p.out = p.out[:len(p.out)-p.spaces]
	}

	if !bytes.HasSuffix(p.out, []byte("\n")) {
		p.out = append(p.out, '\n')
	}

	// π.out is not written to after this function returns.
	return unsafex.StringAlias(p.out)
}

func (p *printer) print(cond Cond, cursor cursor) {
	for tag, cursor := range cursor {
		if !tag.check(cond) {
			continue
		}

		switch tag.kind {
		case kindText:
			p.writeIndent(false)

			p.out = append(p.out, tag.text...)
			p.spaces = 0
			p.newlines = 0

		case kindSpace:
			if p.newlines > 0 {
				continue
			}

			for i := p.spaces; i < len(tag.text); i++ {
				p.out = append(p.out, ' ')
				p.spaces++
			}

		case kindBreak:
			p.out = p.out[:len(p.out)-p.spaces]
			p.spaces = 0

			for i := p.newlines; i < len(tag.text); i++ {
				p.out = append(p.out, '\n')
				p.newlines++
			}

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

func (p *printer) html(cursor cursor) {
	for tag, cursor := range cursor {
		var cond string
		switch tag.cond {
		case Flat:
			cond = " if=flat"
		case Broken:
			cond = " if=broken"
		}

		p.writeIndent(true)
		switch tag.kind {
		case kindText:
			if cond != "" {
				fmt.Fprintf(&p.out, "<p%v>%q</p>\n", cond, tag.text)
			} else {
				fmt.Fprintf(&p.out, "%q\n", tag.text)
			}

		case kindSpace:
			fmt.Fprintf(&p.out, "<sp count=%v%v>\n", len(tag.text), cond)

		case kindBreak:
			fmt.Fprintf(&p.out, "<br count=%v%v>\n", len(tag.text), cond)

		case kindGroup:
			name := "span"
			if tag.broken {
				name = "div"
			}

			var limit string
			if tag.limit != math.MaxInt {
				limit = fmt.Sprintf(" limit=%v", tag.limit)
			}

			fmt.Fprintf(&p.out, "<%v%v width=%v col=%v%v>\n", name, limit, tag.width, tag.column, cond)
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			p.writeIndent(true)
			fmt.Fprintf(&p.out, "</%v>\n", name)

		case kindIndent:
			fmt.Fprintf(&p.out, "<indent by=%q%v>\n", tag.text, cond)
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			p.writeIndent(true)
			fmt.Fprintf(&p.out, "</indent>\n")

		case kindUnindent:
			fmt.Fprintf(&p.out, "<unindent%v>\n", cond)
			p.withIndent("    ", func(p *printer) { p.html(cursor) })
			p.writeIndent(true)
			fmt.Fprintf(&p.out, "</unindent>\n")
		}
	}
}

func (p *printer) writeIndent(force bool) {
	if force || p.newlines > 0 {
		p.newlines = 0
		p.out = append(p.out, p.indent...)
	}
}

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
	p.indent = prev
	p.indents = append(p.indents, popped)
}

type byteWriter []byte

// Write implements [io.Write].
func (w *byteWriter) Write(buf []byte) (int, error) {
	*w = append(*w, buf...)
	return len(buf), nil
}
