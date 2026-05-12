// Copyright 2020-2026 Buf Technologies, Inc.
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
	"github.com/bufbuild/protocompile/experimental/token"
)

// firstOptionKeyHasLeadingComment reports whether the first option's
// path begins with a leading comment. Such comments cannot render
// cleanly inline since `[/* comment */ key = value]` would force a
// softline break after the comment in the broken (file-level) context.
func (p *printer) firstOptionKeyHasLeadingComment(entries ast.Commas[ast.Option]) bool {
	if entries.Len() == 0 {
		return false
	}
	firstTok := pathFirstToken(entries.At(0).Path)
	if firstTok.IsZero() {
		return false
	}
	att, ok := p.trivia.tokenTrivia(firstTok.ID())
	if !ok {
		return false
	}
	return sliceHasComment(att.leading)
}

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
	p.printToken(decl.Semicolon(), p.semiGap())
}

func (p *printer) printPackage(decl ast.DeclPackage, gap gapStyle) {
	p.printToken(decl.KeywordToken(), gap)
	p.printPath(decl.Path(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), p.semiGap())
}

func (p *printer) printImport(decl ast.DeclImport, gap gapStyle) {
	p.printToken(decl.KeywordToken(), gap)
	modifiers := decl.ModifierTokens()
	for i := range modifiers.Len() {
		p.printToken(modifiers.At(i), gapSpace)
	}
	p.printExpr(decl.ImportPath(), gapSpace)
	p.printCompactOptions(decl.Options())
	p.printToken(decl.Semicolon(), p.semiGap())
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
	case ast.DefKindInvalid:
		p.printInvalidDef(decl, gap)
	}
}

// printInvalidDef prints a corrupt or unrecognized definition by emitting
// whatever tokens it has, in declaration order. This preserves source text
// for parse-error artifacts rather than silently dropping them.
func (p *printer) printInvalidDef(decl ast.DeclDef, gap gapStyle) {
	p.printType(decl.Type(), gap)
	p.printPath(decl.Name(), gapSpace)
	p.printSignature(decl.Signature())
	if !decl.Equals().IsZero() {
		p.printToken(decl.Equals(), gapSpace)
		p.printExpr(decl.Value(), gapSpace)
	}
	p.printCompactOptions(decl.Options())
	if !decl.Body().IsZero() {
		p.printBody(decl.Body())
	} else {
		p.printToken(decl.Semicolon(), p.semiGap())
	}
}

func (p *printer) printOption(opt ast.DefOption, gap gapStyle) {
	p.printToken(opt.Keyword, gap)
	p.printPath(opt.Path, gapSpace)
	if !opt.Equals.IsZero() {
		p.printToken(opt.Equals, gapSpace)
		// Convert trailing // comments to /* */ on the value expression,
		// since the `;` follows on the same line and a line comment
		// would consume it.
		restore := p.ctx.with(lineToBlock(true), indentExpr(true))
		p.printExpr(opt.Value, gapSpace)
		restore()
	}
	p.printToken(opt.Semicolon, p.semiGap())
}

func (p *printer) printMessage(msg ast.DefMessage, gap gapStyle) {
	p.printToken(msg.Keyword, gap)
	p.printPath(msg.Decl.Name(), gapSpace)
	p.printBody(msg.Body)
}

func (p *printer) printEnum(e ast.DefEnum, gap gapStyle) {
	p.printToken(e.Keyword, gap)
	p.printPath(e.Decl.Name(), gapSpace)
	p.printBody(e.Body)
}

func (p *printer) printService(svc ast.DefService, gap gapStyle) {
	p.printToken(svc.Keyword, gap)
	p.printPath(svc.Decl.Name(), gapSpace)
	p.printBody(svc.Body)
}

func (p *printer) printExtend(ext ast.DefExtend, gap gapStyle) {
	p.printToken(ext.Keyword, gap)
	p.printPath(ext.Extendee, gapSpace)
	p.printBody(ext.Body)
}

