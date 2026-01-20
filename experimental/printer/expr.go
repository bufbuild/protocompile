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
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

// printExpr prints an expression.
func (p *printer) printExpr(expr ast.ExprAny) {
	if expr.IsZero() {
		return
	}

	switch expr.Kind() {
	case ast.ExprKindLiteral:
		p.printLiteral(expr.AsLiteral())
	case ast.ExprKindPath:
		p.printPath(expr.AsPath().Path)
	case ast.ExprKindPrefixed:
		p.printPrefixed(expr.AsPrefixed())
	case ast.ExprKindRange:
		p.printExprRange(expr.AsRange())
	case ast.ExprKindArray:
		p.printArray(expr.AsArray())
	case ast.ExprKindDict:
		p.printDict(expr.AsDict())
	case ast.ExprKindField:
		p.printExprField(expr.AsField())
	}
}

func (p *printer) printLiteral(lit ast.ExprLiteral) {
	if lit.IsZero() {
		return
	}
	p.printToken(lit.Token)
}

func (p *printer) printPrefixed(expr ast.ExprPrefixed) {
	if expr.IsZero() {
		return
	}
	p.printToken(expr.PrefixToken())
	p.printExpr(expr.Expr())
}

func (p *printer) printExprRange(expr ast.ExprRange) {
	if expr.IsZero() {
		return
	}
	start, end := expr.Bounds()
	p.printExpr(start)
	p.printToken(expr.Keyword())
	p.printExpr(end)
}

func (p *printer) printArray(expr ast.ExprArray) {
	if expr.IsZero() {
		return
	}

	brackets := expr.Brackets()
	if !brackets.IsZero() {
		p.printFusedBrackets(brackets, func(child *printer) {
			elements := expr.Elements()
			for i := range elements.Len() {
				if i > 0 {
					child.printToken(elements.Comma(i - 1))
				}
				child.printExpr(elements.At(i))
			}
		})
	} else {
		// Synthetic array - emit brackets manually
		p.text(keyword.LBracket.String())
		elements := expr.Elements()
		for i := range elements.Len() {
			if i > 0 {
				p.text(keyword.Comma.String())
				p.text(" ")
			}
			p.printExpr(elements.At(i))
		}
		p.text(keyword.RBracket.String())
	}
}

func (p *printer) printDict(expr ast.ExprDict) {
	if expr.IsZero() {
		return
	}

	p.text(keyword.LBrace.String())
	elements := expr.Elements()
	if elements.Len() > 0 {
		p.push(dom.Indent(p.opts.Indent, func(push dom.Sink) {
			child := newPrinter(push, p.opts)
			for i := range elements.Len() {
				child.newline()
				child.printExprField(elements.At(i))
			}
		}))
		p.newline()
	}
	p.text(keyword.RBrace.String())
}

func (p *printer) printExprField(expr ast.ExprField) {
	if expr.IsZero() {
		return
	}

	if !expr.Key().IsZero() {
		p.printExpr(expr.Key())
	}
	if !expr.Colon().IsZero() {
		p.printToken(expr.Colon())
	}
	if !expr.Value().IsZero() {
		p.printExpr(expr.Value())
	}
}
