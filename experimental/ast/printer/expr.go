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

	// Print the first string part using the fused token's outer trivia.
	p.printTokenAs(tok, gap, openTok.Text())

	// Collect interior string parts from the children cursor.
	var parts []token.Token
	cursor := tok.Children()
	for child := cursor.NextSkippable(); !child.IsZero(); child = cursor.NextSkippable() {
		if !child.Kind().IsSkippable() {
			parts = append(parts, child)
		}
	}

	if !p.options.Format {
		for i, part := range parts {
			p.emitTriviaSlot(trivia, i)
			p.printToken(part, gapNone)
		}
		p.emitRemainingTrivia(trivia, len(parts))
		p.printToken(closeTok, gapNone)
		return
	}

	// In format mode, indent continuation parts.
	p.withIndent(func(indented *printer) {
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
	p.printExpr(expr.Expr(), gapNone)
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

	openTok, closeTok := brackets.StartEnd()
	slots := p.trivia.scopeTrivia(brackets.ID())
	elements := expr.Elements()

	if !p.options.Format {
		p.printToken(openTok, gap)
		for i := range elements.Len() {
			p.emitTriviaSlot(slots, i)
			elemGap := gapNone
			if i > 0 {
				p.printToken(elements.Comma(i-1), gapNone)
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

	p.printToken(openTok, gap)
	p.withIndent(func(indented *printer) {
		for i := range elements.Len() {
			indented.emitTriviaSlot(slots, i)
			if i > 0 {
				indented.printToken(elements.Comma(i-1), gapNone)
			}
			indented.printExpr(elements.At(i), gapNewline)
		}
		indented.emitTriviaSlot(slots, elements.Len())
	})
	p.printToken(closeTok, gapNewline)
}

func (p *printer) printDict(expr ast.ExprDict, gap gapStyle) {
	if expr.IsZero() {
		return
	}

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

	p.printTokenAs(openTok, gap, openText)
	p.withIndent(func(indented *printer) {
		for i := range elements.Len() {
			indented.emitTriviaSlot(trivia, i)
			indented.printExprField(elements.At(i), gapNewline)
		}
		indented.emitTriviaSlot(trivia, elements.Len())
	})
	p.printTokenAs(closeTok, gapNewline, closeText)
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
