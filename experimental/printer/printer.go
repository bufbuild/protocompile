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
	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/dom"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
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

// printer is the internal state for printing AST nodes.
// It tracks a cursor position in the token stream to preserve
// whitespace and comments between semantic tokens.
type printer struct {
	cursor  *token.Cursor
	push    dom.Sink
	opts    Options
	lastTok token.Token // Tracks last printed token for gap logic
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
		// printToken in printDecl will flush whitespace from cursor to the first token.
		// We don't need separate whitespace handling here.
		p.printDecl(d)
	}
	p.flushRemaining()
}

// printToken emits a token with gap-aware spacing.
// For synthetic tokens, it applies appropriate spacing based on the previous token.
// For original tokens, it flushes whitespace/comments from the cursor.
func (p *printer) printToken(tok token.Token) {
	if tok.IsZero() {
		return
	}

	if tok.IsSynthetic() {
		// For synthetic tokens, apply gap based on context.
		p.applySyntheticGap(tok)
		p.push(dom.Text(tok.Text()))
	} else {
		// For original tokens, flush whitespace from cursor to this token.
		// We only emit the whitespace immediately preceding the target token.
		// Any whitespace before skipped (deleted) content is discarded.
		if p.cursor != nil {
			targetSpan := tok.Span()
			wsStart, wsEnd := -1, -1
			for skipped := range p.cursor.RestSkippable() {
				if !skipped.IsSynthetic() && skipped.Span().Start >= targetSpan.Start {
					break
				}
				if skipped.Kind().IsSkippable() {
					span := skipped.Span()
					if wsStart < 0 {
						wsStart = span.Start
					}
					wsEnd = span.End
				} else {
					// Hit a non-skippable token that we're skipping over.
					// Discard accumulated whitespace - it belongs to the skipped content.
					wsStart, wsEnd = -1, -1
				}
			}
			if wsStart >= 0 {
				wsSpan := source.Span{File: p.cursor.Context().File, Start: wsStart, End: wsEnd}
				p.push(dom.Text(wsSpan.Text()))
			}
			// Advance cursor past the semantic token we're about to print
			p.cursor.NextSkippable()
		}
		p.push(dom.Text(tok.Text()))
	}

	p.lastTok = tok
}

// applySyntheticGap emits appropriate spacing before a synthetic token
// based on what was previously printed.
func (p *printer) applySyntheticGap(current token.Token) {
	if p.lastTok.IsZero() {
		return
	}

	lastKw := p.lastTok.Keyword()
	currentKw := current.Keyword()

	// After semicolon or closing brace: newline needed
	if lastKw == keyword.Semi || lastKw == keyword.RBrace {
		p.push(dom.Text("\n"))
		return
	}

	// After opening BRACE (body context): newline
	// Check BOTH leaf (LBrace) and fused (Braces) forms
	if lastKw == keyword.LBrace || lastKw == keyword.Braces {
		if !(currentKw == keyword.RBrace || currentKw == keyword.Braces) {
			p.push(dom.Text("\n"))
		}
		return
	}
	// Tight gaps: no space around dots
	if currentKw == keyword.Dot || lastKw == keyword.Dot {
		return
	}
	// Before punctuation: no space (semicolons, commas)
	if currentKw == keyword.Semi || currentKw == keyword.Comma {
		return
	}
	// After open paren/bracket (inline context): no space
	// Check BOTH leaf and fused forms
	if lastKw == keyword.LParen || lastKw == keyword.Parens ||
		lastKw == keyword.LBracket || lastKw == keyword.Brackets {
		return
	}
	// Before close paren/bracket/brace: no space
	// Check both leaf keywords and text (for fused tokens which return the fused keyword)
	if currentKw == keyword.RParen || currentKw == keyword.RBracket || currentKw == keyword.RBrace {
		return
	}
	// For fused close tokens, Keyword() returns the fused form (e.g., Brackets instead of RBracket)
	// So also check by text
	currentText := current.Text()
	if currentText == ")" || currentText == "]" || currentText == "}" {
		return
	}
	// Default: space between tokens
	p.push(dom.Text(" "))
}

