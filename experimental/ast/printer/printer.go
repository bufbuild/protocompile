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
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
)

// gapStyle specifies the whitespace intent before a token.
type gapStyle int

const (
	gapNone gapStyle = iota
	gapSpace
	gapNewline
	gapSoftline      // gapSoftline inserts a space if the group is flat, or a newline if the group is broken
	gapBlankline     // gapBlankline inserts two newline characters
	gapInline        // gapInline acts like gapNone when there are no comments; when there are comments, it spaces around them
	gapPreserve      // gapPreserve is like gapNone but inherits the source's spacing around comments
	gapPreserveTight // gapPreserveTight is like gapPreserve but suppresses the inherited space before the first comment
)

// scopeKind distinguishes file-level scopes from body-level scopes,
// which have different gap rules.
type scopeKind int

const (
	scopeFile scopeKind = iota
	scopeBody
)

// PrintFile renders an AST file to protobuf source text.
func PrintFile(options Options, file *ast.File) string {
	options = options.withDefaults()

	// In format mode, a file with no declarations and no comments
	// produces empty output. The dom renderer always appends a trailing
	// newline, so we short-circuit here.
	if options.Format && file.Decls().Len() == 0 {
		trivia := buildTriviaIndex(file.Stream())
		scope := trivia.scopeTrivia(0)
		if !triviaHasComments(scope) {
			return ""
		}
	}

	return dom.Render(options.domOptions(), func(push dom.Sink) {
		p := newPrinter(options, buildTriviaIndex(file.Stream()), push)
		p.printFile(file)
	})
}

// Print renders a snippet for an AST declaration to protobuf source text.
//
// The output reflects the declaration in its source position, including the
// [detachedTrivia] at that position (e.g. a section comment that preceded
// the decl).
//
// Currently only top-level declarations preserve their leading detached trivia.
// Nested declarations (inside a message body, for example) emit their attached
// leading trivia but not the body scope's detached slot at their position.
//
// For printing entire files, use [PrintFile] instead.
func Print(options Options, decl ast.DeclAny) string {
	options = options.withDefaults()
	domOpts := options.domOptions()

	// Explicitly set OmitTrailingNewline to true to avoid [dom] from padding the
	// output with a trailing newlines, allowing callers to manage the spacing of
	// the output.
	domOpts.OmitTrailingNewline = true
	return dom.Render(domOpts, func(push dom.Sink) {
		file := decl.Context()
		trivia := buildTriviaIndex(file.Stream())
		p := newPrinter(options, trivia, push)

		// Emit the decl's preceding scope slot for top-level decls so that section
		// comments at the decl's source position render alongside the decl. Nested
		// decls fall back to attached leading trivia only.
		if i, ok := fileDeclIndex(file, decl); ok {
			p.emitTriviaSlot(trivia.scopeTrivia(0), i)
		}
		p.printDecl(decl, gapNewline)

		// If non-format mode left any trailing trivia in pending after the last
		// token, flush it.
		p.emitTrivia(gapNone)
	})
}

// fileDeclIndex returns the index of decl in file.Decls() at the top
// level, or false if decl is not a top-level declaration.
func fileDeclIndex(file *ast.File, decl ast.DeclAny) (int, bool) {
	decls := file.Decls()
	for i := range decls.Len() {
		if decls.At(i) == decl {
			return i, true
		}
	}
	return 0, false
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	options Options
	trivia  *triviaIndex
	pending []token.Token
	push    dom.Sink

	// indent is the cached indentation string applied by [printer.withIndent],
	// computed once from [Options.TabstopWidth] at construction time to avoid
	// re-allocating on every nested scope.
	indent string

	// ctx stores expected formatting behaviours based on the scope of the
	// printed entity.
	ctx *context
}

// newPrinter constructs a printer for the given options, trivia index,
// and dom sink. options should already have defaults applied via
// [Options.withDefaults].
func newPrinter(options Options, trivia *triviaIndex, push dom.Sink) *printer {
	return &printer{
		options: options,
		trivia:  trivia,
		push:    push,
		indent:  strings.Repeat(" ", options.TabstopWidth),
		ctx:     new(context),
	}
}

