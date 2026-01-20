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
)

// printDecl dispatches to the appropriate printer based on declaration kind.
func (p *printer) printDecl(decl ast.DeclAny) {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		p.printEmpty(decl.AsEmpty())
	case ast.DeclKindSyntax:
		p.printSyntax(decl.AsSyntax())
	case ast.DeclKindPackage:
		p.printPackage(decl.AsPackage())
	case ast.DeclKindImport:
		p.printImport(decl.AsImport())
	case ast.DeclKindDef:
		p.printDef(decl.AsDef())
	case ast.DeclKindBody:
		p.printBody(decl.AsBody())
	case ast.DeclKindRange:
		p.printRange(decl.AsRange())
	}
}

func (p *printer) printEmpty(decl ast.DeclEmpty) {
	p.printToken(decl.Semicolon())
}

func (p *printer) printSyntax(decl ast.DeclSyntax) {
	p.printToken(decl.KeywordToken())
	p.printToken(decl.Equals())
	p.printExpr(decl.Value())
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon())
}

func (p *printer) printPackage(decl ast.DeclPackage) {
	p.printToken(decl.KeywordToken())
	p.printPath(decl.Path())
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon())
}

func (p *printer) printImport(decl ast.DeclImport) {
	p.printToken(decl.KeywordToken())
	modifiers := decl.ModifierTokens()
	for i := range modifiers.Len() {
		p.printToken(modifiers.At(i))
	}
	p.printExpr(decl.ImportPath())
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon())
}

func (p *printer) printDef(decl ast.DeclDef) {
	switch decl.Classify() {
	case ast.DefKindOption:
		p.printOption(decl.AsOption())
	case ast.DefKindMessage:
		p.printMessage(decl.AsMessage())
	case ast.DefKindEnum:
		p.printEnum(decl.AsEnum())
	case ast.DefKindService:
		p.printService(decl.AsService())
	case ast.DefKindField:
		p.printField(decl.AsField())
	case ast.DefKindEnumValue:
		p.printEnumValue(decl.AsEnumValue())
	case ast.DefKindOneof:
		p.printOneof(decl.AsOneof())
	case ast.DefKindMethod:
		p.printMethod(decl.AsMethod())
	case ast.DefKindExtend:
		p.printExtend(decl.AsExtend())
	case ast.DefKindGroup:
		p.printGroup(decl.AsGroup())
	}
}

func (p *printer) printOption(opt ast.DefOption) {
	p.printToken(opt.Keyword)
	p.printPath(opt.Path)
	if !opt.Equals.IsZero() {
		p.printToken(opt.Equals)
		p.printExpr(opt.Value)
	}
	p.printToken(opt.Semicolon)
}

func (p *printer) printMessage(msg ast.DefMessage) {
	p.printToken(msg.Keyword)
	p.printToken(msg.Name)
	p.printBody(msg.Body)
}

func (p *printer) printEnum(e ast.DefEnum) {
	p.printToken(e.Keyword)
	p.printToken(e.Name)
	p.printBody(e.Body)
}

func (p *printer) printService(svc ast.DefService) {
	p.printToken(svc.Keyword)
	p.printToken(svc.Name)
	p.printBody(svc.Body)
}

func (p *printer) printExtend(ext ast.DefExtend) {
	p.printToken(ext.Keyword)
	p.printPath(ext.Extendee)
	p.printBody(ext.Body)
}

func (p *printer) printOneof(o ast.DefOneof) {
	p.printToken(o.Keyword)
	p.printToken(o.Name)
	p.printBody(o.Body)
}

func (p *printer) printGroup(g ast.DefGroup) {
	p.printToken(g.Keyword)
	p.printToken(g.Name)
	if !g.Equals.IsZero() {
		p.printToken(g.Equals)
		p.printExpr(g.Tag)
	}
	p.printCompactOptions(g.Options)
	p.printBody(g.Body)
}

func (p *printer) printField(f ast.DefField) {
	p.printType(f.Type)
	p.printToken(f.Name)
	if !f.Equals.IsZero() {
		p.printToken(f.Equals)
		p.printExpr(f.Tag)
	}
	p.printCompactOptions(f.Options)
	p.printToken(f.Semicolon)
}

func (p *printer) printEnumValue(ev ast.DefEnumValue) {
	p.printToken(ev.Name)
	if !ev.Equals.IsZero() {
		p.printToken(ev.Equals)
		p.printExpr(ev.Tag)
	}
	p.printCompactOptions(ev.Options)
	p.printToken(ev.Semicolon)
}

func (p *printer) printMethod(m ast.DefMethod) {
	p.printToken(m.Keyword)
	p.printToken(m.Name)
	p.printSignature(m.Signature)
	if !m.Body.IsZero() {
		p.printBody(m.Body)
	} else {
		p.printToken(m.Decl.Semicolon())
	}
}

func (p *printer) printSignature(sig ast.Signature) {
	if sig.IsZero() {
		return
	}

	// Print input parameter list with its brackets
	// Note: brackets are fused tokens, so we handle them specially to preserve whitespace
	inputs := sig.Inputs()
	inputBrackets := inputs.Brackets()
	if !inputBrackets.IsZero() {
		p.printFusedBrackets(inputBrackets, func(child *printer) {
			child.printTypeListContents(inputs)
		})
	}

	// Print returns clause if present
	if !sig.Returns().IsZero() {
		p.printToken(sig.Returns())
		outputs := sig.Outputs()
		outputBrackets := outputs.Brackets()
		if !outputBrackets.IsZero() {
			p.printFusedBrackets(outputBrackets, func(child *printer) {
				child.printTypeListContents(outputs)
			})
		}
	}
}

func (p *printer) printTypeListContents(list ast.TypeList) {
	for i := range list.Len() {
		if i > 0 {
			p.printToken(list.Comma(i - 1))
		}
		p.printType(list.At(i))
	}
}

func (p *printer) printBody(body ast.DeclBody) {
	if body.IsZero() {
		return
	}

	braces := body.Braces()
	openTok, closeTok := braces.StartEnd()

	p.emitOpen(openTok)

	decls := body.Decls()
	if decls.Len() > 0 {
		p.push(dom.Indent(p.opts.Indent, func(push dom.Sink) {
			child := p.childWithCursor(push, braces, openTok)
			for d := range seq.Values(decls) {
				child.printDecl(d)
			}
			child.flushRemaining()
		}))
	}

	p.emitClose(closeTok, openTok)
}

func (p *printer) printRange(r ast.DeclRange) {
	if !r.KeywordToken().IsZero() {
		p.printToken(r.KeywordToken())
	}

	ranges := r.Ranges()
	for i := range ranges.Len() {
		if i > 0 {
			p.printToken(ranges.Comma(i - 1))
		}
		p.printExpr(ranges.At(i))
	}
	p.printCompactOptions(r.Options())
	p.printToken(r.Semicolon())
}

func (p *printer) printCompactOptions(co ast.CompactOptions) {
	if co.IsZero() {
		return
	}
	p.printFusedBrackets(co.Brackets(), func(child *printer) {
		entries := co.Entries()
		for i := range entries.Len() {
			if i > 0 {
				child.printToken(entries.Comma(i - 1))
			}
			opt := entries.At(i)
			child.printPath(opt.Path)
			if !opt.Equals.IsZero() {
				child.printToken(opt.Equals)
				child.printExpr(opt.Value)
			}
		}
	})
}
