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
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// printExpr prints an expression with the specified leading gap.
func (p *printer) printExpr(expr ast.ExprAny, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}

	switch expr.Kind() {
	case ast.ExprKindLiteral:
		tok := expr.AsLiteral().Token
		if !tok.IsLeaf() {
			p.printCompoundString(tok, gap, ctx)
		} else {
			p.printToken(tok, gap, ctx)
		}
	case ast.ExprKindPath:
		p.printPath(expr.AsPath().Path, gap, ctx)
	case ast.ExprKindPrefixed:
		p.printPrefixed(expr.AsPrefixed(), gap, ctx)
	case ast.ExprKindRange:
		p.printExprRange(expr.AsRange(), gap, ctx)
	case ast.ExprKindArray:
		p.printArray(expr.AsArray(), gap, ctx)
	case ast.ExprKindDict:
		p.printDict(expr.AsDict(), gap, ctx)
	case ast.ExprKindField:
		p.printExprField(expr.AsField(), gap, ctx)
	}
}

// printCompoundString prints a fused compound string token (e.g. "a" "b" "c").
// Each string part is printed on its own line in format mode.
func (p *printer) printCompoundString(tok token.Token, gap gapStyle, ctx printCtx) {
	openTok, closeTok := tok.StartEnd()
	trivia := p.trivia.scopeTrivia(tok.ID())

	// Collect interior string parts from the children cursor.
	var parts []token.Token
	cursor := tok.Children()
	for child := cursor.NextSkippable(); !child.IsZero(); child = cursor.NextSkippable() {
		if !child.Kind().IsSkippable() {
			parts = append(parts, child)
		}
	}

	if !p.options.Format {
		// Print the first string part using the fused token's outer trivia.
		p.printTokenAs(tok, gap, openTok.Text(), ctx)
		for i, part := range parts {
			p.emitTriviaSlot(trivia, i)
			p.printToken(part, gapNone, ctx)
		}
		p.emitRemainingTrivia(trivia, len(parts))
		p.printToken(closeTok, gapNone, ctx)
		return
	}

	// In format mode, all parts go on their own lines.
	// Clear lineToBlock: intermediate // comments between string parts
	// are on their own lines and are safe as-is.
	// The caller's ctx is used for the last part's trailing, since a
	// trailing // there would eat the following token (`;`, `]`, etc)
	// if the caller requested conversion.
	partsCtx := ctx
	partsCtx.lineToBlock = false

	printParts := func(pp *printer) {
		pp.printTokenAs(tok, gapNewline, openTok.Text(), partsCtx)
		for i, part := range parts {
			pp.emitTriviaSlot(trivia, i)
			pp.printToken(part, gapNewline, partsCtx)
		}
		pp.emitRemainingTrivia(trivia, len(parts))

		// Emit the last part's leading trivia with conversion off,
		// then use the caller's ctx for trailing only.
		att, hasTrivia := pp.trivia.tokenTrivia(closeTok.ID())
		if hasTrivia {
			pp.appendPending(att.leading)
			pp.emitTrivia(gapNewline)
		} else {
			pp.emitGap(gapNewline)
		}
		pp.push(dom.Text(closeTok.Text()))
		if hasTrivia {
			pp.emitTrailing(att.trailing, ctx)
		}
	}

	if ctx.indentExpr {
		// After `=` or `:`. Indent parts one level so they break
		// under the assignment.
		p.withIndent(func(indented *printer) {
			printParts(indented)
		})
	} else {
		// Already in an indented context (e.g., array elements).
		// Parts go at the current indent level.
		printParts(p)
	}
}

