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
	"github.com/bufbuild/protocompile/experimental/token"
)

// printDecl dispatches to the appropriate printer based on declaration kind.
//
// gap controls the whitespace before the declaration's first token. The caller
// determines this based on the declaration's position within its scope (e.g.
// gapNone for the first declaration in a file, gapBlankline between sections).
func (p *printer) printDecl(decl ast.DeclAny, gap gapStyle, ctx printCtx) {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		if p.options.Format {
			return
		}
		p.printToken(decl.AsEmpty().Semicolon(), gap, ctx)
	case ast.DeclKindSyntax:
		p.printSyntax(decl.AsSyntax(), gap, ctx)
	case ast.DeclKindPackage:
		p.printPackage(decl.AsPackage(), gap, ctx)
	case ast.DeclKindImport:
		p.printImport(decl.AsImport(), gap, ctx)
	case ast.DeclKindDef:
		p.printDef(decl.AsDef(), gap, ctx)
	case ast.DeclKindBody:
		p.printBody(decl.AsBody(), ctx)
	case ast.DeclKindRange:
		p.printRange(decl.AsRange(), gap, ctx)
	}
}

func (p *printer) printSyntax(decl ast.DeclSyntax, gap gapStyle, ctx printCtx) {
	p.printToken(decl.KeywordToken(), gap, ctx)
	p.printToken(decl.Equals(), gapSpace, ctx)
	p.printExpr(decl.Value(), gapSpace, ctx)
	p.printCompactOptions(decl.Options(), ctx)
	p.printToken(decl.Semicolon(), p.semiGap(), ctx)
}

func (p *printer) printPackage(decl ast.DeclPackage, gap gapStyle, ctx printCtx) {
	p.printToken(decl.KeywordToken(), gap, ctx)
	p.printPath(decl.Path(), gapSpace, ctx)
	p.printCompactOptions(decl.Options(), ctx)
	p.printToken(decl.Semicolon(), p.semiGap(), ctx)
}

func (p *printer) printImport(decl ast.DeclImport, gap gapStyle, ctx printCtx) {
	p.printToken(decl.KeywordToken(), gap, ctx)
	modifiers := decl.ModifierTokens()
	for i := range modifiers.Len() {
		p.printToken(modifiers.At(i), gapSpace, ctx)
	}
	p.printExpr(decl.ImportPath(), gapSpace, ctx)
	p.printCompactOptions(decl.Options(), ctx)
	p.printToken(decl.Semicolon(), p.semiGap(), ctx)
}

func (p *printer) printDef(decl ast.DeclDef, gap gapStyle, ctx printCtx) {
	switch decl.Classify() {
	case ast.DefKindOption:
		p.printOption(decl.AsOption(), gap, ctx)
	case ast.DefKindMessage:
		p.printMessage(decl.AsMessage(), gap, ctx)
	case ast.DefKindEnum:
		p.printEnum(decl.AsEnum(), gap, ctx)
	case ast.DefKindService:
		p.printService(decl.AsService(), gap, ctx)
	case ast.DefKindField:
		p.printField(decl.AsField(), gap, ctx)
	case ast.DefKindEnumValue:
		p.printEnumValue(decl.AsEnumValue(), gap, ctx)
	case ast.DefKindOneof:
		p.printOneof(decl.AsOneof(), gap, ctx)
	case ast.DefKindMethod:
		p.printMethod(decl.AsMethod(), gap, ctx)
	case ast.DefKindExtend:
		p.printExtend(decl.AsExtend(), gap, ctx)
	case ast.DefKindGroup:
		p.printGroup(decl.AsGroup(), gap, ctx)
	}
}

func (p *printer) printOption(opt ast.DefOption, gap gapStyle, ctx printCtx) {
	p.printToken(opt.Keyword, gap, ctx)
	p.printPath(opt.Path, gapSpace, ctx)
	if !opt.Equals.IsZero() {
		p.printToken(opt.Equals, gapSpace, ctx)
		// Convert trailing // comments to /* */ on the value expression,
		// since the `;` follows on the same line and a line comment
		// would consume it.
		valueCtx := ctx
		valueCtx.lineToBlock = true
		valueCtx.indentExpr = true
		p.printExpr(opt.Value, gapSpace, valueCtx)
	}
	p.printToken(opt.Semicolon, p.semiGap(), ctx)
}