func (p *printer) printOneof(o ast.DefOneof, gap gapStyle) {
	p.printToken(o.Keyword, gap)
	p.printPath(o.Decl.Name(), gapSpace)
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
	p.printPath(g.Decl.Name(), gapSpace)
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
	p.printPath(f.Decl.Name(), gapSpace)
	if !f.Equals.IsZero() {
		p.printToken(f.Equals, gapSpace)
		p.printExpr(f.Tag, gapSpace)
	}
	p.printCompactOptions(f.Options)
	p.printToken(f.Semicolon, p.semiGap())
}

func (p *printer) printEnumValue(ev ast.DefEnumValue, gap gapStyle) {
	p.printPath(ev.Decl.Name(), gap)
	if !ev.Equals.IsZero() {
		p.printToken(ev.Equals, gapSpace)
		p.printExpr(ev.Tag, gapSpace)
	}
	p.printCompactOptions(ev.Options)
	p.printToken(ev.Semicolon, p.semiGap())
}

func (p *printer) printMethod(m ast.DefMethod, gap gapStyle) {
	p.printToken(m.Keyword, gap)
	p.printPath(m.Decl.Name(), gapSpace)
	p.printSignature(m.Signature)
	if !m.Body.IsZero() {
		p.printBody(m.Body)
	} else {
		p.printToken(m.Decl.Semicolon(), p.semiGap())
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
			slots := p.trivia.scopeTrivia(inputs.Brackets().ID())
			p.printToken(openTok, gapPreserve)
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				indented.printTypeListContents(inputs, slots)
				p.push(tagSoftbreak)
			})
			p.printToken(closeTok, gapPreserve)
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
					indented.push(tagSoftbreak)
					indented.printTypeListContents(outputs, slots)
					p.push(tagSoftbreak)
				})
				p.printToken(closeTok, gapPreserve)
			})
		}
	}
}

func (p *printer) printTypeListContents(list ast.TypeList, trivia detachedTrivia) {
	gap := gapPreserve
	for i := range list.Len() {
		p.emitTriviaSlot(trivia, i)
		if i > 0 {
			p.printToken(list.Comma(i-1), p.semiGap())
			gap = gapSoftline
		}
		p.printType(list.At(i), gap)
	}
	p.emitRemainingTrivia(trivia, list.Len())
}

// printBody prints a decl-bearing body scope (`{ ... }` on message,
// enum, service, oneof, extend, or RPC method).
//
// In non-format mode, decls are emitted as-is with their source
// trivia. In format mode, the layout decision is governed by
// [printer.bodyShouldBreak] modulo a force-broken signal raised when
// the scope contains comments anywhere (which need their own lines).
// An empty body collapses to `{}`.
func (p *printer) printBody(body ast.DeclBody) {
	if body.IsZero() || body.Braces().IsZero() {
		return
	}
	// Line comments are safe on their own lines inside bodies; reset
	// the inline-conversion flag for the body's scope.
	defer p.ctx.with(lineToBlock(false))()

	openTok, closeTok := body.Braces().StartEnd()
	trivia := p.trivia.scopeTrivia(body.Braces().ID())

	p.printToken(openTok, gapSpace)

	closeComments, closeAtt := p.extractCloseComments(closeTok)
	hasContent := body.Decls().Len() > 0 || !trivia.isEmpty() || len(closeComments) > 0
	if !hasContent {
		p.printToken(closeTok, gapNone)
		return
	}

	if p.options.Format {
		// LayoutDynamic can keep a non-empty body flat when the source
		// had it flat and there are no comments anywhere in the scope.
		// Comments anywhere force broken so per-decl gap handling and
		// trivia slots remain on their own lines. Whitespace-only
		// trivia between decls is fine; only comments matter here.
		forceBroken := triviaHasComments(trivia) ||
			len(closeComments) > 0 ||
			p.scopeHasAttachedComments(body.Braces())
		if !forceBroken && !p.bodyShouldBreak(openTok, closeTok) {
			decls := body.Decls()
			p.withGroup(func(p *printer) {
				p.withIndent(func(indented *printer) {
					for i := range decls.Len() {
						indented.printDecl(decls.At(i), gapSoftline)
					}
				})
				p.push(tagSoftlineFlat, tagSoftbreak)
			})
			p.printToken(closeTok, gapNone)
			return
		}
	}

	p.withIndent(func(indented *printer) {
		indented.printScopeDecls(trivia, body.Decls(), scopeBody)
		// Emit close comments inside the indent block. Also flush
		// any pending slot comments that would otherwise be emitted
		// outside the indent block with wrong indentation.
		if len(closeComments) > 0 || indented.pendingHasComments() {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt)
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
		p.emitComment(t)
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
		p.emitComment(t)
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
			p.printToken(ranges.Comma(i-1), p.semiGap())
		}
		p.printExpr(ranges.At(i), gapSpace)
	}
	p.printCompactOptions(r.Options())
	p.printToken(r.Semicolon(), p.semiGap())
}

