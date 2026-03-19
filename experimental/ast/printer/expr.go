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
func (p *printer) printExpr(expr ast.ExprAny, gap gapStyle) {
	if expr.IsZero() {
		return
	}

	switch expr.Kind() {
	case ast.ExprKindLiteral:
		tok := expr.AsLiteral().Token
		if !tok.IsLeaf() {
			p.printCompoundString(tok, gap)
		} else {
			p.printToken(tok, gap)
		}
	case ast.ExprKindPath:
		p.printPath(expr.AsPath().Path, gap)
	case ast.ExprKindPrefixed:
		p.printPrefixed(expr.AsPrefixed(), gap)
	case ast.ExprKindRange:
		p.printExprRange(expr.AsRange(), gap)
	case ast.ExprKindArray:
		p.printArray(expr.AsArray(), gap)
	case ast.ExprKindDict:
		p.printDict(expr.AsDict(), gap)
	case ast.ExprKindField:
		p.printExprField(expr.AsField(), gap)
	}
}

// printCompoundString prints a fused compound string token (e.g. "a" "b" "c").
// Each string part is printed on its own line in format mode.
func (p *printer) printCompoundString(tok token.Token, gap gapStyle) {
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
		p.printTokenAs(tok, gap, openTok.Text())
		for i, part := range parts {
			p.emitTriviaSlot(trivia, i)
			p.printToken(part, gapNone)
		}
		p.emitRemainingTrivia(trivia, len(parts))
		p.printToken(closeTok, gapNone)
		return
	}

	// In format mode, all parts go on their own indented lines.
	// The first element uses the fused token's trivia, with a
	// newline gap to start on a new line after the `=`.
	// In format mode, all parts go on their own indented lines.
	p.withIndent(func(indented *printer) {
		indented.printTokenAs(tok, gapNewline, openTok.Text())
		for i, part := range parts {
			indented.emitTriviaSlot(trivia, i)
			indented.printToken(part, gapNewline)
		}
		indented.emitRemainingTrivia(trivia, len(parts))
		indented.printToken(closeTok, gapNewline)
	})
}

func (p *printer) printPrefixed(expr ast.ExprPrefixed, gap gapStyle) {
	if expr.IsZero() {
		return
	}
	p.printToken(expr.PrefixToken(), gap)
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
			for pc := range inner.AsPath().Path.Components {
				if !pc.Separator().IsZero() {
					firstTok = pc.Separator()
				} else if !pc.Name().IsZero() {
					firstTok = pc.Name()
				}
				break
			}
		}
		if !firstTok.IsZero() {
			if att, ok := p.trivia.tokenTrivia(firstTok.ID()); ok {
				for _, t := range att.leading {
					if t.Kind() == token.Comment {
						valueGap = gapSpace
						break
					}
				}
			}
		}
	}
	p.printExpr(expr.Expr(), valueGap)
}

func (p *printer) printExprRange(expr ast.ExprRange, gap gapStyle) {
	if expr.IsZero() {
		return
	}
	start, end := expr.Bounds()
	p.printExpr(start, gap)
	p.printToken(expr.Keyword(), gapSpace)
	p.printExpr(end, gapSpace)
}

func (p *printer) printArray(expr ast.ExprArray, gap gapStyle) {
	if expr.IsZero() {
		return
	}

	brackets := expr.Brackets()
	if brackets.IsZero() {
		return
	}

	// Entering a nested scope: clear convertLineToBlock since //
	// comments on their own lines inside arrays are fine.
	saved := p.convertLineToBlock
	p.convertLineToBlock = false
	defer func() { p.convertLineToBlock = saved }()

	openTok, closeTok := brackets.StartEnd()
	slots := p.trivia.scopeTrivia(brackets.ID())
	elements := expr.Elements()

	if !p.options.Format {
		p.printToken(openTok, gap)
		for i := range elements.Len() {
			p.emitTriviaSlot(slots, i)
			elemGap := gapNone
			if i > 0 {
				p.printToken(elements.Comma(i-1), p.semiGap())
				elemGap = gapSpace
			}
			p.printExpr(elements.At(i), elemGap)
		}
		p.emitTriviaSlot(slots, elements.Len())
		p.printToken(closeTok, gapNone)
		return
	}

	hasComments := triviaHasComments(slots)

	if elements.Len() == 0 && !hasComments {
		p.printToken(openTok, gap)
		p.printToken(closeTok, gapNone)
		return
	}

	if elements.Len() == 1 && !hasComments {
		p.withGroup(func(p *printer) {
			p.printToken(openTok, gap)
			p.withIndent(func(indented *printer) {
				indented.push(dom.TextIf(dom.Broken, "\n"))
				indented.emitTriviaSlot(slots, 0)
				indented.printExpr(elements.At(0), gapNone)
				indented.emitTriviaSlot(slots, 1)
			})
			p.push(dom.TextIf(dom.Broken, "\n"))
			p.printToken(closeTok, gapNone)
		})
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	p.printToken(openTok, gap)
	p.withIndent(func(indented *printer) {
		for i := range elements.Len() {
			indented.emitTriviaSlot(slots, i)
			if i > 0 {
				indented.printToken(elements.Comma(i-1), p.semiGap())
			}
			indented.printExpr(elements.At(i), gapNewline)
		}
		indented.emitTriviaSlot(slots, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, slots.blankBeforeClose)
		}
	})

	if len(closeComments) > 0 {
		p.emitGap(gapNewline)
		p.push(dom.Text(closeTok.Text()))
		p.emitTrailing(closeAtt.trailing)
	} else {
		p.printToken(closeTok, gapNewline)
	}
}