// printFile prints all declarations in a file, zipping with trivia slots.
func (p *printer) printFile(file *ast.File) {
	trivia := p.trivia.scopeTrivia(0)
	decls := seq.Indexer[ast.DeclAny](file.Decls())
	if p.options.Format {
		sorted := seq.ToSlice(decls)
		sortFileDeclsForFormat(sorted)
		decls = seq.NewFunc(len(sorted), func(i int) ast.DeclAny {
			return sorted[i]
		})
	}
	p.printScopeDecls(trivia, decls, scopeFile)
	// In format mode, trailing file comments need a newline gap so they
	// don't run into the last declaration's closing token. But if there
	// are no declarations at all, emit nothing (empty file = empty output).
	endGap := gapNone
	if p.options.Format {
		if decls.Len() > 0 || p.pendingHasComments() {
			endGap = gapNewline
			// If the last declaration's trailing trivia had a blank
			// line before the remaining tokens, preserve it for EOF
			// comments separated from the last declaration.
			if trivia.blankBeforeClose && p.pendingHasComments() {
				endGap = gapBlankline
			}
		}
	}
	p.emitTrivia(endGap)
}

// pendingHasComments reports whether pending contains comments.
func (p *printer) pendingHasComments() bool {
	return sliceHasComment(p.pending)
}

// printToken emits a token with its trivia.
func (p *printer) printToken(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}
	p.printTokenAs(tok, gap, tok.Text())
}

// printTokenSuppressTrailing prints a token with its leading trivia but
// suppresses its trailing trivia. The caller is responsible for emitting
// the trailing trivia elsewhere (e.g., inside an indented block).
func (p *printer) printTokenSuppressTrailing(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}
	att, hasTrivia := p.trivia.tokenTrivia(tok.ID())
	if hasTrivia {
		p.appendPending(att.leading)
		if !p.options.Format {
			gap = gapNone
		}
		p.emitTrivia(gap)
	} else {
		p.emitGap(gap)
	}
	p.push(dom.Text(tok.Text()))
	// Trailing trivia intentionally not emitted.
}

// printTokenAs prints a token using replacement text instead of the token's
// own text. This is used for normalizing delimiters (e.g., angle brackets
// to curly braces) while preserving the token's attached trivia.
func (p *printer) printTokenAs(tok token.Token, gap gapStyle, text string) {
	att, hasTrivia := p.trivia.tokenTrivia(tok.ID())
	if hasTrivia {
		p.appendPending(att.leading)
	}

	if len(text) > 0 {
		if hasTrivia {
			if !p.options.Format {
				// In non-format mode, ignore caller's gap and print trivia as-is,
				// which may or may not include whitespace.
				gap = gapNone
			}
			p.emitTrivia(gap)
		} else {
			p.emitGap(gap)
		}

		p.push(dom.Text(text))
	}

	if hasTrivia {
		p.emitTrailing(att.trailing)
	}
}

// emitTrailing emits trailing attached trivia for a token.
func (p *printer) emitTrailing(trailing []token.Token) {
	if len(trailing) == 0 {
		return
	}
	if p.options.Format {
		for _, t := range trailing {
			if t.Kind() == token.Comment {
				p.push(dom.Text(" "))
				if p.ctx.lineToBlock && strings.HasPrefix(t.Text(), "//") {
					// Convert // comment to /* comment */ for inline contexts.
					body := strings.TrimPrefix(strings.TrimRight(t.Text(), " \t"), "//")
					// If the body contains "*/", insert a space to keep
					// it from prematurely terminating the block comment.
					body = strings.ReplaceAll(body, "*/", "* /")
					p.push(dom.Text("/*" + body + " */"))
				} else {
					p.emitComment(t)
				}
			}
		}
	} else {
		p.pending = append(p.pending, trailing...)
	}
}

// emitCommaTrivia emits trailing trivia from a comma token that is not
// itself printed (e.g., commas removed from message literal fields in
// format mode). This ensures comments attached to skipped commas are
// never lost.
func (p *printer) emitCommaTrivia(comma token.Token) {
	if comma.IsZero() {
		return
	}
	att, ok := p.trivia.tokenTrivia(comma.ID())
	if !ok {
		return
	}
	p.emitTrailing(att.trailing)
}