// flushSkippableUntil emits all whitespace/comments from the cursor up to the target token.
// This does NOT advance the cursor past the target - used for fused brackets where we
// need to handle the cursor specially.
// Only emits whitespace immediately preceding the target; whitespace before skipped content
// is discarded.
func (p *printer) flushSkippableUntil(target token.Token) {
	if p.cursor == nil || target.IsSynthetic() {
		return
	}

	targetSpan := target.Span()
	wsStart, wsEnd := -1, -1
	for skipped := range p.cursor.RestSkippable() {
		if !skipped.IsSynthetic() && skipped.Span().Start >= targetSpan.Start {
			break
		}
		if skipped.Kind().IsSkippable() {
			span := skipped.Span()
			if wsStart < 0 {
				wsStart = span.Start
			}
			wsEnd = span.End
		} else {
			// Hit a non-skippable token that we're skipping over.
			// Discard accumulated whitespace - it belongs to the skipped content.
			wsStart, wsEnd = -1, -1
		}
	}
	if wsStart >= 0 {
		wsSpan := source.Span{File: p.cursor.Context().File, Start: wsStart, End: wsEnd}
		p.push(dom.Text(wsSpan.Text()))
	}
}

// printFusedBrackets handles fused bracket pairs (parens, brackets) specially.
// When NextSkippable is called on an open bracket, it jumps past the close bracket.
// This function preserves whitespace by using a child cursor for the bracket contents.
func (p *printer) printFusedBrackets(brackets token.Token, printContents func(child *printer)) {
	if brackets.IsZero() {
		return
	}

	openTok, closeTok := brackets.StartEnd()

	p.emitOpen(openTok)

	child := p.childWithCursor(p.push, brackets, openTok)
	printContents(child)
	child.flushRemaining()

	p.emitClose(closeTok, openTok)
}

// text emits raw text without cursor tracking.
// Used for synthetic content or manual formatting.
func (p *printer) text(s string) {
	p.push(dom.Text(s))
}

// newline emits a newline character.
func (p *printer) newline() {
	p.push(dom.Text("\n"))
}

// flushRemaining emits any remaining skippable tokens from the cursor.
// Only emits trailing whitespace after the last printed content.
// Whitespace before any remaining non-skippable (unprinted) content is discarded.
func (p *printer) flushRemaining() {
	if p.cursor == nil {
		return
	}

	wsStart, wsEnd := -1, -1
	for tok := range p.cursor.RestSkippable() {
		if tok.Kind().IsSkippable() {
			span := tok.Span()
			if wsStart < 0 {
				wsStart = span.Start
			}
			wsEnd = span.End
		} else {
			// Hit a non-skippable token that we're not printing.
			// Discard accumulated whitespace - it belongs to unprinted content.
			wsStart, wsEnd = -1, -1
		}
	}
	if wsStart >= 0 {
		wsSpan := source.Span{File: p.cursor.Context().File, Start: wsStart, End: wsEnd}
		p.push(dom.Text(wsSpan.Text()))
	}
}

// childWithCursor creates a child printer with a cursor over the fused token's children.
// The lastTok is set to the open bracket for proper gap context.
func (p *printer) childWithCursor(push dom.Sink, brackets token.Token, open token.Token) *printer {
	child := &printer{
		push:    push,
		opts:    p.opts,
		lastTok: open, // Set context so first child token knows it follows '{'
	}
	if !brackets.IsLeaf() && !open.IsSynthetic() {
		child.cursor = brackets.Children()
	}
	return child
}

// emitOpen prints an open token with proper whitespace handling.
// For fused tokens, this does NOT advance the cursor (caller must handle that).
func (p *printer) emitOpen(open token.Token) {
	if open.IsSynthetic() {
		p.applySyntheticGap(open)
	} else {
		p.flushSkippableUntil(open)
	}
	p.push(dom.Text(open.Text()))
	p.lastTok = open
}

// emitClose prints a close token and advances the parent cursor.
func (p *printer) emitClose(closeToken token.Token, openToken token.Token) {
	// For synthetic close tokens, apply gap logic (e.g., newline after last semicolon)
	if closeToken.IsSynthetic() {
		p.applySyntheticGap(closeToken)
	}
	p.push(dom.Text(closeToken.Text()))
	p.lastTok = closeToken
	// Advance parent cursor past the whole fused pair
	if p.cursor != nil && !openToken.IsSynthetic() {
		p.cursor.NextSkippable()
	}
}