func (p *printer) printPrefixed(expr ast.ExprPrefixed, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}
	p.printToken(expr.PrefixToken(), gap, ctx)
	// In format mode, check if the value has leading comments. If so,
	// use gapSpace for proper spacing (e.g., "- /* comment */ 32").
	// Otherwise use gapNone to keep prefix glued (e.g., "-32").
	valueGap := gapNone
	if p.options.Format {
		inner := expr.Expr()
		var firstTok token.Token
		switch inner.Kind() {
		case ast.ExprKindLiteral:
			firstTok = inner.AsLiteral().Token
		case ast.ExprKindPath:
			firstTok = pathFirstToken(inner.AsPath().Path)
		}
		if !firstTok.IsZero() {
			if att, ok := p.trivia.tokenTrivia(firstTok.ID()); ok {
				if sliceHasComment(att.leading) {
					valueGap = gapSpace
				}
			}
		}
	}
	p.printExpr(expr.Expr(), valueGap, ctx)
}

func (p *printer) printExprRange(expr ast.ExprRange, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}
	start, end := expr.Bounds()
	p.printExpr(start, gap, ctx)
	p.printToken(expr.Keyword(), gapSpace, ctx)
	p.printExpr(end, gapSpace, ctx)
}

func (p *printer) printArray(expr ast.ExprArray, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}

	brackets := expr.Brackets()
	if brackets.IsZero() {
		return
	}

	// Containers manage their own indentation.
	ctx.lineToBlock = false
	ctx.indentExpr = false

	openTok, closeTok := brackets.StartEnd()
	slots := p.trivia.scopeTrivia(brackets.ID())
	elements := expr.Elements()

	if !p.options.Format {
		p.printToken(openTok, gap, ctx)
		for i := range elements.Len() {
			p.emitTriviaSlot(slots, i)
			elemGap := gapNone
			if i > 0 {
				p.printToken(elements.Comma(i-1), p.semiGap(), ctx)
				elemGap = gapSpace
			}
			p.printExpr(elements.At(i), elemGap, ctx)
		}
		p.emitTriviaSlot(slots, elements.Len())
		p.printToken(closeTok, gapNone, ctx)
		return
	}

	hasComments := triviaHasComments(slots)

	if elements.Len() == 0 && !hasComments {
		p.printToken(openTok, gap, ctx)
		p.printToken(closeTok, gapNone, ctx)
		return
	}

	if elements.Len() == 1 && !hasComments {
		// Emit the open bracket with the caller's gap outside the
		// group so that a gapNewline (e.g. from a dict field context)
		// doesn't force the group to break.
		p.printToken(openTok, gap, ctx)
		p.withGroup(func(p *printer) {
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				indented.emitTriviaSlot(slots, 0)
				indented.printExpr(elements.At(0), gapNone, ctx)
				indented.emitTriviaSlot(slots, 1)
			})
			p.push(tagSoftbreak)
		})
		p.printToken(closeTok, gapNone, ctx)
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	p.printToken(openTok, gap, ctx)
	p.withIndent(func(indented *printer) {
		for i := range elements.Len() {
			indented.emitTriviaSlot(slots, i)
			if i > 0 {
				indented.printToken(elements.Comma(i-1), p.semiGap(), ctx)
			}
			indented.printExpr(elements.At(i), gapNewline, ctx)
		}
		indented.emitTriviaSlot(slots, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, slots.blankBeforeClose)
		}
	})

	p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt, ctx)
}

