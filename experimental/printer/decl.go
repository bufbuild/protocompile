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
	"github.com/bufbuild/protocompile/experimental/seq"
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
		p.printFusedBrackets(inputs.Brackets(), gapNone, func(child *printer) {
			child.printTypeListContents(inputs)
		})
	}

	if !sig.Returns().IsZero() {
		p.printToken(sig.Returns(), gapSpace)
		outputs := sig.Outputs()
		if !outputs.Brackets().IsZero() {
			p.printFusedBrackets(outputs.Brackets(), gapSpace, func(child *printer) {
				child.printTypeListContents(outputs)
			})
		}
	}
}

func (p *printer) printTypeListContents(list ast.TypeList) {
	for i := range list.Len() {
		gap := gapNone
		if i > 0 {
			p.printToken(list.Comma(i-1), gapNone)
			gap = gapSpace
		}
		p.printType(list.At(i), gap)
	}
}

func (p *printer) printBody(body ast.DeclBody) {
	if body.IsZero() {
		return
	}

	p.printFusedBrackets(body.Braces(), gapSpace, func(child *printer) {
		if body.Decls().Len() > 0 {
			child.withIndent(func(indented *printer) {
				for d := range seq.Values(body.Decls()) {
					indented.printDecl(d)
				}
				indented.flushRemaining()
			})
		}
	})
}

func (p *printer) printRange(r ast.DeclRange) {
	if !r.KeywordToken().IsZero() {
		p.printToken(r.KeywordToken(), gapNone)
	}

	ranges := r.Ranges()
	for i := range ranges.Len() {
		gap := gapSpace
		if i > 0 {
			p.printToken(ranges.Comma(i-1), gapNone)
		}
		p.printExpr(ranges.At(i), gap)
	}
	p.printCompactOptions(r.Options())
	p.printToken(r.Semicolon(), gapNone)
}

func (p *printer) printCompactOptions(co ast.CompactOptions) {
	if co.IsZero() {
		return
	}
	p.printFusedBrackets(co.Brackets(), gapSpace, func(child *printer) {
		entries := co.Entries()
		for i := range entries.Len() {
			if i > 0 {
				child.printToken(entries.Comma(i-1), gapNone)
			}
			opt := entries.At(i)
			gap := gapNone
			if i > 0 {
				gap = gapSpace
			}
			child.printPath(opt.Path, gap)
			if !opt.Equals.IsZero() {
				child.printToken(opt.Equals, gapSpace)
				child.printExpr(opt.Value, gapSpace)
			}
		}
	})
}
