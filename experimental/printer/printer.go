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
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// gapStyle specifies the whitespace intent before a token.
type gapStyle int

const (
	gapNone gapStyle = iota
	gapSpace
	gapNewline
)

// PrintFile renders an AST file to protobuf source text.
func PrintFile(file *ast.File, opts Options) string {
	opts = opts.withDefaults()
	return dom.Render(opts.domOptions(), func(push dom.Sink) {
		p := &printer{
			push:   push,
			opts:   opts,
			cursor: file.Stream().Cursor(),
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
		p := newPrinter(push, opts)
		p.printDecl(decl)
	})
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	cursor  *token.Cursor
	push    dom.Sink
	opts    Options
	lastTok token.Token
}

// newPrinter creates a new printer with the given options.
func newPrinter(push dom.Sink, opts Options) *printer {
	return &printer{
		push: push,
		opts: opts,
	}
}

// printFile prints all declarations in a file, preserving whitespace between them.
func (p *printer) printFile(file *ast.File) {
	for d := range seq.Values(file.Decls()) {
		p.printDecl(d)
	}
	p.flushRemaining()
}

// printToken is the standard entry point for printing a semantic token.
func (p *printer) printToken(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}

	// 1. Emit content with gaps/trivia
	p.emitTokenContent(tok, gap)

	// 2. Advance cursor past this token
	if p.cursor != nil && !tok.IsSynthetic() {
		p.cursor.NextSkippable()
	}
}

// emitTokenContent handles the Gap -> Trivia -> Token flow.
// It does NOT advance the cursor.
func (p *printer) emitTokenContent(tok token.Token, gap gapStyle) {
	if tok.IsSynthetic() {
		switch gap {
		case gapNewline:
			p.emit("\n")
		case gapSpace:
			p.emit(" ")
		}
		p.emit(tok.Text())
		p.lastTok = tok
		return
	}

	// Original token: flush trivia (preserves original whitespace/comments) then emit
	p.flushSkippableUntil(tok)
	p.emit(tok.Text())
	p.lastTok = tok
}

// printFusedBrackets handles parens/braces where the AST token is "fused" (skips children).
func (p *printer) printFusedBrackets(brackets token.Token, gap gapStyle, printContents func(child *printer)) {
	if brackets.IsZero() {
		return
	}

	openTok, closeTok := brackets.StartEnd()
	p.emitTokenContent(openTok, gap)
	child := p.childWithCursor(p.push, brackets, openTok)
	printContents(child)
	child.flushRemaining()
	closeGap := gapNone
	if child.lastTok != openTok && isBrace(openTok) {
		closeGap = gapNewline
	}
	p.emitTokenContent(closeTok, closeGap)
	p.lastTok = closeTok

	// Advance parent cursor past the fused group
	if p.cursor != nil && !openTok.IsSynthetic() {
		p.cursor.NextSkippable()
	}
}

// flushSkippableUntil emits whitespace/comments from the cursor up to target.
// Pass token.Zero to flush all remaining tokens.
func (p *printer) flushSkippableUntil(target token.Token) {
	if p.cursor == nil {
		return
	}

	stopAt := -1
	if !target.IsZero() && !target.IsSynthetic() {
		stopAt = target.Span().Start
	}

	spanStart, spanEnd := -1, -1
	afterDeleted := false

	for tok := range p.cursor.RestSkippable() {
		if stopAt >= 0 && !tok.IsSynthetic() && tok.Span().Start >= stopAt {
			break
		}

		if !tok.Kind().IsSkippable() {
			if spanStart >= 0 {
				text := p.spanText(spanStart, spanEnd)
				if blankIdx := strings.LastIndex(text, "\n\n"); blankIdx >= 0 {
					p.emit(text[:blankIdx])
				}
			}
			spanStart, spanEnd = -1, -1
			afterDeleted = true
			continue
		}

		span := tok.Span()
		if afterDeleted {
			if idx := strings.IndexByte(span.Text(), '\n'); idx >= 0 {
				afterDeleted = false
				spanStart = span.Start + idx
				spanEnd = span.End
			}
			continue
		}

		if spanStart < 0 {
			spanStart = span.Start
		}
		spanEnd = span.End
	}

	if spanStart >= 0 {
		p.emit(p.spanText(spanStart, spanEnd))
	}
}

// flushRemaining emits any remaining skippable tokens from the cursor.
func (p *printer) flushRemaining() {
	p.flushSkippableUntil(token.Zero)
}

// spanText returns the source text for the given byte range.
func (p *printer) spanText(start, end int) string {
	return source.Span{File: p.cursor.Context().File, Start: start, End: end}.Text()
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

// childWithCursor creates a child printer with a cursor over the fused token's children.
func (p *printer) childWithCursor(push dom.Sink, brackets token.Token, open token.Token) *printer {
	child := &printer{
		push:    push,
		opts:    p.opts,
		lastTok: open,
	}
	if !brackets.IsLeaf() && !open.IsSynthetic() {
		child.cursor = brackets.Children()
	}
	return child
}

// isBrace returns true if tok is a brace (not paren, bracket, or angle).
func isBrace(tok token.Token) bool {
	kw := tok.Keyword()
	return kw == keyword.LBrace || kw == keyword.RBrace || kw == keyword.Braces
}
