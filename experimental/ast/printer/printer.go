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
	gapSoftline  // gapSoftline inserts a space if the group is flat, or a newline if the group is broken
	gapBlankline // gapBlankline inserts two newline characters
	gapInline    // gapInline acts like gapNone when there are no comments; when there are comments, it spaces around them
	gapGlue      // gapGlue is like gapNone but comments are glued with no surrounding spaces (for path separators)
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
		trivia := buildTriviaIndex(file.Stream())
		p := &printer{
			trivia:  trivia,
			push:    push,
			options: options,
		}
		p.printFile(file)
	})
}

// Print renders a single declaration to protobuf source text.
//
// For printing entire files, use [PrintFile] instead.
func Print(options Options, decl ast.DeclAny) string {
	options = options.withDefaults()
	return dom.Render(options.domOptions(), func(push dom.Sink) {
		p := &printer{
			push:    push,
			options: options,
		}
		p.printDecl(decl, gapNewline)
		p.emitTrivia(gapNone)
	})
}

// printer tracks state for printing AST nodes with fidelity.
type printer struct {
	options Options
	trivia  *triviaIndex
	pending []token.Token
	push    dom.Sink

	// convertLineToBlock, when true, causes emitTrailing to convert
	// line comments (// ...) to block comments (/* ... */). This
	// only affects trailing trivia, not leading. It is set in
	// contexts where inline tokens follow without a newline break
	// (paths, compact options, option values before `;`) so that
	// a trailing // comment doesn't eat the next token.
	convertLineToBlock bool
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
	}
	if hasTrivia {
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
				text := strings.TrimRight(t.Text(), " \t")
				switch {
				case strings.HasPrefix(text, "/*"):
					p.emitBlockComment(text)
				case p.convertLineToBlock:
					// Convert // comment to /* comment */ for inline contexts.
					body := strings.TrimPrefix(text, "//")
					p.push(dom.Text("/*" + body + " */"))
				default:
					p.push(dom.Text(text))
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

// appendPending buffers trivia tokens, filtering non-newline whitespace
// in format mode.
func (p *printer) appendPending(tokens []token.Token) {
	if p.options.Format {
		for _, tok := range tokens {
			if tok.Kind() == token.Space && tok.Text() != "\n" {
				continue
			}
			p.pending = append(p.pending, tok)
		}
		return
	}
	p.pending = append(p.pending, tokens...)
}

// printScopeDecls prints declarations in a scope, computing
// inter-declaration gaps and emitting trivia slots between them.
func (p *printer) printScopeDecls(trivia detachedTrivia, decls seq.Indexer[ast.DeclAny], scope scopeKind) {
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
		return p.firstDeclGap(trivia, scope)
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

// firstDeclGap computes the gap before the first declaration in a scope,
// flushing any detached leading comments when necessary.
func (p *printer) firstDeclGap(trivia detachedTrivia, scope scopeKind) gapStyle {
	if !p.options.Format {
		if scope == scopeFile {
			return gapNone
		}
		return gapNewline
	}

	// Detect leading comments that need to be flushed separately from
	// the first declaration. At file level, these are copyright headers
	// or other file-leading comments. At body level, these are comments
	// between '{' and the first member that were separated by a blank
	// line in the source.
	flush := false
	if scope == scopeFile {
		flush = p.pendingHasComments()
	} else {
		flush = trivia.hasBlankBefore(0)
	}

	if flush {
		beforeComments := gapNone
		if scope == scopeBody {
			beforeComments = gapNewline
		}
		p.emitTrivia(beforeComments)
		return gapNewline
	}

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
		p.push(dom.Text(" "))
	case gapNewline:
		p.push(dom.Text("\n"))
	case gapSoftline:
		p.push(dom.TextIf(dom.Flat, " "))
		p.push(dom.TextIf(dom.Broken, "\n"))
	case gapBlankline:
		p.push(dom.Text("\n"))
		p.push(dom.Text("\n"))
	case gapInline:
		// gapInline emits nothing when there are no comments.
		// Comment handling is done in emitTrivia.
	case gapGlue:
		// gapGlue emits nothing (like gapNone).
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
	case gapGlue:
		// When comments are present in a glued context, the
		// post-comment gap needs a space (e.g., /* comment */ stream).
		// Without comments, gapGlue still emits nothing.
		afterGap = gapSpace
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

	hasComment := false
	prevIsLine := false
	newlineRun := 0
	for _, tok := range p.pending {
		if tok.Kind() == token.Space {
			if tok.Text() == "\n" {
				newlineRun++
			}
			continue
		}
		if tok.Kind() != token.Comment {
			continue
		}
		if !hasComment {
			p.emitGap(firstGap)
		} else {
			p.emitGap(commentGap(afterGap, prevIsLine, newlineRun))
		}
		newlineRun = 0
		text := strings.TrimRight(tok.Text(), " \t")
		isLine := strings.HasPrefix(text, "//")
		if strings.HasPrefix(text, "/*") {
			p.emitBlockComment(text)
		} else {
			p.push(dom.Text(text))
		}
		hasComment = true
		prevIsLine = isLine
	}
	p.pending = p.pending[:0]

	if hasComment {
		// Use the actual newlineRun from trailing tokens after the last
		// comment. When the source had a blank line (2+ newlines) after
		// the last comment, this preserves it.
		p.emitGap(commentGap(afterGap, prevIsLine, newlineRun))
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

// semiGap returns the gap to use before a semicolon or comma.
// In format mode, uses gapInline to keep comments on the same line as
// the preceding token. In non-format mode, uses gapNone.
func (p *printer) semiGap() gapStyle {
	if p.options.Format {
		return gapInline
	}
	return gapNone
}

// withLineToBlock runs fn with convertLineToBlock set to the given value,
// restoring the previous value when fn returns. This controls whether
// trailing // comments are converted to /* */ to prevent them from
// eating following tokens like `;` or `]`.
func (p *printer) withLineToBlock(enabled bool, fn func()) {
	saved := p.convertLineToBlock
	p.convertLineToBlock = enabled
	defer func() { p.convertLineToBlock = saved }()
	fn()
}

// withIndent runs fn with an indented printer, swapping the sink temporarily.
func (p *printer) withIndent(fn func(p *printer)) {
	originalPush := p.push
	p.push(dom.Indent(strings.Repeat(" ", p.options.TabstopWidth), func(indentSink dom.Sink) {
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

	for i := 1; i < end; i++ {
		p.push(dom.Text("\n"))
		trimmed := strings.TrimLeft(lines[i], " \t")

		if trimmed == "" {
			continue
		}

		if prefix != 0 {
			trimmed = strings.TrimRight(trimmed, " \t")
			p.push(dom.Text(" " + trimmed))
		} else {
			line := unindent(lines[i], minIndent)
			line = strings.TrimRight(line, " \t")
			if line == "" {
				continue
			}
			p.push(dom.Text("   " + line))
		}
	}

	// Emit standalone closing line if applicable.
	if standaloneClose {
		p.push(dom.Text("\n"))
		if prefix == '*' {
			p.push(dom.Text(" */"))
		} else {
			p.push(dom.Text("*/"))
		}
	}
}

// isCommentPrefix reports whether ch is a valid block comment line prefix.
// Letters and digits are not valid prefixes.
func isCommentPrefix(ch byte) bool {
	return !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9'))
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
	return 0
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
