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

import "github.com/bufbuild/protocompile/experimental/ast"

// printExpr prints an expression with the specified leading gap.
func (p *printer) printExpr(expr ast.ExprAny, gap gapStyle) {
	if expr.IsZero() {
		return
	}

	switch expr.Kind() {
	case ast.ExprKindLiteral:
		p.printToken(expr.AsLiteral().Token, gap)
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
	slots := p.trivia.scopeSlots(brackets.ID())

	p.printToken(openTok, gap)
	elements := expr.Elements()
	for i := range elements.Len() {
		p.emitSlot(slots, i)
		elemGap := gapNone
		if i > 0 {
			p.printToken(elements.Comma(i-1), gapNone)
			elemGap = gapSpace
		}
		p.printExpr(elements.At(i), elemGap)
	}
	p.emitSlot(slots, elements.Len())
	p.printToken(closeTok, gapNone)
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
	slots := p.trivia.scopeSlots(braces.ID())

	p.printToken(openTok, gap)
	elements := expr.Elements()
	if elements.Len() > 0 || len(slots) > 0 {
		p.withIndent(func(indented *printer) {
			for i := range elements.Len() {
				indented.emitSlot(slots, i)
				indented.printExprField(elements.At(i), gapNewline)
			}
			indented.emitSlot(slots, elements.Len())
		})
	}

	p.printToken(closeTok, gapSoftline)
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
	}
	if !expr.Value().IsZero() {
		valueGap := gapSpace
		if first {
			valueGap = gap
		}
		p.printExpr(expr.Value(), valueGap)
	}
}
