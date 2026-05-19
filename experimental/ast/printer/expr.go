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

	// In format mode, all parts go on their own lines.
	// Clear lineToBlock for the parts: intermediate // comments
	// between string parts are on their own lines and are safe as-is.
	// The restorer below scopes the change and is also invoked inline
	// before the closing part's trailing emit -- a trailing // there
	// would eat the following token (`;`, `]`, etc) if the caller
	// requested conversion, so we rewind to the caller's state for
	// that one emit. The restorer is idempotent, so the deferred call
	// at function exit is safe regardless.
	// pairLeadingBlock is reset for the compound-string body: a
	// surrounding broken array sets it true to inline-pair leading
	// block comments with the array element (the compound string as
	// a whole), but interior block comments between parts must stay
	// on their own line.
	//
	// trailingBlockOnNewLine is set based on whether the source's
	// compound string spanned multiple lines: the legacy formatter
	// preserves source layout. For a source-vertical compound
	// (parts on separate lines), trailing block comments stay
	// inline with their part. For a source-flat compound (all on
	// one line, which we break out vertically), trailing block
	// comments get their own line.
	srcVertical := !sourceWasFlat(openTok, closeTok)
	indented := p.ctx.indentExpr
	restore := p.ctx.with(
		lineToBlock(false),
		trailingBlockOnNewLine(!srcVertical),
		pairLeadingBlock(false),
	)
	defer restore()

	printParts := func(pp *printer) {
		pp.printTokenAs(tok, gapNewline, openTok.Text())
		for i, part := range parts {
			pp.emitTriviaSlot(trivia, i)
			pp.printToken(part, gapNewline)
		}
		pp.emitRemainingTrivia(trivia, len(parts))

		// Emit the last part's leading trivia with conversion off.
		att, hasTrivia := pp.trivia.tokenTrivia(closeTok.ID())
		if hasTrivia {
			pp.appendPending(att.leading)
			pp.emitTrivia(gapNewline)
		} else {
			pp.emitGap(gapNewline)
		}
		pp.push(dom.Text(closeTok.Text()))
		if hasTrivia {
			// Rewind to the caller's state so the trailing emit
			// uses the caller's lineToBlock.
			restore()
			pp.emitTrailing(att.trailing)
		}
	}

	if indented {
		// After `=` or `:`. Indent parts one level so they break
		// under the assignment.
		p.withIndent(func(indented *printer) {
			printParts(indented)
		})
	} else {
		// Already in an indented context (e.g., array elements).
		// Parts go at the current indent level.
		printParts(p)
	}
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
			firstTok = pathFirstToken(inner.AsPath().Path)
		}
		if !firstTok.IsZero() {
			if att, ok := p.trivia.tokenTrivia(firstTok.ID()); ok {
				if sliceHasComment(att.leading) {
					valueGap = gapSpace
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

	// Containers manage their own indentation; both flags reset for the
	// scope of this array. Capture the outer lineToBlock value so the
	// close-bracket emit can restore it: a `// comment` trailing on `]`
	// is a boundary token whose rewrite policy comes from the enclosing
	// scope (e.g. a dict field whose value is this array).
	outerLineToBlock := p.ctx.lineToBlock
	defer p.ctx.with(lineToBlock(false), indentExpr(false))()

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
		restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
		p.printToken(closeTok, gapNone)
		restoreClose()
		return
	}

	wantBroken := hasComments || p.literalShouldBreak(openTok, closeTok, elements.Len())

	if !wantBroken {
		// Flat path: emit the elements inside a group with softbreak
		// padding and softline separators. dom.Group will auto-break
		// the group if its flat width exceeds MaxWidth or if any
		// element contains a newline.
		p.printToken(openTok, gap)
		p.withGroup(func(p *printer) {
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				for i := range elements.Len() {
					indented.emitTriviaSlot(slots, i)
					elemGap := gapNone
					if i > 0 {
						indented.printToken(elements.Comma(i-1), gapNone)
						elemGap = gapSoftline
					}
					indented.printExpr(elements.At(i), elemGap)
				}
				indented.emitTriviaSlot(slots, elements.Len())
			})
			p.push(tagSoftbreak)
		})
		restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
		p.printToken(closeTok, gapNone)
		restoreClose()
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	defer p.ctx.with(trailingBlockOnNewLine(true), pairLeadingBlock(true))()

	p.printToken(openTok, gap)
	p.withIndent(func(indented *printer) {
		for i := range elements.Len() {
			// Comma is the boundary token of the previous element in
			// the trivia walker, so emit it (and its trailing) first;
			// then emit the detached slot[i] which holds between-comma
			// and-this-element trivia; then the element itself.
			//
			// Suppress trailingBlockOnNewLine while emitting the
			// comma so a `*/` trailing on the comma stays inline
			// with it: the legacy formatter only puts trailing-on-
			// VALUE block comments on their own line, not trailing-
			// on-comma.
			if i > 0 {
				restore := indented.ctx.with(trailingBlockOnNewLine(false))
				indented.printToken(elements.Comma(i-1), p.semiGap())
				restore()
			}
			indented.emitTriviaSlot(slots, i)
			elemGap := gapNewline
			if i > 0 && slots.hasBlankBefore(i) {
				elemGap = gapBlankline
			}
			// Array elements are values — let trailing block comments
			// on path-final tokens respect the surrounding broken
			// scope's trailingBlockOnNewLine policy.
			restore := indented.ctx.with(pathInValueContext(true))
			indented.printExpr(elements.At(i), elemGap)
			restore()
		}
		indented.emitTriviaSlot(slots, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, slots.blankBeforeClose)
		}
	})

	// Always emit `]` on its own line in broken layout, even when
	// source glued the bracket to the last element. This is a
	// style divergence from the legacy formatter, which preserves
	// the source's glued placement (e.g. `{foo: 98}]`). Consistent
	// newline placement is more readable. Exercised by
	// TestFormat/message_literals.proto.
	restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
	p.emitCloseTok(closeTok, closeTok.Text(), closeComments, closeAtt)
	restoreClose()
}

