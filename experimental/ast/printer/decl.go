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
)

// printDecl dispatches to the appropriate printer based on declaration kind.
//
// gap controls the whitespace before the declaration's first token. The caller
// determines this based on the declaration's position within its scope (e.g.
// gapNone for the first declaration in a file, gapBlankline between sections).
func (p *printer) printDecl(decl ast.DeclAny, gap gapStyle) {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		if p.options.Format {
			return
		}
		p.printToken(decl.AsEmpty().Semicolon(), gap)
	case ast.DeclKindSyntax:
		p.printSyntax(decl.AsSyntax(), gap)
	case ast.DeclKindPackage:
		p.printPackage(decl.AsPackage(), gap)
	case ast.DeclKindImport:
		p.printImport(decl.AsImport(), gap)
	case ast.DeclKindDef:
		p.printDef(decl.AsDef(), gap)
	case ast.DeclKindBody:
		p.printBody(decl.AsBody())
	case ast.DeclKindRange:
		p.printRange(decl.AsRange(), gap)
	}
}

func (p *printer) printSyntax(decl ast.DeclSyntax, gap gapStyle) {
	p.printToken(decl.KeywordToken(), gap)
	p.printToken(decl.Equals(), gapSpace)
	p.printExpr(decl.Value(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
}

func (p *printer) printPackage(decl ast.DeclPackage, gap gapStyle) {
	p.printToken(decl.KeywordToken(), gap)
	p.printPath(decl.Path(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
}

func (p *printer) printImport(decl ast.DeclImport, gap gapStyle) {
	p.printToken(decl.KeywordToken(), gap)
	modifiers := decl.ModifierTokens()
	for i := range modifiers.Len() {
		p.printToken(modifiers.At(i), gapSpace)
	}
	p.printExpr(decl.ImportPath(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), gapNone)
}

func (p *printer) printDef(decl ast.DeclDef, gap gapStyle) {
	switch decl.Classify() {
	case ast.DefKindOption:
		p.printOption(decl.AsOption(), gap)
	case ast.DefKindMessage:
		p.printMessage(decl.AsMessage(), gap)
	case ast.DefKindEnum:
		p.printEnum(decl.AsEnum(), gap)
	case ast.DefKindService:
		p.printService(decl.AsService(), gap)
	case ast.DefKindField:
		p.printField(decl.AsField(), gap)
	case ast.DefKindEnumValue:
		p.printEnumValue(decl.AsEnumValue(), gap)
	case ast.DefKindOneof:
		p.printOneof(decl.AsOneof(), gap)
	case ast.DefKindMethod:
		p.printMethod(decl.AsMethod(), gap)
	case ast.DefKindExtend:
		p.printExtend(decl.AsExtend(), gap)
	case ast.DefKindGroup:
		p.printGroup(decl.AsGroup(), gap)
	}
}

func (p *printer) printOption(opt ast.DefOption, gap gapStyle) {
	p.printToken(opt.Keyword, gap)
	p.printPath(opt.Path, gapSpace)
	if !opt.Equals.IsZero() {
		p.printToken(opt.Equals, gapSpace)
		p.printExpr(opt.Value, gapSpace)
	}
	p.printToken(opt.Semicolon, gapNone)
}

func (p *printer) printMessage(msg ast.DefMessage, gap gapStyle) {
	p.printToken(msg.Keyword, gap)
	p.printToken(msg.Name, gapSpace)
	p.printBody(msg.Body)
}

func (p *printer) printEnum(e ast.DefEnum, gap gapStyle) {
	p.printToken(e.Keyword, gap)
	p.printToken(e.Name, gapSpace)
	p.printBody(e.Body)
}

func (p *printer) printService(svc ast.DefService, gap gapStyle) {
	p.printToken(svc.Keyword, gap)
	p.printToken(svc.Name, gapSpace)
	p.printBody(svc.Body)
}

func (p *printer) printExtend(ext ast.DefExtend, gap gapStyle) {
	p.printToken(ext.Keyword, gap)
	p.printPath(ext.Extendee, gapSpace)
	p.printBody(ext.Body)
}

func (p *printer) printOneof(o ast.DefOneof, gap gapStyle) {
	p.printToken(o.Keyword, gap)
	p.printToken(o.Name, gapSpace)
	p.printBody(o.Body)
}

func (p *printer) printGroup(g ast.DefGroup, gap gapStyle) {
	// Print type prefixes (optional/required/repeated) from the underlying
	// DeclDef, since DefGroup.Keyword is the "group" keyword itself.
	for prefix := range g.Decl.Prefixes() {
		p.printToken(prefix.PrefixToken(), gap)
		gap = gapSpace
	}

	p.printToken(g.Keyword, gap)
	p.printToken(g.Name, gapSpace)
	if !g.Equals.IsZero() {
		p.printToken(g.Equals, gapSpace)
		p.printExpr(g.Tag, gapSpace)
	}
	p.printCompactOptions(g.Options)

	// Use Decl.Body() because DefGroup.Body is not populated by AsGroup().
	p.printBody(g.Decl.Body())
}

func (p *printer) printField(f ast.DefField, gap gapStyle) {
	p.printType(f.Type, gap)
	p.printToken(f.Name, gapSpace)
	if !f.Equals.IsZero() {
		p.printToken(f.Equals, gapSpace)
		p.printExpr(f.Tag, gapSpace)
	}
	p.printCompactOptions(f.Options)
	p.printToken(f.Semicolon, gapNone)
}

func (p *printer) printEnumValue(ev ast.DefEnumValue, gap gapStyle) {
	p.printToken(ev.Name, gap)
	if !ev.Equals.IsZero() {
		p.printToken(ev.Equals, gapSpace)
		p.printExpr(ev.Tag, gapSpace)
	}
	p.printCompactOptions(ev.Options)
	p.printToken(ev.Semicolon, gapNone)
}

func (p *printer) printMethod(m ast.DefMethod, gap gapStyle) {
	p.printToken(m.Keyword, gap)
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
			trivia := p.trivia.scopeTrivia(inputs.Brackets().ID())
			p.printToken(openTok, gapNone)
			p.withIndent(func(indented *printer) {
				indented.push(dom.TextIf(dom.Broken, "\n"))
				indented.printTypeListContents(inputs, trivia)
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
				slots := p.trivia.scopeTrivia(outputs.Brackets().ID())
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

func (p *printer) printTypeListContents(list ast.TypeList, trivia detachedTrivia) {
	gap := gapNone
	for i := range list.Len() {
		p.emitTriviaSlot(trivia, i)
		if i > 0 {
			p.printToken(list.Comma(i-1), gapNone)
			gap = gapSoftline
		}
		p.printType(list.At(i), gap)
	}
	p.emitRemainingTrivia(trivia, list.Len())
}

func (p *printer) printBody(body ast.DeclBody) {
	if body.IsZero() || body.Braces().IsZero() {
		return
	}

	openTok, closeTok := body.Braces().StartEnd()
	trivia := p.trivia.scopeTrivia(body.Braces().ID())

	p.printToken(openTok, gapSpace)

	var closeComments []token.Token
	if p.options.Format {
		att, hasTrivia := p.trivia.tokenTrivia(closeTok.ID())
		if hasTrivia {
			for _, t := range att.leading {
				if t.Kind() == token.Comment {
					closeComments = att.leading
					break
				}
			}
		}
	}

	hasContent := body.Decls().Len() > 0 || !trivia.isEmpty() || len(closeComments) > 0
	if !hasContent {
		p.printToken(closeTok, gapNone)
		return
	}

	p.withIndent(func(indented *printer) {
		indented.printScopeDecls(trivia, body.Decls(), gapNewline)
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	if len(closeComments) > 0 {
		p.emitGap(gapNewline)
		p.push(dom.Text(closeTok.Text()))
		att, _ := p.trivia.tokenTrivia(closeTok.ID())
		p.emitTrailing(att.trailing)
	} else {
		p.printToken(closeTok, gapNewline)
	}
}

// emitCloseComments emits close-brace leading comments inside an
// indented context, flushing any pending scope trivia first.
func (p *printer) emitCloseComments(comments []token.Token, blankBeforeClose bool) {
	gap := gapNewline
	if blankBeforeClose {
		gap = gapBlankline
	}
	for _, t := range p.pending {
		if t.Kind() != token.Comment {
			continue
		}
		p.emitGap(gap)
		p.push(dom.Text(t.Text()))
		gap = gapNewline
	}
	p.pending = p.pending[:0]

	newlineRun := 0
	for _, t := range comments {
		if t.Kind() == token.Space {
			if t.Text() == "\n" {
				newlineRun++
			}
			continue
		}
		if t.Kind() != token.Comment {
			continue
		}
		if newlineRun >= 2 {
			gap = gapBlankline
		}
		newlineRun = 0
		p.emitGap(gap)
		p.push(dom.Text(t.Text()))
		gap = gapNewline
	}
}

func (p *printer) printRange(r ast.DeclRange, gap gapStyle) {
	if !r.KeywordToken().IsZero() {
		p.printToken(r.KeywordToken(), gap)
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
	slots := p.trivia.scopeTrivia(brackets.ID())
	entries := co.Entries()

	if p.options.Format {
		// In format mode, compact options layout is deterministic:
		// - 1 option: inline [key = value]
		// - 2+ options: expanded one-per-line
		if entries.Len() == 1 {
			// Single option: stays inline. No group wrapping, so
			// message literal values expand naturally while keeping
			// [ and ] on the field line.
			p.printToken(openTok, gapSpace)
			opt := entries.At(0)
			p.emitTriviaSlot(slots, 0)
			p.printPath(opt.Path, gapNone)
			if !opt.Equals.IsZero() {
				p.printToken(opt.Equals, gapSpace)
				p.printExpr(opt.Value, gapSpace)
			}
			p.emitTriviaSlot(slots, 1)
			p.emitTrivia(gapNone)
			p.printToken(closeTok, gapNone)
		} else {
			// Multiple options: always expand one-per-line.
			p.printToken(openTok, gapSpace)
			p.withIndent(func(indented *printer) {
				for i := range entries.Len() {
					indented.emitTriviaSlot(slots, i)
					if i > 0 {
						indented.printToken(entries.Comma(i-1), gapNone)
					}
					opt := entries.At(i)
					indented.printPath(opt.Path, gapNewline)
					if !opt.Equals.IsZero() {
						indented.printToken(opt.Equals, gapSpace)
						indented.printExpr(opt.Value, gapSpace)
					}
				}
				indented.emitTriviaSlot(slots, entries.Len())
			})
			p.emitTrivia(gapNone)
			p.printToken(closeTok, gapNewline)
		}
		return
	}

	p.withGroup(func(p *printer) {
		p.printToken(openTok, gapSpace)
		p.withIndent(func(indented *printer) {
			for i := range entries.Len() {
				indented.emitTriviaSlot(slots, i)
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
			p.emitTriviaSlot(slots, entries.Len())
		})
		p.emitTrivia(gapNone)
		p.push(dom.TextIf(dom.Broken, "\n"))
		p.printToken(closeTok, gapNone)
	})
}
