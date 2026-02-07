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
	})
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	trivia *triviaIndex
	push   dom.Sink
	opts   Options
}

// printFile prints all declarations in a file, zipping with trivia slots.
func (p *printer) printFile(file *ast.File) {
	slots := p.trivia.scopeSlots(0)
	p.printScopeDecls(slots, file.Decls())
	// Emit any remaining trivia at the end of the file (e.g., EOF comments).
	p.emitScopeEnd(0)
}

// printToken is the standard entry point for printing a semantic token.
// It emits leading attached trivia, the gap, the token text, and trailing
// attached trivia.
func (p *printer) printToken(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}

	att, hasTrivia := p.trivia.tokenTrivia(tok.ID())

	// Emit leading attached trivia. When the token was seen during building
	// (natural token), its leading trivia contains the original whitespace
	// (possibly empty for the very first token). For synthetic tokens
	// (hasTrivia=false), fall back to the gap style.
	if hasTrivia {
		p.emitTriviaRun(att.leading)
	} else {
		p.emitGap(gap)
	}

	// Emit the token text.
	p.emit(tok.Text())

	// Emit trailing attached trivia.
	if hasTrivia && len(att.trailing.tokens) > 0 {
		p.emitTriviaRun(att.trailing)
	}
}

// printScopeDecls zips trivia slots with declarations.
// If there are more slots than children+1 (due to AST mutations),
// remaining slots are flushed after the last child.
func (p *printer) printScopeDecls(slots []slot, decls seq.Indexer[ast.DeclAny]) {
	limit := max(len(slots), decls.Len()+1)
	for i := range limit {
		p.emitSlot(slots, i)
		if i < decls.Len() {
			p.printDecl(decls.At(i))
		}
	}
}

// emitSlot emits the detached trivia for slot[i], if it exists.
func (p *printer) emitSlot(slots []slot, i int) {
	if i >= len(slots) {
		return
	}
	for _, run := range slots[i].runs {
		p.emitTriviaRun(run)
	}
}

// emitTriviaRun emits a run of trivia tokens as a single text node.
//
// Concatenating into one string is necessary because the dom merges
// adjacent whitespace-only tags of the same kind, which would collapse
// separate "\n" tokens into a single newline, losing blank lines.
func (p *printer) emitTriviaRun(run triviaRun) {
	var buf strings.Builder
	for _, tok := range run.tokens {
		buf.WriteString(tok.Text())
	}
	p.emit(buf.String())
}

// emitGap emits a gap based on the style.
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

// emit writes text to the output.
func (p *printer) emit(s string) {
	if len(s) > 0 {
		p.push(dom.Text(s))
	}
}

// withIndent runs fn with an indented printer, swapping the sink temporarily.
func (p *printer) withIndent(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Indent(p.opts.Indent, func(indentSink dom.Sink) {
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

// emitScopeEnd emits any trailing trivia at the end of a scope.
func (p *printer) emitScopeEnd(scopeID token.ID) {
	run := p.trivia.getScopeEnd(scopeID)
	if len(run.tokens) > 0 {
		p.emitTriviaRun(run)
	}
}