// appendPending buffers trivia tokens for later processing by emitTrivia.
func (p *printer) appendPending(tokens []token.Token) {
	p.pending = append(p.pending, tokens...)
}

// printScopeDecls prints declarations in a scope, computing
// inter-declaration gaps and emitting trivia slots between them.
//
// Detached trivia (scope.slots[i]) stays at scope positions so that
// section-boundary comments do not travel with their declaration when
// the file is sorted in format mode.
//
// Attached trivia (leading on first token, trailing on last) is handled by
// [printer.printDecl] via [printer.printTokenAs].
func (p *printer) printScopeDecls(
	trivia detachedTrivia,
	decls seq.Indexer[ast.DeclAny],
	scope scopeKind,
) {
	for i := range decls.Len() {
		p.emitTriviaSlot(trivia, i)
		gap := p.declGap(decls, trivia, i, scope)
		p.printDecl(decls.At(i), gap)
	}
	p.emitRemainingTrivia(trivia, decls.Len())
}

// declGap computes the gap before declaration i in a scope.
//
// For the first declaration (i==0), it handles flushing detached leading
// comments (copyright headers at file level, or comments between '{'
// and the first member at body level). For subsequent declarations, it
// determines whether a blank line or regular newline separates them.
func (p *printer) declGap(
	decls seq.Indexer[ast.DeclAny],
	trivia detachedTrivia,
	i int,
	scope scopeKind,
) gapStyle {
	if i == 0 {
		return p.firstDeclGap(scope)
	}

	if !p.options.Format {
		return gapNewline
	}

	// File level: blank line between different sections (syntax ->
	// package, imports -> options, etc.). For body declarations,
	// preserve blank lines from the source rather than always adding them.
	if scope == scopeFile {
		prev, curr := rankDecl(decls.At(i-1)), rankDecl(decls.At(i))
		if prev != curr {
			return gapBlankline
		}
		if curr == rankBody && trivia.hasBlankBefore(i) {
			return gapBlankline
		}
		return gapNewline
	}

	// Body level: preserve blank lines from the original source.
	if trivia.hasBlankBefore(i) {
		return gapBlankline
	}
	return gapNewline
}

// firstDeclGap computes the gap before the first declaration in a scope.
func (p *printer) firstDeclGap(scope scopeKind) gapStyle {
	if !p.options.Format {
		if scope == scopeFile {
			return gapNone
		}
		return gapNewline
	}

	// We deliberately do not flush p.pending here. The slot trivia
	// already in pending (file-leading comments, or body comments
	// between '{' and the first member) needs to merge with the first
	// declaration's leading trivia in a single emitTrivia pass so that
	// blank-line counts (newlineRun) span the whole boundary; otherwise
	// splitDetached's separation of "last \n on the next token's leading"
	// from the rest of the run would fragment a blank line into two
	// flushes and lose it. The natural gap returned here propagates into
	// printTokenAs, which appends att.leading to pending and then calls
	// emitTrivia, seeing all the trivia together.
	if scope == scopeFile {
		return gapNone
	}
	return gapNewline
}

// emitTriviaSlot appends the detached trivia for slot[i] to pending.
// In format mode, whitespace tokens are filtered via appendPending.
func (p *printer) emitTriviaSlot(trivia detachedTrivia, i int) {
	if i >= len(trivia.slots) {
		return
	}
	p.appendPending(trivia.slots[i])
}

// emitRemainingTrivia emits the remaining detached trivia for slot >= i.
func (p *printer) emitRemainingTrivia(trivia detachedTrivia, i int) {
	for ; i < len(trivia.slots); i++ {
		p.emitTriviaSlot(trivia, i)
	}
}

// emitGap pushes whitespace tags for the given gap style.
func (p *printer) emitGap(gap gapStyle) {
	switch gap {
	case gapSpace:
		p.push(tagSpace)
	case gapNewline:
		p.push(tagNewline)
	case gapSoftline:
		softline(p.push)
	case gapBlankline:
		blankline(p.push)
	case gapInline, gapPreserve, gapPreserveTight:
		// No visual gap. Comment handling is done in emitTrivia.
	}
}

