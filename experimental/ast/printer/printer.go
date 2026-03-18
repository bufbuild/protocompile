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
		}
	}
	p.emitTrivia(endGap)
}

// pendingHasComments reports whether pending contains comments.
func (p *printer) pendingHasComments() bool {
	for _, tok := range p.pending {
		if tok.Kind() == token.Comment {
			return true
		}
	}
	return false
}

// printToken emits a token with its trivia.
func (p *printer) printToken(tok token.Token, gap gapStyle) {
	if tok.IsZero() {
		return
	}
	p.printTokenAs(tok, gap, tok.Text())
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
				text := t.Text()
				if p.options.Format {
					text = strings.TrimRight(text, " \t")
				}
				p.push(dom.Text(text))
			}
		}
	} else {
		p.pending = append(p.pending, trailing...)
	}
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
		// gapGlue is used for path separators where comments should be
		// glued to their tokens with no surrounding spaces.
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
		text := tok.Text()
		if p.options.Format {
			text = strings.TrimRight(text, " \t")
		}
		p.push(dom.Text(text))
		hasComment = true
		prevIsLine = strings.HasPrefix(text, "//")
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