func (p *printer) printDict(expr ast.ExprDict, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}
	// Containers manage their own indentation.
	ctx.lineToBlock = false
	ctx.indentExpr = false

	braces := expr.Braces()
	if braces.IsZero() {
		return
	}

	openTok, closeTok := braces.StartEnd()
	trivia := p.trivia.scopeTrivia(braces.ID())
	elements := expr.Elements()

	if !p.options.Format {
		p.printToken(openTok, gap, ctx)
		if elements.Len() > 0 || !trivia.isEmpty() {
			p.withIndent(func(indented *printer) {
				for i := range elements.Len() {
					indented.emitTriviaSlot(trivia, i)
					indented.printExprField(elements.At(i), gapNewline, ctx)
					indented.emitCommaTrivia(elements.Comma(i), ctx)
				}
				indented.emitTriviaSlot(trivia, elements.Len())
			})
		}
		p.printToken(closeTok, gapSoftline, ctx)
		return
	}

	openText, closeText := openTok.Text(), closeTok.Text()
	if braces.Keyword() == keyword.Angles {
		openText = "{"
		closeText = "}"
	}

	hasComments := triviaHasComments(trivia)
	// Also check for comments attached to any token in the scope
	// (trailing on open brace, leading on close brace, or on any
	// interior token). These force multi-line expansion.
	if !hasComments {
		hasComments = p.scopeHasAttachedComments(braces)
	}

	if elements.Len() == 0 && !hasComments {
		p.printTokenAs(openTok, gap, openText, ctx)
		p.printTokenAs(closeTok, gapNone, closeText, ctx)
		return
	}

	if elements.Len() == 1 && !hasComments {
		// Emit the open brace with the caller's gap outside the
		// group so that a gapNewline (e.g. from an array context)
		// doesn't force the group to break.
		p.printTokenAs(openTok, gap, openText, ctx)
		p.withGroup(func(p *printer) {
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				indented.emitTriviaSlot(trivia, 0)
				indented.printExprField(elements.At(0), gapNone, ctx)
				indented.emitCommaTrivia(elements.Comma(0), ctx)
				indented.emitTriviaSlot(trivia, 1)
			})
			p.push(tagSoftbreak)
		})
		p.printTokenAs(closeTok, gapNone, closeText, ctx)
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	// Check if the open brace has trailing comments that should be
	// moved inside the indented block.
	openTrailing := p.extractOpenTrailing(openTok)

	if len(openTrailing) > 0 {
		// Suppress trailing on open brace; emit inside indent block.
		// Cannot use printTokenAs here because we need to suppress
		// trailing and may need replacement text (angle -> brace).
		att, hasTrivia := p.trivia.tokenTrivia(openTok.ID())
		if hasTrivia {
			p.appendPending(att.leading)
			p.emitTrivia(gap)
		} else {
			p.emitGap(gap)
		}
		p.push(dom.Text(openText))
	} else {
		p.printTokenAs(openTok, gap, openText, ctx)
	}
	p.withIndent(func(indented *printer) {
		if len(openTrailing) > 0 {
			indented.appendPending(openTrailing)
			indented.emitTrivia(gapNewline)
		}
		for i := range elements.Len() {
			indented.emitTriviaSlot(trivia, i)
			indented.printExprField(elements.At(i), gapNewline, ctx)
			indented.emitCommaTrivia(elements.Comma(i), ctx)
		}
		indented.emitTriviaSlot(trivia, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	p.emitCloseTok(closeTok, closeText, closeComments, closeAtt, ctx)
}

func (p *printer) printExprField(expr ast.ExprField, gap gapStyle, ctx printCtx) {
	if expr.IsZero() {
		return
	}

	first := true
	if !expr.Key().IsZero() {
		p.printExpr(expr.Key(), gap, ctx)
		first = false
	}
	if !expr.Colon().IsZero() {
		// gapInline ensures comments between the key and colon get a
		// space before them but the colon follows immediately after the
		// last comment with no gap. This prevents an idempotency issue
		// where trivia between ] and : gets assigned differently on
		// reparse (leading vs trailing) depending on line breaks.
		p.printToken(expr.Colon(), gapInline, ctx)
	} else if p.options.Format && !expr.Key().IsZero() && !expr.Value().IsZero() {
		// Insert colon in format mode when missing (e.g. "e []" -> "e: []").
		p.push(dom.Text(":"))
	}
	if !expr.Value().IsZero() {
		valueGap := gapSpace
		if first {
			valueGap = gap
		}
		valueCtx := ctx
		valueCtx.indentExpr = true
		p.printExpr(expr.Value(), valueGap, valueCtx)
	}
}
