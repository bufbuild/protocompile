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
				p.push(dom.Text(t.Text()))
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
func (p *printer) printScopeDecls(trivia detachedTrivia, decls seq.Indexer[ast.DeclAny], firstGap gapStyle) {
	isFileLevel := firstGap == gapNone
	for i := range decls.Len() {
		p.emitTriviaSlot(trivia, i)

		var gap gapStyle
		switch {
		case i == 0 && p.options.Format && p.shouldFlushLeadingComments(isFileLevel, trivia, i):
			flushGap := gapNone
			if !isFileLevel {
				flushGap = gapNewline
			}
			p.emitTrivia(flushGap)
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

func (p *printer) shouldFlushLeadingComments(isFileLevel bool, trivia detachedTrivia, i int) bool {
	if isFileLevel {
		return p.pendingHasComments()
	}
	return i < len(trivia.blankBefore) && trivia.blankBefore[i]
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
		var buf strings.Builder
		for _, tok := range p.pending {
			buf.WriteString(tok.Text())
		}
		p.push(dom.Text(buf.String()))
		p.pending = p.pending[:0]
		return
	}

	afterGap := gapSoftline
	if gap == gapSpace {
		afterGap = gapSpace
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
			p.emitGap(gap)
		} else {
			p.emitGap(commentGap(afterGap, prevIsLine, newlineRun))
		}
		newlineRun = 0
		p.push(dom.Text(tok.Text()))
		hasComment = true
		prevIsLine = strings.HasPrefix(tok.Text(), "//")
	}
	p.pending = p.pending[:0]

	if hasComment {
		p.emitGap(commentGap(afterGap, prevIsLine, 0))
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
