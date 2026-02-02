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
		// For original tokens, flush whitespace/comments from cursor to this token.
		p.flushSkippableUntil(tok)
		if p.cursor != nil {
			// Advance cursor past the semantic token we're about to print
			p.cursor.NextSkippable()
		}
		p.push(dom.Text(tok.Text()))
	}

	p.lastTok = tok
}

// isOpenBracket returns true if tok is an open bracket (including fused tokens).
func isOpenBracket(tok token.Token) bool {
	kw := tok.Keyword()
	if !kw.IsBrackets() {
		return false
	}
	left, _, joined := kw.Brackets()
	if kw == left {
		return true // Leaf open bracket (LParen, LBracket, LBrace, Lt)
	}
	if kw == joined {
		// Fused bracket - check if this is the open end by comparing IDs
		open, _ := tok.StartEnd()
		return tok.ID() == open.ID()
	}
	return false
}

// isCloseBracket returns true if tok is a close bracket (including fused tokens).
func isCloseBracket(tok token.Token) bool {
	kw := tok.Keyword()
	if !kw.IsBrackets() {
		return false
	}
	_, right, joined := kw.Brackets()
	if kw == right {
		return true // Leaf close bracket (RParen, RBracket, RBrace, Gt)
	}
	if kw == joined {
		// Fused bracket - check if this is the close end by comparing IDs
		_, close := tok.StartEnd()
		return tok.ID() == close.ID()
	}
	return false
}

// applySyntheticGap emits appropriate spacing before a synthetic token
// based on what was previously printed.
func (p *printer) applySyntheticGap(current token.Token) {
	if p.lastTok.IsZero() {
		return
	}

	lastKw := p.lastTok.Keyword()
	currentKw := current.Keyword()

	// Classify last token
	lastIsOpenBrace := lastKw == keyword.LBrace || (lastKw == keyword.Braces && isOpenBracket(p.lastTok))
	lastIsCloseBrace := lastKw == keyword.RBrace || (lastKw == keyword.Braces && isCloseBracket(p.lastTok))
	lastIsOpenParen := isOpenBracket(p.lastTok) && (lastKw == keyword.LParen || lastKw == keyword.Parens)
	lastIsOpenBracket := isOpenBracket(p.lastTok) && (lastKw == keyword.LBracket || lastKw == keyword.Brackets)
	lastIsOpenAngle := isOpenBracket(p.lastTok) && (lastKw == keyword.Lt || lastKw == keyword.Angles)
	lastIsSemi := lastKw == keyword.Semi
	lastIsDot := lastKw == keyword.Dot

	// Classify current token
	currentIsCloseBrace := isCloseBracket(current) && (currentKw == keyword.RBrace || currentKw == keyword.Braces)
	currentIsCloseParen := isCloseBracket(current) && (currentKw == keyword.RParen || currentKw == keyword.Parens)
	currentIsCloseBracket := isCloseBracket(current) && (currentKw == keyword.RBracket || currentKw == keyword.Brackets)
	currentIsCloseAngle := isCloseBracket(current) && (currentKw == keyword.Gt || currentKw == keyword.Angles)
	currentIsSemi := currentKw == keyword.Semi
	currentIsComma := currentKw == keyword.Comma
	currentIsDot := currentKw == keyword.Dot

	// After semicolon or closing brace: newline needed
	if lastIsSemi || lastIsCloseBrace {
		p.push(dom.Text("\n"))
		return
	}

	// After opening BRACE (body context): newline (unless immediately followed by close)
	if lastIsOpenBrace {
		if !currentIsCloseBrace {
			p.push(dom.Text("\n"))
		}
		return
	}

	// Tight gaps: no space around dots
	if currentIsDot || lastIsDot {
		return
	}

	// Before punctuation: no space (semicolons, commas)
	if currentIsSemi || currentIsComma {
		return
	}

	// After open paren/bracket/angle (inline context): no space
	if lastIsOpenParen || lastIsOpenBracket || lastIsOpenAngle {
		return
	}

	// Before close paren/bracket/brace/angle: no space
	if currentIsCloseParen || currentIsCloseBracket || currentIsCloseBrace || currentIsCloseAngle {
		return
	}

	// Default: space between tokens
	p.push(dom.Text(" "))
}

// flushSkippableUntil emits whitespace/comments from the cursor up to target.
// Pass token.Zero to flush all remaining tokens.
//
// When encountering deleted content (non-skippable tokens before target):
//   - Detached comments (preceded by blank line) are preserved
//   - Attached comments (no blank line before deleted content) are discarded
//   - Trailing comments (same line as deleted content) are discarded
func (p *printer) flushSkippableUntil(target token.Token) {
	if p.cursor == nil {
		return
	}

	stopAt := -1
	if !target.IsZero() && !target.IsSynthetic() {
		stopAt = target.Span().Start
	}

	spanStart, spanEnd := -1, -1 // Accumulated whitespace/comment span
	afterDeleted := false        // True after deleted content; skip until newline

	for tok := range p.cursor.RestSkippable() {
		if stopAt >= 0 && !tok.IsSynthetic() && tok.Span().Start >= stopAt {
			break
		}

		// Deleted content: flush detached comments, enter skip mode
		if !tok.Kind().IsSkippable() {
			if spanStart >= 0 {
				text := p.spanText(spanStart, spanEnd)
				if blankIdx := strings.LastIndex(text, "\n\n"); blankIdx >= 0 {
					// Flush detached content BEFORE the blank line separator.
					// The spacing will come entirely from whitespace after the deleted content.
					p.push(dom.Text(text[:blankIdx]))
				}
			}
			spanStart, spanEnd = -1, -1
			afterDeleted = true
			continue
		}

		span := tok.Span()

		if afterDeleted {
			// Skip same-line trailing comment; resume at first newline
			newlineIdx := strings.Index(span.Text(), "\n")
			if newlineIdx < 0 {
				continue
			}
			afterDeleted = false
			spanStart = span.Start + newlineIdx
			spanEnd = span.End
			continue
		}

		// Normal: accumulate span
		if spanStart < 0 {
			spanStart = span.Start
		}
		spanEnd = span.End
	}

	if spanStart >= 0 {
		p.push(dom.Text(p.spanText(spanStart, spanEnd)))
	}
}

// spanText returns the source text for the given byte range.
func (p *printer) spanText(start, end int) string {
	return source.Span{File: p.cursor.Context().File, Start: start, End: end}.Text()
}

// flushRemaining emits any remaining skippable tokens from the cursor.
func (p *printer) flushRemaining() {
	p.flushSkippableUntil(token.Zero)
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
	// Apply gap logic when:
	// 1. The close token is synthetic, OR
	// 2. The close token is non-synthetic but follows synthetic content
	//    (e.g., original `{}` with inserted content needs newline before `}`)
	if closeToken.IsSynthetic() || p.lastTok.IsSynthetic() {
		p.applySyntheticGap(closeToken)
	}
	p.push(dom.Text(closeToken.Text()))
	p.lastTok = closeToken
	// Advance parent cursor past the whole fused pair
	if p.cursor != nil && !openToken.IsSynthetic() {
		p.cursor.NextSkippable()
	}
}
