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
func PrintFile(options Options, file *ast.File) string {
	options = options.withDefaults()
	return dom.Render(options.domOptions(), func(push dom.Sink) {
		trivia := buildTriviaIndex(file.Stream())
		p := &printer{
			trivia:  trivia,
			push:    push,
			options: options,
		}
		p.printFile(file)
	})
}

// Print renders a single declaration to protobuf source text.
//
// For printing entire files, use [PrintFile] instead.
func Print(options Options, decl ast.DeclAny) string {
	options = options.withDefaults()
	return dom.Render(options.domOptions(), func(push dom.Sink) {
		p := &printer{
			push:    push,
			options: options,
		}
		p.printDecl(decl)
		p.flushPending(gapNone)
	})
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	options Options
	trivia  *triviaIndex
	pending []token.Token
	push    dom.Sink
}

// printFile prints all declarations in a file, zipping with trivia slots.
func (p *printer) printFile(file *ast.File) {
	trivia := p.trivia.scopeTrivia(0)
	decls := seq.Indexer[ast.DeclAny](file.Decls())
	if p.options.Format {
		sorted := seq.ToSlice(decls)
		sortFileDeclsForFormat(sorted)
		decls = seq.NewFunc(len(sorted), func(i int) ast.DeclAny {
			return sorted[i]
		})
	}
	p.printScopeDecls(trivia, decls)
	p.flushPending(gapNone)
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
		p.pending = append(p.pending, att.leading...)
	}

	// Flush pending trivia and emit the gap, then the token text.
	//
	// In non-format mode, the gap is only a fallback for synthetic tokens
	// (those with no trivia entry). When trivia exists (even if the leading
	// slice is empty), the trivia replaces the gap entirely.
	//
	// In format mode, the gap is always used: whitespace tokens are discarded
	// and the gap provides canonical spacing.
	if hasTrivia && !p.options.Format {
		p.flushPending(gapNone)
	} else {
		p.flushPending(gap)
	}
	p.push(dom.Text(tok.Text()))

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
	p.pending = append(p.pending, tokens...)
}

// emit writes non-whitespace text to the output, flushing pending whitespace first.
func (p *printer) emit(s string) {
	if len(s) > 0 {
		p.flushPending(gapNone)
		p.push(dom.Text(s))
	}
}

// flushPending flushes accumulated trivia from the pending buffer and emits
// the gap. This is the format-aware flush point.
//
// Non-format mode: concatenate all token text into one string, push as a
// single dom.Text. If pending is empty and gap != gapNone, emit the gap
// (synthetic token fallback). Same behavior as before.
//
// Format mode: walk pending tokens, emit comment tokens verbatim (with a
// newline before each if needed), discard whitespace (Space) tokens. Then
// emit the gap. This replaces original whitespace with gap-based spacing
// while preserving comments.
func (p *printer) flushPending(gap gapStyle) {
	if p.options.Format {
		p.flushPendingFormat(gap)
		return
	}
	p.flushPendingNonFormat(gap)
}

// flushPendingNonFormat handles the non-format flush path.
func (p *printer) flushPendingNonFormat(gap gapStyle) {
	if len(p.pending) > 0 {
		var buf strings.Builder
		for _, tok := range p.pending {
			buf.WriteString(tok.Text())
		}
		p.push(dom.Text(buf.String()))
		p.pending = p.pending[:0]
		return
	}

	// No pending trivia: this is a synthetic token, emit the gap directly.
	switch gap {
	case gapNewline:
		p.push(dom.Text("\n"))
	case gapSpace:
		p.push(dom.Text(" "))
	case gapSoftline:
		p.push(dom.TextIf(dom.Flat, " "))
		p.push(dom.TextIf(dom.Broken, "\n"))
	}
}

// flushPendingFormat handles the format flush path.
func (p *printer) flushPendingFormat(gap gapStyle) {
	// Walk pending tokens: emit comments verbatim, discard whitespace.
	emittedAny := false
	for _, tok := range p.pending {
		if tok.Kind() == token.Comment {
			// Emit a newline before the comment if we have already emitted
			// something (i.e. this is not the very first item in the flush).
			if emittedAny {
				p.push(dom.Text("\n"))
			}
			p.push(dom.Text(tok.Text()))
			emittedAny = true
		}
		// Whitespace (Space) tokens are discarded in format mode.
	}
	p.pending = p.pending[:0]

	// Emit the gap.
	switch gap {
	case gapNewline:
		p.push(dom.Text("\n"))
	case gapSpace:
		p.push(dom.Text(" "))
	case gapSoftline:
		p.push(dom.TextIf(dom.Flat, " "))
		p.push(dom.TextIf(dom.Broken, "\n"))
	}
}

// withIndent runs fn with an indented printer, swapping the sink temporarily.
func (p *printer) withIndent(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Indent(strings.Repeat(" ", p.options.TabstopWidth), func(indentSink dom.Sink) {
		p.push = indentSink
		fn(p)
	}))
	p.push = originalPush
}

// withGroup runs fn with a grouped printer, swapping the sink temporarily.
func (p *printer) withGroup(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Group(p.options.MaxWidth, func(groupSink dom.Sink) {
		p.push = groupSink
		fn(p)
	}))
	p.push = originalPush
}