func (p *printer) printDict(expr ast.ExprDict, gap gapStyle) {
	if expr.IsZero() {
		return
	}
	// Containers manage their own indentation; both flags reset for the
	// scope of this dict. Capture the outer lineToBlock value so the
	// close-brace emit can restore it: a `// comment` trailing on `}` is
	// a boundary token whose rewrite policy comes from the enclosing
	// scope (e.g. a dict field whose value is this nested dict).
	outerLineToBlock := p.ctx.lineToBlock
	defer p.ctx.with(lineToBlock(false), indentExpr(false))()

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
					indented.emitCommaTrivia(elements.Comma(i))
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
		restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
		p.printTokenAs(closeTok, gapNone, closeText)
		restoreClose()
		return
	}

	wantBroken := hasComments || p.literalShouldBreak(openTok, closeTok, elements.Len())

	if !wantBroken {
		// Flat path: emit fields inside a group with softbreak padding
		// and softline separators between fields. Message-literal fields
		// have no explicit separator emitted; the softline (space when
		// flat, newline when broken) suffices.
		p.printTokenAs(openTok, gap, openText)
		p.withGroup(func(p *printer) {
			p.withIndent(func(indented *printer) {
				indented.push(tagSoftbreak)
				for i := range elements.Len() {
					indented.emitTriviaSlot(trivia, i)
					fieldGap := gapNone
					if i > 0 {
						fieldGap = gapSoftline
					}
					indented.printExprField(elements.At(i), fieldGap)
					indented.emitCommaTrivia(elements.Comma(i))
				}
				indented.emitTriviaSlot(trivia, elements.Len())
			})
			p.push(tagSoftbreak)
		})
		restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
		p.printTokenAs(closeTok, gapNone, closeText)
		restoreClose()
		return
	}

	closeComments, closeAtt := p.extractCloseComments(closeTok)

	// pairLeadingBlock is intentionally NOT set here: legacy buf
	// format does not pair leading block comments with dict fields
	// (only with array elements).
	defer p.ctx.with(trailingBlockOnNewLine(true), pairLeadingBlock(false))()

	// Check if the open brace has trailing comments that should be
	// moved inside the indented block.
	openTrailing := p.extractOpenTrailing(openTok)

	if len(openTrailing) > 0 {
		// Suppress trailing on open brace; emit inside indent block.
		// Cannot use printTokenAs here because we need to suppress
		// trailing and may need replacement text (angle -> brace).
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
			fieldGap := gapNewline
			if i > 0 {
				// Prefer walker-recorded blankBefore (works when
				// source has comma boundaries); otherwise fall back
				// to span-based detection (works when source elides
				// commas — e.g. the legacy formatter's emitted
				// output, which we re-format on idempotency passes).
				if trivia.hasBlankBefore(i) ||
					sourceBlankLineBetweenFields(elements.At(i-1), elements.At(i)) {
					fieldGap = gapBlankline
				}
			}
			// The legacy formatter rewrites `// comment` trailings to
			// `/* comment */` only when the field value ends in a
			// closing scope bracket (`]` or `}`) — i.e. when the
			// value is itself an array or dict literal. Primitive
			// values (literals, paths) keep their `//` trailings.
			// The flag is set for both the value emit (so a trailing
			// on `]`/`}` rewrites via printArray/printDict's close-
			// token restore) and the comma-trivia emit (so a `// `
			// on the elided comma also rewrites).
			field := elements.At(i)
			valueKind := field.Value().Kind()
			rewriteFieldTrailing := valueKind == ast.ExprKindArray || valueKind == ast.ExprKindDict
			fieldRestore := func() {}
			if rewriteFieldTrailing {
				fieldRestore = indented.ctx.with(lineToBlock(true))
			}
			indented.printExprField(field, fieldGap)
			// Trailing on the comma should stay inline (the legacy
			// formatter only puts trailing-on-VALUE block comments
			// on their own line, not trailing-on-comma).
			commaMods := []modifier{trailingBlockOnNewLine(false)}
			if rewriteFieldTrailing {
				commaMods = append(commaMods, lineToBlock(true))
			}
			restore := indented.ctx.with(commaMods...)
			indented.emitCommaTrivia(elements.Comma(i))
			restore()
			fieldRestore()
		}
		indented.emitTriviaSlot(trivia, elements.Len())
		if len(closeComments) > 0 {
			indented.emitCloseComments(closeComments, trivia.blankBeforeClose)
		}
	})

	restoreClose := p.ctx.with(closeTrailingMods(outerLineToBlock)...)
	p.emitCloseTok(closeTok, closeText, closeComments, closeAtt)
	restoreClose()
}

