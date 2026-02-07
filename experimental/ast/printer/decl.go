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
)

// printDecl dispatches to the appropriate printer based on declaration kind.
func (p *printer) printDecl(decl ast.DeclAny) {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		p.printToken(decl.AsEmpty().Semicolon(), gapNone)
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

func (p *printer) printSyntax(decl ast.DeclSyntax) {
	p.printToken(decl.KeywordToken(), gapNewline)
	p.printToken(decl.Equals(), gapSpace)
	p.printExpr(decl.Value(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
}

func (p *printer) printPackage(decl ast.DeclPackage) {
	p.printToken(decl.KeywordToken(), gapNewline)
	p.printPath(decl.Path(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
}

func (p *printer) printImport(decl ast.DeclImport) {
	p.printToken(decl.KeywordToken(), gapNewline)
	modifiers := decl.ModifierTokens()
	for i := range modifiers.Len() {
		p.printToken(modifiers.At(i), gapSpace)
	}
	p.printExpr(decl.ImportPath(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
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
	p.printToken(opt.Keyword, gapNewline)
	p.printPath(opt.Path, gapSpace)
	if !opt.Equals.IsZero() {
		p.printToken(opt.Equals, gapSpace)
		p.printExpr(opt.Value, gapSpace)
	}
	p.printToken(opt.Semicolon, gapNone)
}

func (p *printer) printMessage(msg ast.DefMessage) {
	p.printToken(msg.Keyword, gapNewline)
	p.printToken(msg.Name, gapSpace)
	p.printBody(msg.Body)
}

func (p *printer) printEnum(e ast.DefEnum) {
	p.printToken(e.Keyword, gapNewline)
	p.printToken(e.Name, gapSpace)
	p.printBody(e.Body)
}

func (p *printer) printService(svc ast.DefService) {
	p.printToken(svc.Keyword, gapNewline)
	p.printToken(svc.Name, gapSpace)
	p.printBody(svc.Body)
}

func (p *printer) printExtend(ext ast.DefExtend) {
	p.printToken(ext.Keyword, gapNewline)
	p.printPath(ext.Extendee, gapSpace)
	p.printBody(ext.Body)
}

func (p *printer) printOneof(o ast.DefOneof) {
	p.printToken(o.Keyword, gapNewline)
	p.printToken(o.Name, gapSpace)
	p.printBody(o.Body)
}

func (p *printer) printGroup(g ast.DefGroup) {
	p.printToken(g.Keyword, gapNewline)
	p.printToken(g.Name, gapSpace)
	if !g.Equals.IsZero() {
		p.printToken(g.Equals, gapSpace)
		p.printExpr(g.Tag, gapSpace)
	}
	p.printCompactOptions(g.Options)
	p.printBody(g.Body)
}

func (p *printer) printField(f ast.DefField) {
	p.printType(f.Type, gapNewline)
	p.printToken(f.Name, gapSpace)
	if !f.Equals.IsZero() {
		p.printToken(f.Equals, gapSpace)
		p.printExpr(f.Tag, gapSpace)
	}
	p.printCompactOptions(f.Options)
	p.printToken(f.Semicolon, gapNone)
}

func (p *printer) printEnumValue(ev ast.DefEnumValue) {
	p.printToken(ev.Name, gapNewline)
	if !ev.Equals.IsZero() {
		p.printToken(ev.Equals, gapSpace)
		p.printExpr(ev.Tag, gapSpace)
	}
	p.printCompactOptions(ev.Options)
	p.printToken(ev.Semicolon, gapNone)
}

func (p *printer) printMethod(m ast.DefMethod) {
	p.printToken(m.Keyword, gapNewline)
	p.printToken(m.Name, gapSpace)
	p.printSignature(m.Signature)
	if !m.Body.IsZero() {
		p.printBody(m.Body)
	} else {
		p.printToken(m.Decl.Semicolon(), gapNone)
	}
}

func (p *printer) printSignature(sig ast.Signature) {
	if sig.IsZero() {
		return
	}

	inputs := sig.Inputs()
	if !inputs.Brackets().IsZero() {
		p.withGroup(func(p *printer) {
			openTok, closeTok := inputs.Brackets().StartEnd()
			slots := p.trivia.scopeSlots(inputs.Brackets().ID())
			p.printToken(openTok, gapNone)
			p.withIndent(func(indented *printer) {
				indented.push(dom.TextIf(dom.Broken, "\n"))
				indented.printTypeListContents(inputs, slots)
				p.push(dom.TextIf(dom.Broken, "\n"))
			})
			p.printToken(closeTok, gapNone)
		})
	}

	if !sig.Returns().IsZero() {
		p.printToken(sig.Returns(), gapSpace)
		outputs := sig.Outputs()
		if !outputs.Brackets().IsZero() {
			p.withGroup(func(p *printer) {
				openTok, closeTok := outputs.Brackets().StartEnd()
				slots := p.trivia.scopeSlots(outputs.Brackets().ID())
				p.printToken(openTok, gapSpace)
				p.withIndent(func(indented *printer) {
					indented.push(dom.TextIf(dom.Broken, "\n"))
					indented.printTypeListContents(outputs, slots)
					p.push(dom.TextIf(dom.Broken, "\n"))
				})
				p.printToken(closeTok, gapNone)
			})
		}
	}
}

func (p *printer) printTypeListContents(list ast.TypeList, slots []slot) {
	gap := gapNone
	for i := range list.Len() {
		p.emitSlot(slots, i)
		if i > 0 {
			p.printToken(list.Comma(i-1), gapNone)
			gap = gapSoftline
		}
		p.printType(list.At(i), gap)
	}
	p.emitSlot(slots, list.Len())
}

func (p *printer) printBody(body ast.DeclBody) {
	if body.IsZero() {
		return
	}

	braces := body.Braces()
	if braces.IsZero() {
		return
	}

	openTok, closeTok := braces.StartEnd()
	slots := p.trivia.scopeSlots(braces.ID())

	p.printToken(openTok, gapSpace)

	closeGap := gapNone
	if body.Decls().Len() > 0 || len(slots) > 0 {
		closeGap = gapNewline
		p.withIndent(func(indented *printer) {
			indented.printScopeDecls(slots, body.Decls())
		})
	}
	p.printToken(closeTok, closeGap)
}

func (p *printer) printRange(r ast.DeclRange) {
	if !r.KeywordToken().IsZero() {
		p.printToken(r.KeywordToken(), gapNone)
	}

	ranges := r.Ranges()
	for i := range ranges.Len() {
		if i > 0 {
			p.printToken(ranges.Comma(i-1), gapNone)
		}
		p.printExpr(ranges.At(i), gapSpace)
	}
	p.printCompactOptions(r.Options())
	p.printToken(r.Semicolon(), gapNone)
}

func (p *printer) printCompactOptions(co ast.CompactOptions) {
	if co.IsZero() {
		return
	}

	brackets := co.Brackets()
	if brackets.IsZero() {
		return
	}

	openTok, closeTok := brackets.StartEnd()
	slots := p.trivia.scopeSlots(brackets.ID())

	p.withGroup(func(p *printer) {
		p.printToken(openTok, gapSpace)
		entries := co.Entries()
		p.withIndent(func(indented *printer) {
			for i := range entries.Len() {
				indented.emitSlot(slots, i)
				if i > 0 {
					indented.printToken(entries.Comma(i-1), gapNone)
					indented.printPath(entries.At(i).Path, gapSoftline)
				} else {
					indented.printPath(entries.At(i).Path, gapNone)
				}

				opt := entries.At(i)
				if !opt.Equals.IsZero() {
					indented.printToken(opt.Equals, gapSpace)
					indented.printExpr(opt.Value, gapSpace)
				}
			}
			p.emitSlot(slots, entries.Len())
		})
		p.printToken(closeTok, gapNone)
	})
}