// printCompactOptions prints a `[ ... ]` compact-options bracket
// attached to a field, enum value, range, or other decl that carries
// inline options.
//
// In format mode the helper picks one of three layouts:
//
//   - Single-entry inline: `[opt = value]` on the field line. Under
//     [LayoutDynamic] the entry is wrapped in a [dom.Group] so that
//     an inner break (a width-broken nested literal, a hard newline)
//     propagates upward and the brackets follow.
//
//   - Multi-entry flat: `[a = 1, b = 2]` inside a [dom.Group] with
//     softline separators, breaking on width.
//
//   - Expanded (one entry per line): triggered by 2+ entries under
//     [LayoutStrict], by scope-attached comments, or by `//` line
//     comments that would otherwise consume the closing bracket
//     when [Formatting.RewriteTrailingLineCommentsToBlock] is false.
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
		openTrailing := p.extractOpenTrailing(openTok)

		// For formatted compact option elements, force multi-line expansion if the
		// brackets contain comments that would break inline formatting.
		//
		// Comments in leading trivia are not coerced to block comments via
		// lineToBlock (which only affects trailing trivia): line comments eat the
		// rest of the line, and block comments produce softline gaps that break
		// outside the indent wrapper.
		forceExpand := len(openTrailing) > 0 ||
			triviaHasComments(slots) ||
			p.scopeHasUninlineableLeadingComments(brackets) ||
			p.firstOptionKeyHasLeadingComment(entries)
		// Layout fallback: when trailing `//` rewrite is disabled, a `//`
		// inside an inline `[...]` would consume the closing bracket.
		// Force broken so the comment terminates safely on its own line.
		if !p.options.Formatting.RewriteTrailingLineCommentsToBlock &&
			p.scopeHasLineTrailingComments(brackets) {
			forceExpand = true
		}
		wantBroken := forceExpand || p.literalShouldBreak(openTok, closeTok, entries.Len())

		switch {
		case !wantBroken && entries.Len() == 1:
			// Single option, flat. Convert any trailing // comments to
			// /* */ so they don't eat the closing bracket.
			//
			// Under [LayoutDynamic], wrap the entry in a [dom.Group] so
			// an inner break (from a width-broken nested literal, or a
			// hard newline) propagates upward and the brackets follow:
			//   [(opt) = {                          [
			//     long: "value"           becomes     (opt) = {
			//   }]                                      long: "value"
			//                                         }
			//                                       ]
			// Under [LayoutStrict], the brackets stay flush with the
			// field line and the value expands naturally inside,
			// matching the legacy formatter byte-for-byte.
			singleRestore := p.ctx.with(lineToBlock(true))
			opt := entries.At(0)
			emitEntry := func(p *printer) {
				p.emitTriviaSlot(slots, 0)
				p.printPath(opt.Path, gapNone)
				if !opt.Equals.IsZero() {
					p.printToken(opt.Equals, gapSpace)
					valueRestore := p.ctx.with(indentExpr(true))
					p.printExpr(opt.Value, gapSpace)
					valueRestore()
				}
				p.emitTriviaSlot(slots, 1)
				p.emitTrivia(gapNone)
			}
			if p.options.Formatting.LiteralLayout == LayoutDynamic {
				p.printToken(openTok, gapSpace)
				p.withGroup(func(p *printer) {
					p.withIndent(func(indented *printer) {
						indented.push(tagSoftbreak)
						emitEntry(indented)
					})
					p.push(tagSoftbreak)
				})
				p.printToken(closeTok, gapNone)
			} else {
				p.printToken(openTok, gapSpace)
				emitEntry(p)
				p.printToken(closeTok, gapNone)
			}
			singleRestore()

		case !wantBroken:
			// Multi-option, flat: emit inside a group with softbreak
			// padding and softline separators so the group breaks if
			// flat width exceeds MaxWidth.
			p.printToken(openTok, gapSpace)
			p.withGroup(func(p *printer) {
				p.withIndent(func(indented *printer) {
					indented.push(tagSoftbreak)
					for i := range entries.Len() {
						indented.emitTriviaSlot(slots, i)
						opt := entries.At(i)
						if i > 0 {
							indented.printToken(entries.Comma(i-1), gapNone)
							indented.printPath(opt.Path, gapSoftline)
						} else {
							indented.printPath(opt.Path, gapNone)
						}
						if !opt.Equals.IsZero() {
							indented.printToken(opt.Equals, gapSpace)
							restore := p.ctx.with(indentExpr(true))
							indented.printExpr(opt.Value, gapSpace)
							restore()
						}
					}
					indented.emitTriviaSlot(slots, entries.Len())
				})
				p.push(tagSoftbreak)
			})
			p.printToken(closeTok, gapNone)

		default:
			// Multiple options or comments force expand: one-per-line.
			// pairLeadingBlock is intentionally NOT set here: the
			// legacy formatter does not pair leading block comments
			// with compact option entries (only with array elements).
			defer p.ctx.with(trailingBlockOnNewLine(true), pairLeadingBlock(false))()
			// Print the open bracket inline, including its trailing
			// trivia. A trailing `// note` after `[` ends the line
			// naturally and the first entry follows on a new
			// indented line via gapNewline; a trailing block comment
			// `/* note */` also stays inline with the bracket. This
			// matches the legacy formatter's `[ // note\n  ...` style.
			p.printToken(openTok, gapSpace)
			closeComments, closeAtt := p.extractCloseComments(closeTok)
			p.withIndent(func(indented *printer) {
				for i := range entries.Len() {
					// Emit the comma (and its trailing) first; then the
					// detached slot[i] between comma and this entry;
					// then the entry itself.
					//
					// Suppress trailingBlockOnNewLine for the comma's
					// emit so a trailing `*/` on it stays inline.
					if i > 0 {
						restore := indented.ctx.with(trailingBlockOnNewLine(false))
						indented.printToken(entries.Comma(i-1), p.semiGap())
						restore()
					}
					indented.emitTriviaSlot(slots, i)
					opt := entries.At(i)
					optGap := gapNewline
					if i > 0 && slots.hasBlankBefore(i) {
						optGap = gapBlankline
					}
					indented.printPath(opt.Path, optGap)
					if !opt.Equals.IsZero() {
						indented.printToken(opt.Equals, gapSpace)
						restore := p.ctx.with(indentExpr(true), pathInValueContext(true))
						indented.printExpr(opt.Value, gapSpace)
						restore()
					}
				}
				indented.emitTriviaSlot(slots, entries.Len())
				if len(closeComments) > 0 {
					indented.emitCloseComments(closeComments, slots.blankBeforeClose)
				}
			})
			p.emitTrivia(gapNone)
			p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt)
		}
		return
	}

	p.withGroup(func(p *printer) {
		p.printToken(openTok, gapSpace)
		p.withIndent(func(indented *printer) {
			for i := range entries.Len() {
				indented.emitTriviaSlot(slots, i)
				opt := entries.At(i)
				if i > 0 {
					indented.printToken(entries.Comma(i-1), p.semiGap())
					indented.printPath(opt.Path, gapSoftline)
				} else {
					indented.printPath(opt.Path, gapNone)
				}

				if !opt.Equals.IsZero() {
					indented.printToken(opt.Equals, gapSpace)
					indented.printExpr(opt.Value, gapSpace)
				}
			}
			p.emitTriviaSlot(slots, entries.Len())
		})
		p.emitTrivia(gapNone)
		p.push(tagSoftbreak)
		p.printToken(closeTok, gapNone)
	})
}