// closeTrailingMods returns the context modifiers to apply around a
// literal-scope close-token emit so that a trailing comment on `]`/`}`
// is rendered consistently with one attached to a following comma.
//
// outerLineToBlock is the lineToBlock value captured before the
// containing array/dict reset it to false; restoring it lets a trailing
// `// comment` on the close-bracket be rewritten when the enclosing
// scope (a dict field) wants such trailings rewritten.
//
// When outerLineToBlock is true, trailingBlockOnNewLine is also forced
// false: a trailing block comment on `]`/`}` in dict-field-value
// position should pair with the close-bracket on one line (matching how
// the same comment attached to the elided comma would render). Without
// this, idempotency breaks — first-pass output writes the comment
// inline; on re-parse it attaches to `]`/`}`; second-pass would push it
// to its own line under the broken-scope's trailingBlockOnNewLine.
func closeTrailingMods(outerLineToBlock bool) []modifier {
	mods := []modifier{lineToBlock(outerLineToBlock)}
	if outerLineToBlock {
		mods = append(mods, trailingBlockOnNewLine(false))
	}
	return mods
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
		// gapInline ensures comments between the key and colon get a
		// space before them but the colon follows immediately after the
		// last comment with no gap. This prevents an idempotency issue
		// where trivia between ] and : gets assigned differently on
		// reparse (leading vs trailing) depending on line breaks.
		//
		// Exception: when the colon's leading trivia ends in an inline
		// block comment (e.g. extension key `[ ... ] /* Three */ :`),
		// gapInline would emit `*/:` glued. Switch to gapSpace so the
		// `:` follows a separating space.
		colonGap := gapInline
		if att, ok := p.trivia.tokenTrivia(expr.Colon().ID()); ok {
			if pendingEndsWithInlineBlockComment(att.leading) {
				colonGap = gapSpace
			}
		}
		p.printToken(expr.Colon(), colonGap)
	} else if p.options.Format && !expr.Key().IsZero() && !expr.Value().IsZero() {
		// Insert colon in format mode when missing (e.g. "e []" -> "e: []").
		p.push(dom.Text(":"))
	}
	if !expr.Value().IsZero() {
		valueGap := gapSpace
		if first {
			valueGap = gap
		}
		restore := p.ctx.with(indentExpr(true), pathInValueContext(true))
		p.printExpr(expr.Value(), valueGap)
		restore()
	}
}