func (p *printer) printMessage(msg ast.DefMessage, gap gapStyle, ctx printCtx) {
	p.printToken(msg.Keyword, gap, ctx)
	p.printToken(msg.Name, gapSpace, ctx)
	p.printBody(msg.Body, ctx)
}

func (p *printer) printEnum(e ast.DefEnum, gap gapStyle, ctx printCtx) {
	p.printToken(e.Keyword, gap, ctx)
	p.printToken(e.Name, gapSpace, ctx)
	p.printBody(e.Body, ctx)
}

func (p *printer) printService(svc ast.DefService, gap gapStyle, ctx printCtx) {
	p.printToken(svc.Keyword, gap, ctx)
	p.printToken(svc.Name, gapSpace, ctx)
	p.printBody(svc.Body, ctx)
}

func (p *printer) printExtend(ext ast.DefExtend, gap gapStyle, ctx printCtx) {
	p.printToken(ext.Keyword, gap, ctx)
	p.printPath(ext.Extendee, gapSpace, ctx)
	p.printBody(ext.Body, ctx)
}

func (p *printer) printOneof(o ast.DefOneof, gap gapStyle, ctx printCtx) {
	p.printToken(o.Keyword, gap, ctx)
	p.printToken(o.Name, gapSpace, ctx)
	p.printBody(o.Body, ctx)
}

func (p *printer) printGroup(g ast.DefGroup, gap gapStyle, ctx printCtx) {
	// Print type prefixes (optional/required/repeated) from the underlying
	// DeclDef, since DefGroup.Keyword is the "group" keyword itself.
	for prefix := range g.Decl.Prefixes() {
		p.printToken(prefix.PrefixToken(), gap, ctx)
		gap = gapSpace
	}

	p.printToken(g.Keyword, gap, ctx)
	p.printToken(g.Name, gapSpace, ctx)
	if !g.Equals.IsZero() {
		p.printToken(g.Equals, gapSpace, ctx)
		p.printExpr(g.Tag, gapSpace, ctx)
	}
	p.printCompactOptions(g.Options, ctx)

	// Use Decl.Body() because DefGroup.Body is not populated by AsGroup().
	p.printBody(g.Decl.Body(), ctx)
}

func (p *printer) printField(f ast.DefField, gap gapStyle, ctx printCtx) {
	p.printType(f.Type, gap, ctx)
	p.printToken(f.Name, gapSpace, ctx)
	if !f.Equals.IsZero() {
		p.printToken(f.Equals, gapSpace, ctx)
		p.printExpr(f.Tag, gapSpace, ctx)
	}
	p.printCompactOptions(f.Options, ctx)
	p.printToken(f.Semicolon, p.semiGap(), ctx)
}

func (p *printer) printEnumValue(ev ast.DefEnumValue, gap gapStyle, ctx printCtx) {
	p.printToken(ev.Name, gap, ctx)
	if !ev.Equals.IsZero() {
		p.printToken(ev.Equals, gapSpace, ctx)
		p.printExpr(ev.Tag, gapSpace, ctx)
	}
	p.printCompactOptions(ev.Options, ctx)
	p.printToken(ev.Semicolon, p.semiGap(), ctx)
}

func (p *printer) printMethod(m ast.DefMethod, gap gapStyle, ctx printCtx) {
	p.printToken(m.Keyword, gap, ctx)
	p.printToken(m.Name, gapSpace, ctx)
	p.printSignature(m.Signature, ctx)
	if !m.Body.IsZero() {
		p.printBody(m.Body, ctx)
	} else {
		p.printToken(m.Decl.Semicolon(), p.semiGap(), ctx)
	}
}

