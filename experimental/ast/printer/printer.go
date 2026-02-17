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

package printer

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/dom"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
)

// gapStyle specifies the whitespace intent before a token.
type gapStyle int

const (
	gapNone gapStyle = iota
	gapSpace
	gapNewline
	gapSoftline // gapSoftline inserts a space if the group is flat, or a newline if the group is broken
)

// PrintFile renders an AST file to protobuf source text.
func PrintFile(file *ast.File, opts Options) string {
	opts = opts.withDefaults()
	return dom.Render(opts.domOptions(), func(push dom.Sink) {
		trivia := buildTriviaIndex(file.Stream())
		p := &printer{
			trivia: trivia,
			push:   push,
			opts:   opts,
		}
		p.printFile(file)
	})
}

// Print renders a single declaration to protobuf source text.
//
// For printing entire files, use [PrintFile] instead.
func Print(decl ast.DeclAny, opts Options) string {
	opts = opts.withDefaults()
	return dom.Render(opts.domOptions(), func(push dom.Sink) {
		p := &printer{
			push: push,
			opts: opts,
		}
		p.printDecl(decl)
		p.flushPending()
	})
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	trivia  *triviaIndex
	pending strings.Builder
	push    dom.Sink
	opts    Options
}

// printFile prints all declarations in a file, zipping with trivia slots.
func (p *printer) printFile(file *ast.File) {
	trivia := p.trivia.scopeTrivia(0)
	p.printScopeDecls(trivia, file.Decls())
	p.flushPending()
}

// printToken is the standard entry point for printing a semantic token.
// It emits leading attached trivia, the gap, the token text, and trailing
// attached trivia.
func (p *printer) printToken(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}

	att, hasTrivia := p.trivia.tokenTrivia(tok.ID())
	if hasTrivia {
		p.emitTriviaRun(att.leading)
	} else {
		p.emitGap(gap)
	}

	// Emit the token text.
	p.emit(tok.Text())

	// Emit trailing attached trivia.
	if hasTrivia && len(att.trailing) > 0 {
		p.emitTriviaRun(att.trailing)
	}
}

// printScopeDecls zips trivia slots with declarations.
// If there are more slots than children+1 (due to AST mutations),
// remaining slots are flushed after the last child.
func (p *printer) printScopeDecls(trivia detachedTrivia, decls seq.Indexer[ast.DeclAny]) {
	for i := range decls.Len() {
		p.emitTriviaSlot(trivia, i)
		if i < decls.Len() {
			p.printDecl(decls.At(i))
		}
	}
	p.emitRemainingTrivia(trivia, decls.Len())
}

// emitTriviaSlot emits the detached trivia for slot[i], if it exists.
func (p *printer) emitTriviaSlot(trivia detachedTrivia, i int) {
	if i >= len(trivia.slots) {
		return
	}
	p.emitTriviaRun(trivia.slots[i])
}

// emitRemainingTrivia emits the remaining detached trivia for slot >= i, if it exists.
func (p *printer) emitRemainingTrivia(trivia detachedTrivia, i int) {
	for ; i < len(trivia.slots); i++ {
		p.emitTriviaSlot(trivia, i)
	}
}

// emitTriviaRun appends trivia tokens to the pending buffer.
//
// Used for trailing trivia and slot (detached) trivia. These accumulate in
// the pending buffer so that adjacent pure-newline runs are combined into a
// single kindBreak dom tag, preventing the dom from merging them and
// collapsing blank lines.
func (p *printer) emitTriviaRun(tokens []token.Token) {
	for _, tok := range tokens {
		p.pending.WriteString(tok.Text())
	}
}

// emitGap writes a gap to the output. If pending already has content
// (from preceding natural trivia), the existing whitespace takes precedence
// and the gap is skipped.
func (p *printer) emitGap(gap gapStyle) {
	switch gap {
	case gapNewline:
		p.emit("\n")
	case gapSpace:
		p.emit(" ")
	case gapSoftline:
		p.push(dom.TextIf(dom.Flat, " "))
		p.push(dom.TextIf(dom.Broken, "\n"))
	}
}

// emit writes non-whitespace text to the output, flushing pending whitespace first.
func (p *printer) emit(s string) {
	if len(s) > 0 {
		p.flushPending()
		p.push(dom.Text(s))
	}
}

// flushPending flushes accumulated whitespace from the pending buffer as a
// single dom.Text node.
func (p *printer) flushPending() {
	if p.pending.Len() > 0 {
		p.push(dom.Text(p.pending.String()))
		p.pending.Reset()
	}
}

// withIndent runs fn with an indented printer, swapping the sink temporarily.
func (p *printer) withIndent(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Indent(strings.Repeat(" ", p.opts.TabstopWidth), func(indentSink dom.Sink) {
		p.push = indentSink
		fn(p)
	}))
	p.push = originalPush
}

// withGroup runs fn with a grouped printer, swapping the sink temporarily.
func (p *printer) withGroup(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Group(p.opts.MaxWidth, func(groupSink dom.Sink) {
		p.push = groupSink
		fn(p)
	}))
	p.push = originalPush
}