func (p *printer) printDict(expr ast.ExprDict, gap gapStyle) {
	if expr.IsZero() {
		return
	}
	// Entering a nested scope: clear convertLineToBlock since //
	// comments on their own lines inside dicts are fine.
	saved := p.convertLineToBlock
	p.convertLineToBlock = false
	defer func() { p.convertLineToBlock = saved }()

	braces := expr.Braces()
	if braces.IsZero() {
		return
	}

	openTok, closeTok := braces.StartEnd()
	trivia := p.trivia.scopeTrivia(braces.ID())
	elements := expr.Elements()

	if !p.options.Format {
		p.printToken(openTok, gap)
		if elements.Len() > 0 || !trivia.isEmpty() {
			p.withIndent(func(indented *printer) {
				for i := range elements.Len() {
					indented.emitTriviaSlot(trivia, i)
					indented.printExprField(elements.At(i), gapNewline)
				}
				indented.emitTriviaSlot(trivia, elements.Len())
			})
		}
		p.printToken(closeTok, gapSoftline)
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
		p.printTokenAs(openTok, gap, openText)
		p.printTokenAs(closeTok, gapNone, closeText)
		return
	}

	if elements.Len() == 1 && !hasComments {
		p.withGroup(func(p *printer) {
			p.printTokenAs(openTok, gap, openText)
			p.withIndent(func(indented *printer) {
				indented.push(dom.TextIf(dom.Broken, "\n"))
				indented.emitTriviaSlot(trivia, 0)
				indented.printExprField(elements.At(0), gapNone)
				indented.emitTriviaSlot(trivia, 1)
			})
			p.push(dom.TextIf(dom.Broken, "\n"))
			p.printTokenAs(closeTok, gapNone, closeText)
		})
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	// Check if the open brace has trailing comments that should be
	// moved inside the indented block.
	var openTrailing []token.Token
	if att, ok := p.trivia.tokenTrivia(openTok.ID()); ok {
		for _, t := range att.trailing {
			if t.Kind() == token.Comment {
				openTrailing = att.trailing
				break
			}
		}
	}

	if len(openTrailing) > 0 {
		// Suppress trailing on open brace; emit inside indent block.
		att, hasTrivia := p.trivia.tokenTrivia(openTok.ID())
		if hasTrivia {
			p.appendPending(att.leading)
			p.emitTrivia(gap)
		} else {
			p.emitGap(gap)
		}
		p.push(dom.Text(openText))
	} else {
		p.printTokenAs(openTok, gap, openText)
	}
	p.withIndent(func(indented *printer) {
		if len(openTrailing) > 0 {
			indented.appendPending(openTrailing)
			indented.emitTrivia(gapNewline)
		}
		for i := range elements.Len() {
			indented.emitTriviaSlot(trivia, i)
			indented.printExprField(elements.At(i), gapNewline)
		}
		indented.emitTriviaSlot(trivia, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	if len(closeComments) > 0 {
		p.emitGap(gapNewline)
		p.push(dom.Text(closeText))
		p.emitTrailing(closeAtt.trailing)
	} else {
		p.printTokenAs(closeTok, gapNewline, closeText)
	}
}

func (p *printer) printExprField(expr ast.ExprField, gap gapStyle) {
	if expr.IsZero() {
		return
	}

	first := true
	if !expr.Key().IsZero() {
		p.printExpr(expr.Key(), gap)
		first = false
	}
	if !expr.Colon().IsZero() {
		p.printToken(expr.Colon(), gapNone)
	} else if p.options.Format && !expr.Key().IsZero() && !expr.Value().IsZero() {
		// Insert colon in format mode when missing (e.g. "e []" -> "e: []").
		p.push(dom.Text(":"))
	}
	if !expr.Value().IsZero() {
		valueGap := gapSpace
		if first {
			valueGap = gap
		}
		p.printExpr(expr.Value(), valueGap)
	}
}
