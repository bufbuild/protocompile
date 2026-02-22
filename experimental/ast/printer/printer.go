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
)

// PrintFile renders an AST file to protobuf source text.
func PrintFile(options Options, file *ast.File) string {
	options = options.withDefaults()
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
	p.printScopeDecls(trivia, decls, gapNone)
	// In format mode, trailing file comments need a newline gap so they
	// don't run into the last declaration's closing token.
	endGap := gapNone
	if p.options.Format {
		endGap = gapNewline
	}
	p.emitTrivia(endGap)
}

// pendingHasComments reports whether the pending trivia buffer contains
// any comment tokens.
func (p *printer) pendingHasComments() bool {
	for _, tok := range p.pending {
		if tok.Kind() == token.Comment {
			return true
		}
	}
	return false
}

// printToken is the standard entry point for printing a semantic token.
// It emits leading attached trivia, the gap, the token text, and trailing
// attached trivia.
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

	// Flush pending trivia and emit the gap, then the token text.
	//
	// In non-format mode, the gap is only a fallback for synthetic tokens
	// (those with no trivia entry). When trivia exists (even if the leading
	// slice is empty), the trivia replaces the gap entirely.
	//
	// In format mode, the gap is always used: whitespace tokens are discarded
	// and the gap provides canonical spacing.
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

	// Emit trailing attached trivia.
	if hasTrivia && len(att.trailing) > 0 {
		if p.options.Format {
			// In format mode, emit trailing comments immediately with a
			// canonical space, keeping them on the same line as the token.
			for _, t := range att.trailing {
				if t.Kind() == token.Comment {
					p.push(dom.Text(" "))
					p.push(dom.Text(t.Text()))
				}
			}
		} else {
			p.pending = append(p.pending, att.trailing...)
		}
	}
}

// appendPending appends trivia tokens to the pending buffer.
// In format mode, non-newline whitespace tokens (spaces) are filtered
// out. Newline tokens are kept so that emitTrivia can detect blank lines
// (consecutive newlines) between comment groups.
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

// printScopeDecls zips trivia slots with declarations, computing
// inter-declaration gaps.
//
// In format mode, a blank line is inserted between declarations that
// belong to different spacing groups (e.g. between imports and options).
// At the file level (firstGap == gapNone), header comments in slot 0
// are flushed with a blank line before the first declaration.
//
// In non-format mode, all gaps are gapNewline (trivia provides the
// actual whitespace; the gap is only a fallback for synthetic tokens).
func (p *printer) printScopeDecls(trivia detachedTrivia, decls seq.Indexer[ast.DeclAny], firstGap gapStyle) {
	isFileLevel := firstGap == gapNone
	for i := range decls.Len() {
		p.emitTriviaSlot(trivia, i)

		var gap gapStyle
		switch {
		case i == 0 && isFileLevel && p.options.Format && p.pendingHasComments():
			// File-level header comments: flush them with no leading
			// gap, then add a newline that combines with emitTrivia's
			// trailing newline to produce one blank line of separation.
			p.emitTrivia(gapNone)
			p.emitGap(gapNewline)
			gap = gapNone
		case i == 0 && !isFileLevel && p.options.Format &&
			i < len(trivia.blankBefore) && trivia.blankBefore[i]:
			// Body-level first declaration with trailing comments on
			// the open brace followed by a blank line. Flush the
			// comments first, then insert a newline gap. Combined
			// with emitTrivia's trailing newline after the comment,
			// this produces a blank line of separation.
			p.emitTrivia(gapNewline)
			p.emitGap(gapNewline)
			gap = gapNone
		case i == 0:
			gap = firstGap
		case p.options.Format && isFileLevel &&
			rankDecl(decls.At(i-1)) != rankDecl(decls.At(i)):
			gap = gapBlankline
		case p.options.Format && isFileLevel &&
			rankDecl(decls.At(i)) == rankBody:
			gap = gapBlankline
		case p.options.Format && !isFileLevel &&
			i < len(trivia.blankBefore) && trivia.blankBefore[i]:
			gap = gapBlankline
		default:
			gap = gapNewline
		}

		p.printDecl(decls.At(i), gap)
	}
	p.emitRemainingTrivia(trivia, decls.Len())
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
	}
}

// emitTrivia flushes the pending trivia buffer.
//
// In non-format mode, all pending tokens are concatenated and emitted as
// a single text tag, preserving original formatting.
//
// In format mode, pending contains comment and newline tokens (non-newline
// whitespace was filtered by appendPending). The gap is emitted before the
// first comment. Between comments, a blank line (gapBlankline) is emitted
// when 2+ consecutive newlines separate them; otherwise a newline is used.
// For inline contexts (gap == gapSpace), block comments use spaces to stay
// on the same line; line comments (//) always force a newline after them.
// When there are no comments, the gap is emitted directly.
func (p *printer) emitTrivia(gap gapStyle) {
	if !p.options.Format {
		var buf strings.Builder
		for _, tok := range p.pending {
			buf.WriteString(tok.Text())
		}
		p.push(dom.Text(buf.String()))
		p.pending = p.pending[:0]
		return
	}

	// For inline comment contexts (gap == gapSpace), keep block comments
	// on the same line. Line comments (//) always require a newline after.
	afterGap := gapNewline
	if gap == gapSpace {
		afterGap = gapSpace
	}

	hasComment := false
	prevIsLine := false
	newlineRun := 0
	for _, tok := range p.pending {
		if tok.Kind() == token.Space {
			// Count consecutive newline tokens.
			if tok.Text() == "\n" {
				newlineRun++
			}
			continue
		}
		if tok.Kind() != token.Comment {
			continue
		}
		if !hasComment {
			p.emitGap(gap)
		} else {
			betweenGap := afterGap
			if prevIsLine {
				betweenGap = gapNewline
			}
			if newlineRun >= 2 {
				betweenGap = gapBlankline
			}
			p.emitGap(betweenGap)
		}
		newlineRun = 0
		p.push(dom.Text(tok.Text()))
		hasComment = true
		prevIsLine = strings.HasPrefix(tok.Text(), "//")
	}
	p.pending = p.pending[:0]

	if hasComment {
		endGap := afterGap
		if prevIsLine {
			endGap = gapNewline
		}
		p.emitGap(endGap)
		return
	}
	p.emitGap(gap)
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