func (p *printer) printSignature(sig ast.Signature, ctx printCtx) {
	if sig.IsZero() {
		return
	}

	inputs := sig.Inputs()
	if !inputs.Brackets().IsZero() {
		p.withGroup(func(p *printer) {
			openTok, closeTok := inputs.Brackets().StartEnd()
			slots := p.trivia.scopeTrivia(inputs.Brackets().ID())
			p.printToken(openTok, gapPreserve, ctx)
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				indented.printTypeListContents(inputs, slots, ctx)
				p.push(tagSoftbreak)
			})
			p.printToken(closeTok, gapPreserve, ctx)
		})
	}

	if !sig.Returns().IsZero() {
		p.printToken(sig.Returns(), gapSpace, ctx)
		outputs := sig.Outputs()
		if !outputs.Brackets().IsZero() {
			p.withGroup(func(p *printer) {
				openTok, closeTok := outputs.Brackets().StartEnd()
				slots := p.trivia.scopeTrivia(outputs.Brackets().ID())
				p.printToken(openTok, gapSpace, ctx)
				p.withIndent(func(indented *printer) {
					indented.push(tagSoftbreak)
					indented.printTypeListContents(outputs, slots, ctx)
					p.push(tagSoftbreak)
				})
				p.printToken(closeTok, gapPreserve, ctx)
			})
		}
	}
}

func (p *printer) printTypeListContents(list ast.TypeList, trivia detachedTrivia, ctx printCtx) {
	gap := gapPreserve
	for i := range list.Len() {
		p.emitTriviaSlot(trivia, i)
		if i > 0 {
			p.printToken(list.Comma(i-1), p.semiGap(), ctx)
			gap = gapSoftline
		}
		p.printType(list.At(i), gap, ctx)
	}
	p.emitRemainingTrivia(trivia, list.Len())
}