// commentGap returns the appropriate gap for comment separation.
func commentGap(contextGap gapStyle, isLineComment bool, blankRun int) gapStyle {
	if blankRun >= 2 {
		return gapBlankline
	}
	if isLineComment {
		return gapNewline
	}
	return contextGap
}

// emitTrivia flushes pending trivia. In format mode, only comments
// are emitted with canonical spacing; in non-format mode, all tokens
// are concatenated verbatim.
func (p *printer) emitTrivia(gap gapStyle) {
	if !p.options.Format {
		if len(p.pending) > 0 {
			var buf strings.Builder
			for _, tok := range p.pending {
				buf.WriteString(tok.Text())
			}
			p.push(dom.Text(buf.String()))
			p.pending = p.pending[:0]
		}
		return
	}

	afterGap := gapSoftline
	switch gap {
	case gapSpace:
		afterGap = gapSpace
	case gapPreserve, gapPreserveTight:
		// afterGap is determined dynamically per gap point based on
		// whether the source had whitespace.
		afterGap = gapNone
	case gapInline:
		// gapInline is used for punctuation tokens (`;`, `,`) where
		// comments should have a space before the first and no gap after
		// the last, keeping the punctuation on the same line.
		afterGap = gapNone
	}

	firstGap := gap
	if gap == gapInline {
		firstGap = gapSpace
	}

	// inheritGap promotes base to gapSpace when the source had
	// non-newline whitespace in a gapPreserve/gapPreserveTight context.
	inheritGap := func(base gapStyle, hasSpace bool) gapStyle {
		if (gap == gapPreserve || gap == gapPreserveTight) && hasSpace {
			return gapSpace
		}
		return base
	}

	hasComment := false
	prevIsLine := false
	newlineRun := 0
	hasNonNewlineSpace := false
	for _, tok := range p.pending {
		if tok.Kind() == token.Space {
			if tok.Text() == "\n" {
				newlineRun++
			} else {
				hasNonNewlineSpace = true
			}
			continue
		}
		if tok.Kind() != token.Comment {
			continue
		}

		fg := inheritGap(firstGap, hasNonNewlineSpace)
		ag := inheritGap(afterGap, hasNonNewlineSpace)

		// Suppress the inherited space before the first comment when
		// gapPreserveTight is used (right after an open bracket).
		if gap == gapPreserveTight && !hasComment {
			fg = firstGap
		}

		if !hasComment {
			p.emitGap(fg)
		} else {
			p.emitGap(commentGap(ag, prevIsLine, newlineRun))
		}
		hasNonNewlineSpace = false
		newlineRun = 0
		isLine := strings.HasPrefix(tok.Text(), "//")
		p.emitComment(tok)
		hasComment = true
		prevIsLine = isLine
	}
	p.pending = p.pending[:0]

	if hasComment {
		// Use the actual newlineRun from trailing tokens after the last
		// comment. When the source had a blank line (2+ newlines) after
		// the last comment, this preserves it.
		p.emitGap(commentGap(inheritGap(afterGap, hasNonNewlineSpace), prevIsLine, newlineRun))
		return
	}
	p.emitGap(gap)
}

// extractCloseComments checks if a close token (], }) has leading
// comments in its trivia. Returns the comments and the full trivia
// so the caller can suppress the default printToken and emit the
// comments inside an indented block instead.
func (p *printer) extractCloseComments(closeTok token.Token) ([]token.Token, attachedTrivia) {
	if !p.options.Format {
		return nil, attachedTrivia{}
	}
	att, hasTrivia := p.trivia.tokenTrivia(closeTok.ID())
	if !hasTrivia {
		return nil, attachedTrivia{}
	}
	if sliceHasComment(att.leading) {
		return att.leading, att
	}
	return nil, attachedTrivia{}
}

// extractOpenTrailing returns the trailing trivia for a token if it
// contains comments, or nil otherwise. Used to detect trailing comments
// on open brackets that need to be moved inside an indented block.
func (p *printer) extractOpenTrailing(tok token.Token) []token.Token {
	att, ok := p.trivia.tokenTrivia(tok.ID())
	if !ok {
		return nil
	}
	if sliceHasComment(att.trailing) {
		return att.trailing
	}
	return nil
}

