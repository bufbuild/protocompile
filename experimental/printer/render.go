package printer

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/printer/dom"
)

const (
	defaultLineLimit = 80
	defaultIndent    = "  " // 2 spaces
)

func (p *printer) printFile(file ast.File, format bool) {
	for _, doms := range fileToDom(file, format) {
		p.printDoms(doms, format)
	}
}

func (p *printer) printDoms(doms *dom.Doms, format bool) {
	for _, d := range doms.Contents() {
		d.Format(defaultLineLimit)
		p.printDom(d, format)
	}
}

func (p *printer) printDom(d *dom.Dom, format bool) {
	// TODO: implement
	d.Format(defaultLineLimit)
	for _, c := range d.Chunks() {
		if format {
			// TODO: We should only be writing the indent if the previous element is a new line.
			p.WriteString(strings.Repeat(defaultIndent, int(c.Indent())))
			p.WriteString(c.Text())
			switch c.SplitKind() {
			case dom.SplitKindHard:
				p.WriteString("\n")
			case dom.SplitKindDouble:
				p.WriteString("\n\n")
			case dom.SplitKindSoft, dom.SplitKindNever:
				if c.SpaceWhenUnsplit() {
					p.WriteString(" ")
				}
			}
		} else {
			p.WriteString(c.Text())
		}
		p.printDoms(c.Children(), format)
	}
}