func (p *printer) printBody(body ast.DeclBody, ctx printCtx) {
	if body.IsZero() || body.Braces().IsZero() {
		return
	}
	// Line comments are safe on their own lines inside bodies.
	ctx.lineToBlock = false

	openTok, closeTok := body.Braces().StartEnd()
	trivia := p.trivia.scopeTrivia(body.Braces().ID())

	p.printToken(openTok, gapSpace, ctx)

	closeComments, closeAtt := p.extractCloseComments(closeTok)
	hasContent := body.Decls().Len() > 0 || !trivia.isEmpty() || len(closeComments) > 0
	if !hasContent {
		p.printToken(closeTok, gapNone, ctx)
		return
	}

	p.withIndent(func(indented *printer) {
		indented.printScopeDecls(trivia, body.Decls(), scopeBody, ctx)
		// Emit close comments inside the indent block. Also flush
		// any pending slot comments that would otherwise be emitted
		// outside the indent block with wrong indentation.
		if len(closeComments) > 0 || indented.pendingHasComments() {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt, ctx)
}

// emitCloseComments emits close-brace leading comments inside an
// indented context, flushing any pending scope trivia first.
func (p *printer) emitCloseComments(comments []token.Token, blankBeforeClose bool) {
	// First, flush any pending comments (from trivia slots -- these
	// are typically trailing-on-open comments like "{ // comment").
	// These always use gapNewline since they're the first content
	// inside the indent block.
	for _, t := range p.pending {
		if t.Kind() != token.Comment {
			continue
		}
		p.emitGap(gapNewline)
		text := strings.TrimRight(t.Text(), " \t")
		if strings.HasPrefix(text, "/*") {
			p.emitBlockComment(text)
		} else {
			p.push(dom.Text(text))
		}
	}
	p.pending = p.pending[:0]

	// The gap before close comments: use gapBlankline when
	// blankBeforeClose is true (there was a blank line between the
	// last declaration/comment and the close brace in the source).
	gap := gapNewline
	if blankBeforeClose {
		gap = gapBlankline
	}

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
		text := strings.TrimRight(t.Text(), " \t")
		if strings.HasPrefix(text, "/*") {
			p.emitBlockComment(text)
		} else {
			p.push(dom.Text(text))
		}
		gap = gapNewline
	}
}

func (p *printer) printRange(r ast.DeclRange, gap gapStyle, ctx printCtx) {
	if !r.KeywordToken().IsZero() {
		p.printToken(r.KeywordToken(), gap, ctx)
	}

	ranges := r.Ranges()
	for i := range ranges.Len() {
		if i > 0 {
			p.printToken(ranges.Comma(i-1), p.semiGap(), ctx)
		}
		p.printExpr(ranges.At(i), gapSpace, ctx)
	}
	p.printCompactOptions(r.Options(), ctx)
	p.printToken(r.Semicolon(), p.semiGap(), ctx)
}

func (p *printer) printCompactOptions(co ast.CompactOptions, ctx printCtx) {
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
		// Force multi-line if the brackets contain comments that
		// would break inline formatting. Line comments (//) in
		// leading trivia eat the rest of the line and cannot be
		// converted to block comments (lineToBlock only affects
		// trailing trivia).
		openTrailing := p.extractOpenTrailing(openTok)
		forceExpand := len(openTrailing) > 0 ||
			triviaHasComments(slots) ||
			p.scopeHasLeadingLineComments(brackets)
		if entries.Len() == 1 && !forceExpand {
			// Single option: stays inline. No group wrapping, so
			// message literal values expand naturally while keeping
			// [ and ] on the field line. Convert any trailing //
			// comments to /* */ so they don't eat the closing bracket.
			singleCtx := ctx
			singleCtx.lineToBlock = true
			p.printToken(openTok, gapSpace, singleCtx)
			opt := entries.At(0)
			p.emitTriviaSlot(slots, 0)
			p.printPath(opt.Path, gapNone, singleCtx)
			if !opt.Equals.IsZero() {
				p.printToken(opt.Equals, gapSpace, singleCtx)
				valueCtx := singleCtx
				valueCtx.indentExpr = true
				p.printExpr(opt.Value, gapSpace, valueCtx)
			}
			p.emitTriviaSlot(slots, 1)
			p.emitTrivia(gapNone)
			p.printToken(closeTok, gapNone, singleCtx)
		} else {
			// Multiple options or comments force expand: one-per-line.
			// When the open bracket has trailing comments, suppress
			// them from the inline position and emit them as the first
			// line inside the indented block instead.
			if len(openTrailing) > 0 {
				p.printTokenSuppressTrailing(openTok, gapSpace)
			} else {
				p.printToken(openTok, gapSpace, ctx)
			}
			closeComments, closeAtt := p.extractCloseComments(closeTok)
			p.withIndent(func(indented *printer) {
				if len(openTrailing) > 0 {
					// Emit the trailing comments from the open bracket
					// on their own indented lines. The first option's
					// gapNewline provides separation, so we only need
					// to emit the comments themselves.
					for _, t := range openTrailing {
						if t.Kind() == token.Comment {
							indented.emitGap(gapNewline)
							indented.push(dom.Text(strings.TrimRight(t.Text(), " \t")))
						}
					}
				}
				for i := range entries.Len() {
					indented.emitTriviaSlot(slots, i)
					if i > 0 {
						indented.printToken(entries.Comma(i-1), p.semiGap(), ctx)
					}
					opt := entries.At(i)
					indented.printPath(opt.Path, gapNewline, ctx)
					if !opt.Equals.IsZero() {
						indented.printToken(opt.Equals, gapSpace, ctx)
						valueCtx := ctx
						valueCtx.indentExpr = true
						indented.printExpr(opt.Value, gapSpace, valueCtx)
					}
				}
				indented.emitTriviaSlot(slots, entries.Len())
				if len(closeComments) > 0 {
					indented.emitCloseComments(closeComments, slots.blankBeforeClose)
				}
			})
			p.emitTrivia(gapNone)
			p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt, ctx)
		}
		return
	}

	p.withGroup(func(p *printer) {
		p.printToken(openTok, gapSpace, ctx)
		p.withIndent(func(indented *printer) {
			for i := range entries.Len() {
				indented.emitTriviaSlot(slots, i)
				opt := entries.At(i)
				if i > 0 {
					indented.printToken(entries.Comma(i-1), p.semiGap(), ctx)
					indented.printPath(opt.Path, gapSoftline, ctx)
				} else {
					indented.printPath(opt.Path, gapNone, ctx)
				}

				if !opt.Equals.IsZero() {
					indented.printToken(opt.Equals, gapSpace, ctx)
					indented.printExpr(opt.Value, gapSpace, ctx)
				}
			}
			p.emitTriviaSlot(slots, entries.Len())
		})
		p.emitTrivia(gapNone)
		p.push(tagSoftbreak)
		p.printToken(closeTok, gapNone, ctx)
	})
}