// emitCloseTok emits a close token, respecting pre-extracted close
// comments. When close comments were extracted (and emitted inside the
// preceding indent block), the token text is emitted directly with its
// trailing trivia. Otherwise, printToken/printTokenAs handles it normally.
func (p *printer) emitCloseTok(closeTok token.Token, closeText string, closeComments []token.Token, closeAtt attachedTrivia) {
	if len(closeComments) > 0 {
		p.emitGap(gapNewline)
		p.push(dom.Text(closeText))
		p.emitTrailing(closeAtt.trailing)
	} else {
		p.printTokenAs(closeTok, gapNewline, closeText)
	}
}

// scopeHasAttachedComments checks whether any token in a fused scope
// (brackets, braces, parens) has attached comments (leading or trailing).
func (p *printer) scopeHasAttachedComments(fused token.Token) bool {
	if p.trivia == nil {
		return false
	}
	openTok, closeTok := fused.StartEnd()
	// Check open token trailing.
	if att, ok := p.trivia.tokenTrivia(openTok.ID()); ok {
		if sliceHasComment(att.trailing) {
			return true
		}
	}
	// Check close token leading.
	if att, ok := p.trivia.tokenTrivia(closeTok.ID()); ok {
		if sliceHasComment(att.leading) {
			return true
		}
	}
	// Check interior tokens.
	cursor := fused.Children()
	for tok := cursor.NextSkippable(); !tok.IsZero(); tok = cursor.NextSkippable() {
		if tok.Kind().IsSkippable() {
			continue
		}
		if att, ok := p.trivia.tokenTrivia(tok.ID()); ok {
			if sliceHasComment(att.leading) || sliceHasComment(att.trailing) {
				return true
			}
		}
	}
	return false
}

// scopeHasUninlineableLeadingComments reports whether any interior token
// in a fused scope has a leading comment that prevents inline rendering.
//
// A `//` line comment in leading trivia is always uninlineable (it would
// eat the rest of the line). A `/* */` block comment is uninlineable
// only when it is preceded by a newline in the same leading run, since
// the newline indicates the source had the comment on its own line.
// Block comments mid-expression (e.g. `= /* before */ value`) have no
// preceding newline and stay inline cleanly.
func (p *printer) scopeHasUninlineableLeadingComments(fused token.Token) bool {
	if p.trivia == nil {
		return false
	}
	cursor := fused.Children()
	for tok := cursor.NextSkippable(); !tok.IsZero(); tok = cursor.NextSkippable() {
		if tok.Kind().IsSkippable() {
			continue
		}
		att, ok := p.trivia.tokenTrivia(tok.ID())
		if !ok {
			continue
		}
		sawNewline := false
		for _, t := range att.leading {
			if t.Kind() == token.Space {
				if strings.Contains(t.Text(), "\n") {
					sawNewline = true
				}
				continue
			}
			if t.Kind() != token.Comment {
				continue
			}
			if strings.HasPrefix(t.Text(), "//") || sawNewline {
				return true
			}
		}
	}
	return false
}

// semiGap returns the gap to use before a semicolon or comma.
// In format mode, uses gapInline to keep comments on the same line as
// the preceding token. In non-format mode, uses gapNone.
func (p *printer) semiGap() gapStyle {
	if p.options.Format {
		return gapInline
	}
	return gapNone
}

// withIndent runs fn with an indented printer, swapping the sink temporarily.
func (p *printer) withIndent(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Indent(p.indent, func(indentSink dom.Sink) {
		p.push = indentSink
		fn(p)
	}))
	p.push = originalPush
}

// withGroup runs fn with a grouped printer, swapping the sink temporarily.
func (p *printer) withGroup(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Group(p.options.MaxWidth, func(groupSink dom.Sink) {
		p.push = groupSink
		fn(p)
	}))
	p.push = originalPush
}

// emitComment trims trailing whitespace from a comment token and emits
// it, dispatching to emitBlockComment for /* comments.
func (p *printer) emitComment(tok token.Token) {
	text := strings.TrimRight(tok.Text(), " \t")
	if strings.HasPrefix(text, "/*") {
		p.emitBlockComment(text)
	} else {
		p.push(dom.Text(text))
	}
}

// emitBlockComment normalizes and emits a multi-line block comment as
// separate dom.Text calls so that dom.Indent can apply outer indentation
// to each line.
//
// Single-line block comments (e.g., /* foo */) are emitted as-is.
// Multi-line comments where the closing line has content before */ are
// treated as degenerate and emitted verbatim.
//
// The normalization algorithm matches buf format's behavior:
//   - Detect if all non-empty interior lines share a common non-alphanumeric
//     prefix character (e.g., *, =). If so, strip all whitespace and re-add
//     " " before each line (prefix style). If the prefix is *, the closing
//     line becomes " */".
//   - Otherwise (plain style), compute the minimum visual indentation of
//     non-empty interior lines, unindent by that amount, then add "   "
//     (3 spaces) before each line.
func (p *printer) emitBlockComment(text string) {
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		p.push(dom.Text(text))
		return
	}

	// Determine whether the last line is a standalone closing "*/" or
	// contains content before it (e.g., "  buzz */").
	lastTrimmed := strings.TrimLeft(lines[len(lines)-1], " \t")
	standaloneClose := strings.HasPrefix(lastTrimmed, "*/") && strings.TrimRight(lastTrimmed, " \t") == "*/"

	// Compute minimum indent and detect prefix character across all
	// lines after the first (interior + closing).
	minIndent := -1
	var prefix byte
	prefixSet := false
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " \t")
		if trimmed == "" || trimmed == "*/" {
			continue
		}

		indent := computeVisualIndent(lines[i])
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}

		ch := trimmed[0]
		if isCommentPrefix(ch) {
			if !prefixSet {
				prefix = ch
				prefixSet = true
			} else if ch != prefix {
				prefix = 0
			}
		} else {
			prefix = 0
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	// Emit first line.
	p.push(dom.Text(strings.TrimRight(lines[0], " \t")))

	// Process lines 1..N-1 (interior lines, and closing if not standalone).
	end := len(lines) - 1
	if !standaloneClose {
		// Closing line has content; process it like any other line.
		end = len(lines)
	}

	// pendingNewlines accumulates the number of '\n's that should
	// precede the next content push. We coalesce them into a single
	// wider break tag so blank interior lines survive dom's adjacent-
	// break merge (which keeps the wider of two equal-width breaks but
	// drops one when they are equal).
	pendingNewlines := 0
	for i := 1; i < end; i++ {
		pendingNewlines++
		trimmed := strings.TrimLeft(lines[i], " \t")

		if trimmed == "" {
			continue
		}

		var content string
		if prefix != 0 {
			content = " " + strings.TrimRight(trimmed, " \t")
		} else {
			line := unindent(lines[i], minIndent)
			line = strings.TrimRight(line, " \t")
			if line == "" {
				continue
			}
			content = "   " + line
		}

		p.push(dom.Text(strings.Repeat("\n", pendingNewlines)))
		p.push(dom.Text(content))
		pendingNewlines = 0
	}

	// Emit standalone closing line if applicable.
	if standaloneClose {
		pendingNewlines++
		p.push(dom.Text(strings.Repeat("\n", pendingNewlines)))
		if prefix == '*' {
			p.push(dom.Text(" */"))
		} else {
			p.push(dom.Text("*/"))
		}
	}
}

// isCommentPrefix reports whether ch is a valid block comment line prefix
// character. Only printable ASCII punctuation qualifies (e.g., *, =, -, #).
// Letters, digits, whitespace, and control characters are not valid prefixes.
func isCommentPrefix(ch byte) bool {
	return ch >= '!' && ch <= '~' &&
		!((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9'))
}

// computeVisualIndent returns the visual indentation of a line, expanding
// tabs to 8-column tab stops (matching buf format behavior).
func computeVisualIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 8 - (indent % 8)
		default:
			return indent
		}
	}
	return indent
}

// unindent removes up to n visual columns of leading whitespace from line,
// expanding tabs to 8-column tab stops.
func unindent(line string, n int) string {
	pos := 0
	for i, r := range line {
		if pos == n {
			return line[i:]
		}
		if pos > n {
			// Tab stop overshot; add back spaces to compensate.
			return strings.Repeat(" ", pos-n) + line[i:]
		}
		switch r {
		case ' ':
			pos++
		case '\t':
			pos += 8 - (pos % 8)
		default:
			return line[i:]
		}
	}
	return ""
}
