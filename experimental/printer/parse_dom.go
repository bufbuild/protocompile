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
	"github.com/bufbuild/protocompile/experimental/printer/dom"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TODO list
//
// - finish all decl types
// - debug various decls/defs
// - a bunch of performance optimizations
//   - refactor splitChunk behaviour/callback?
//   - improve performance with cursors
// - a bunch of docs
// - do a naming sanity check with Ed/Miguel

// splitSetter sets the splitKind of the chunk using a cursor based on the surrounding tokens.
type splitSetter func(*token.Cursor) (dom.SplitKind, bool)

// spaceSetter sets a space after the given token when true is returned.
type spaceSetter func(token.Token) bool

type bodyDomsSetter func(indented bool) *dom.Doms

func fileToDom(file ast.File, format bool) []*dom.Doms {
	var doms []*dom.Doms
	seq.Values(file.Decls())(func(decl ast.DeclAny) bool {
		d := declDoms(decl.Context().Stream(), decl, format, 0, true, false)
		doms = append(doms, d)
		return true
	})
	return doms
}

func declDoms(
	stream *token.Stream,
	decl ast.DeclAny,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Doms {
	doms := dom.NewDoms()
	tokens := getTokensForSpan(stream, decl.Span())
	// TODO: what does it mean to have no tokens?
	if tokens == nil {
		return nil
	}
	doms.Insert(prefix(tokens[0], format))
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration/what exactly causes empty decls
	case ast.DeclKindSyntax:
		syntax := decl.AsSyntax()
		doms.Insert(dom.NewDom([]*dom.Chunk{single(
			tokens,
			format,
			func(t token.Token) bool {
				if t.ID() == syntax.Semicolon().ID() || spanOverlappingSpan(t.Span(), syntax.Value().Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				// Not entirely convinced by this.
				return dom.SplitKindDouble, false
			},
			dom.SplitKindDouble,
			splitWithParent,
			indentLevel,
			indented,
		)}))
	case ast.DeclKindPackage:
		pkg := decl.AsPackage()
		doms.Insert(dom.NewDom([]*dom.Chunk{single(
			tokens,
			format,
			func(t token.Token) bool {
				if t.ID() == pkg.Semicolon().ID() || spanOverlappingSpan(t.Span(), pkg.Path().Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				return dom.SplitKindDouble, false
			},
			dom.SplitKindDouble,
			splitWithParent,
			indentLevel,
			indented,
		)}))
	case ast.DeclKindImport:
		imprt := decl.AsImport()
		doms.Insert(dom.NewDom([]*dom.Chunk{single(
			tokens,
			format,
			func(t token.Token) bool {
				if t.ID() == imprt.Semicolon().ID() || spanOverlappingSpan(t.Span(), imprt.ImportPath().Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, _, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				// TODO: there is some consideration for the last import in an import block should
				// always be a double. Something to look at after we have a working prototype.
				if double {
					return dom.SplitKindDouble, false
				}
				return dom.SplitKindHard, false
			},
			dom.SplitKindHard,
			splitWithParent,
			indentLevel,
			indented,
		)}))
	case ast.DeclKindDef:
		doms.Insert(defDom(stream, tokens, decl.AsDef(), format, indentLevel, indented, splitWithParent))
	case ast.DeclKindBody:
		return bodyDoms(stream, decl.AsBody(), format, indentLevel, indented)
	case ast.DeclKindRange:
		// TODO: implement
	default:
		panic("unknown DeclKind in File")
	}
	return doms
}

func defDom(
	stream *token.Stream,
	tokens []token.Token,
	decl ast.DeclDef,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Dom {
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:
		message := decl.AsMessage()
		return compound(
			tokens,
			format,
			message.Body.Span(),
			func(indentedBody bool) *dom.Doms {
				return bodyDoms(stream, message.Body, format, indentLevel+1, indentedBody)
			},
			message.Body.Braces(),
			func(t token.Token) bool {
				return t.ID() != message.Body.Braces().ID()
			},
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindEnum:
		enum := decl.AsEnum()
		return compound(
			tokens,
			format,
			enum.Body.Span(),
			func(indentedBody bool) *dom.Doms {
				return bodyDoms(stream, enum.Body, format, indentLevel+1, indentedBody)
			},
			enum.Body.Braces(),
			func(t token.Token) bool {
				return t.ID() != enum.Body.Braces().ID()
			},
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindService:
		service := decl.AsService()
		return compound(
			tokens,
			format,
			service.Body.Span(),
			func(indentedBody bool) *dom.Doms {
				return bodyDoms(stream, service.Body, format, indentLevel+1, indentedBody)
			},
			service.Body.Braces(),
			func(t token.Token) bool {
				return t.ID() != service.Body.Braces().ID()
			},
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindExtend:
		// TODO: implement
	case ast.DefKindField:
		field := decl.AsField()
		return fieldDom(
			stream,
			tokens,
			field.Span(),
			field.Tag.Span(),
			field.Options,
			field.Semicolon,
			format,
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindOneof:
		oneof := decl.AsOneof()
		return compound(
			tokens,
			format,
			oneof.Body.Span(),
			func(indentedBody bool) *dom.Doms {
				return bodyDoms(stream, oneof.Body, format, indentLevel+1, indentedBody)
			},
			oneof.Body.Braces(),
			func(t token.Token) bool {
				return t.ID() != oneof.Body.Braces().ID()
			},
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindGroup:
		// TODO: implement
	case ast.DefKindEnumValue:
		enumValue := decl.AsEnumValue()
		return fieldDom(
			stream,
			tokens,
			enumValue.Span(),
			enumValue.Tag.Span(),
			enumValue.Options,
			enumValue.Semicolon,
			format,
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindMethod:
		method := decl.AsMethod()
		return compound(
			tokens,
			format,
			method.Body.Span(),
			func(indentedBody bool) *dom.Doms {
				return bodyDoms(stream, method.Body, format, indentLevel+1, indentedBody)
			},
			method.Body.Braces(),
			func(t token.Token) bool {
				if t.ID() == method.Body.Braces().ID() {
					return false
				}
				if spanOverlappingSpan(t.Span(), method.Signature.Inputs().Span()) {
					_, end := method.Signature.Inputs().Brackets().StartEnd()
					return t.ID() == end.ID()
				}
				if spanOverlappingSpan(t.Span(), method.Signature.Outputs().Span()) {
					_, end := method.Signature.Outputs().Brackets().StartEnd()
					return t.ID() == end.ID()
				}
				return true
			},
			indentLevel,
			indented,
			splitWithParent,
		)
	case ast.DefKindOption:
		// TODO: handle semicolon
		return optionDom(tokens, decl.AsOption().Option, format, indentLevel, indented, splitWithParent)
	default:
		panic("unknown DefKind in File")
	}
	return nil
}

func fieldDom(
	stream *token.Stream,
	tokens []token.Token,
	fieldSpan report.Span,
	fieldTagSpan report.Span,
	options ast.CompactOptions,
	semicolon token.Token,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Dom {
	if options.IsZero() {
		return dom.NewDom([]*dom.Chunk{single(
			tokens,
			format,
			func(t token.Token) bool {
				if t.ID() == semicolon.ID() || spanOverlappingSpan(t.Span(), fieldTagSpan) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				if double {
					return dom.SplitKindDouble, false
				}
				if single {
					return dom.SplitKindHard, false
				}
				return dom.SplitKindSoft, false
			},
			dom.SplitKindHard,
			splitWithParent,
			indentLevel,
			indented,
		)})
	}
	return compound(
		tokens,
		format,
		options.Span(),
		func(indentedBody bool) *dom.Doms {
			return optionsDoms(stream, options, format, indentLevel+1, indentedBody)
		},
		options.Brackets(),
		func(t token.Token) bool {
			return t.ID() != options.Brackets().ID()
		},
		indentLevel,
		indented,
		splitWithParent,
	)
}

func optionDom(
	tokens []token.Token,
	option ast.Option,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Dom {
	return dom.NewDom([]*dom.Chunk{single(
		tokens,
		format,
		func(t token.Token) bool {
			if spanWithinSpan(t.Span(), option.Path.Span()) || spanOverlappingSpan(t.Span(), option.Value.Span()) {
				return false
			}
			return true
		},
		func(cursor *token.Cursor) (dom.SplitKind, bool) {
			trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
			if trailingComment {
				return dom.SplitKindNever, true
			}
			if double {
				return dom.SplitKindDouble, false
			}
			if single {
				return dom.SplitKindHard, false
			}
			return dom.SplitKindSoft, false
		},
		dom.SplitKindHard,
		splitWithParent,
		indentLevel,
		indented,
	)})
}

func bodyDoms(
	stream *token.Stream,
	body ast.DeclBody,
	format bool,
	indentLevel uint32,
	indented bool,
) *dom.Doms {
	doms := dom.NewDoms()
	seq.Values(body.Decls())(func(d ast.DeclAny) bool {
		declDoms := declDoms(stream, d, format, indentLevel, indented, true)
		doms.Insert(*declDoms...)
		if doms.Last() != nil {
			switch doms.Last().SplitKind() {
			case dom.SplitKindSoft, dom.SplitKindNever:
				indented = false
			case dom.SplitKindHard, dom.SplitKindDouble:
				indented = true
			}
		}
		return true
	})
	return doms
}

func optionsDoms(
	stream *token.Stream,
	options ast.CompactOptions,
	format bool,
	indentLevel uint32,
	indented bool,
) *dom.Doms {
	doms := dom.NewDoms()
	seq.Values(options.Entries())(func(o ast.Option) bool {
		tokens := getTokensForSpan(stream, o.Span())
		if len(tokens) == 0 {
			return true
		}
		doms.Insert(prefix(tokens[0], format))
		doms.Insert(optionDom(tokens, o, format, indentLevel, indented, true))
		if doms.Last() != nil {
			switch doms.Last().SplitKind() {
			case dom.SplitKindSoft, dom.SplitKindNever:
				indented = false
			case dom.SplitKindHard, dom.SplitKindDouble:
				indented = true
			}
		}
		return true
	})
	return doms
}

func prefix(start token.Token, format bool) *dom.Dom {
	cursor := token.NewCursorAt(start)
	t := cursor.PrevSkippable()
	for t.Kind().IsSkippable() {
		if cursor.PeekPrevSkippable().IsZero() {
			break
		}
		t = cursor.PrevSkippable()
	}
	t = cursor.NextSkippable()
	var chunks []*dom.Chunk
	for t.ID() != start.ID() {
		switch t.Kind() {
		case token.Space:
			// Only create a chunk for spaces if formatting is not applied.
			// Otherwise extraneous whitepsace is dropped and whitespace.
			if !format {
				chunks = append(chunks, dom.NewChunk(t.Text()))
			}
		case token.Comment:
			chunks = append(chunks, single(
				[]token.Token{t},
				format,
				func(_ token.Token) bool {
					return false
				},
				func(cursor *token.Cursor) (dom.SplitKind, bool) {
					_, single, double := trailingCommentSingleDoubleFound(cursor)
					if double {
						return dom.SplitKindDouble, false
					}
					if single {
						return dom.SplitKindHard, false
					}
					return dom.SplitKindSoft, false
				},
				dom.SplitKindHard,
				false,
				0,
				false,
			))
		}
		t = cursor.NextSkippable()
	}
	return dom.NewDom(chunks)
}

func compound(
	tokens []token.Token,
	format bool,
	bodySpan report.Span,
	bodyDomsSetter bodyDomsSetter,
	braces token.Token,
	topLineSpacer spaceSetter,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Dom {
	topLineTokens, bodyTokens := splitTokens(tokens, bodySpan)
	topLineChunk := single(
		topLineTokens,
		format,
		topLineSpacer,
		func(cursor *token.Cursor) (dom.SplitKind, bool) {
			if len(bodyTokens) > 0 {
				// If there is a body, then we check the first body token forward.
				cursor = token.NewCursorAt(bodyTokens[0])
			}
			trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
			if trailingComment {
				return dom.SplitKindNever, true
			}
			if double {
				return dom.SplitKindDouble, false
			}
			if single {
				return dom.SplitKindHard, false
			}
			return dom.SplitKindSoft, false
		},
		dom.SplitKindHard,
		false, // HMMM
		indentLevel,
		indented,
	)
	switch topLineChunk.SplitKind() {
	case dom.SplitKindSoft, dom.SplitKindNever:
		indented = false
	case dom.SplitKindHard, dom.SplitKindDouble:
		indented = true
	}
	bodyDoms := bodyDomsSetter(indented)
	var closingBrace *dom.Chunk
	var extras []token.Token
	if !braces.IsZero() {
		// The prefix consists of all non-skippable tokens in the bodyTokens, backwards from
		// the braces.
		_, end := braces.StartEnd()
		var prefixChunks []*dom.Chunk
		if len(bodyTokens) > 0 {
			for i := len(bodyTokens) - 1; i >= 0; i-- {
				t := bodyTokens[i]
				// Collect tokens after the closing brace and handle them.
				if t.Span().Start >= end.Span().End {
					if t.ID() == end.ID() {
						continue
					}
					extras = append(extras, t)
					continue
				}
				if !t.Kind().IsSkippable() {
					break
				}
				// This logic is the same as prefix, we might be able to refactor.
				switch t.Kind() {
				case token.Space:
					if !format {
						prefixChunks = append(prefixChunks, dom.NewChunk(t.Text()))
					}
				case token.Comment:
					prefixChunks = append(prefixChunks, single(
						[]token.Token{t},
						format,
						func(_ token.Token) bool {
							return false
						},
						func(cursor *token.Cursor) (dom.SplitKind, bool) {
							_, single, double := trailingCommentSingleDoubleFound(cursor)
							if double {
								return dom.SplitKindDouble, false
							}
							if single {
								return dom.SplitKindHard, false
							}
							return dom.SplitKindSoft, false
						},
						dom.SplitKindHard,
						false,
						0,
						false,
					))
				}
			}
			if len(prefixChunks) > 0 {
				bodyDoms.Insert(dom.NewDom(prefixChunks))
			}
		}
		if bodyDoms.Last() != nil {
			switch bodyDoms.Last().SplitKind() {
			case dom.SplitKindSoft, dom.SplitKindNever:
				indented = false
			case dom.SplitKindHard, dom.SplitKindDouble:
				indented = true
			}
		}
		closingBrace = single(
			[]token.Token{end},
			format,
			func(_ token.Token) bool {
				return false
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				if double {
					return dom.SplitKindDouble, false
				}
				if single {
					return dom.SplitKindHard, false
				}
				return dom.SplitKindSoft, false
			},
			// TODO: not exactly, but eh.
			dom.SplitKindDouble,
			splitWithParent, // hmm.
			indentLevel,
			indented,
		)
		if splitWithParent {
			closingBrace.SetIndentWhenSplitWithParent(false)
		}
	}
	topLineChunk.SetChildren(bodyDoms)
	chunks := []*dom.Chunk{topLineChunk}
	if closingBrace != nil {
		chunks = append(chunks, closingBrace)
	}
	// TODO: handle all remaining tokens.
	if len(extras) > 0 {
		chunks = append(chunks, prefix(extras[0], format).Chunks()...)
		chunks = append(chunks, single(
			extras,
			format,
			func(_ token.Token) bool {
				return false
			},
			func(cursor *token.Cursor) (dom.SplitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return dom.SplitKindNever, true
				}
				if double {
					return dom.SplitKindDouble, false
				}
				if single {
					return dom.SplitKindHard, false
				}
				return dom.SplitKindSoft, false
			},
			dom.SplitKindHard,
			splitWithParent,
			indentLevel,
			indented,
		))
	}
	return dom.NewDom(chunks)
}

func single(
	tokens []token.Token,
	format bool,
	spacer spaceSetter,
	splitter splitSetter,
	splitKindIfSplit dom.SplitKind,
	splitWithParent bool,
	indentLevel uint32,
	indented bool,
) *dom.Chunk {
	var text string
	if format {
		for _, t := range tokens {
			if t.Kind() == token.Space {
				continue
			}
			// In formatted mode, whitespace is handled by split logic, trim extraneous whitespaces.
			text += strings.TrimSpace(t.Text())
			if spacer(t) {
				text += " "
			}
		}
	} else {
		for _, t := range tokens {
			text += t.Text()
		}
	}
	chunk := dom.NewChunk(text)
	if format {
		chunk.SetIndent(indentLevel)
		chunk.SetIndented(indented)
		splitKind, spaceWhenUnsplit := splitter(token.NewCursorAt(tokens[len(tokens)-1]))
		chunk.SetSplitKind(splitKind)
		chunk.SetSpaceWhenUnsplit(spaceWhenUnsplit)
		chunk.SetSplitKindIfSplit(splitKindIfSplit)
		chunk.SetSplitWithParent(splitWithParent)
		if splitWithParent {
			chunk.SetIndentWhenSplitWithParent(splitWithParent)
		}
	}
	return chunk
}

func splitTokens(tokens []token.Token, splitSpan report.Span) ([]token.Token, []token.Token) {
	var front, back []token.Token
	for _, t := range tokens {
		if t.Span().Start <= splitSpan.Start {
			// If this is specifically the body brace token, then we only want the opening brace
			// Otherwise we need to add all tokens (e.g. both the open and closing braces of a method
			// input/output).
			if t.Span().Start == splitSpan.Start {
				if !t.IsLeaf() {
					start, end := t.StartEnd()
					if t.ID() == end.ID() {
						continue
					}
					t = start
				}
			}
			front = append(front, t)
			continue
		}
		back = append(back, t)
	}
	return front, back
}

func getTokensForSpan(stream *token.Stream, span report.Span) []token.Token {
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		if spanOverlappingSpan(t.Span(), span) {
			tokens = append(tokens, t)
		}
		// We are past the end, so no need to continue
		if t.Span().Start > span.End {
			return false
		}
		return true
	})
	return tokens
}

// Check that the given span is within the bounds of another span, inclusive.
func spanOverlappingSpan(span, base report.Span) bool {
	return span.Start >= base.Start && span.End <= base.End
}

// Check that the given span is within the bounds of another span, exclusive of the end.
func spanWithinSpan(span, base report.Span) bool {
	return span.Start >= base.Start && span.End < base.End
}

// TODO: docs
func trailingCommentSingleDoubleFound(cursor *token.Cursor) (bool, bool, bool) {
	// Look ahead until the next unskippable token.
	// If there are comments without a new line in between among the unskippable tokens,
	// then we return a soft split.
	t := cursor.NextSkippable()
	var commentFound bool
	var singleFound bool
	var doubleFound bool
	for {
		switch t.Kind() {
		case token.Space:
			if strings.Contains(t.Text(), "\n\n") {
				doubleFound = true
			}
			// If the whitespace contains a string anywhere, we can break out and return early.
			if strings.Contains(t.Text(), "\n") {
				singleFound = true
				return commentFound, singleFound, doubleFound
			}
		case token.Comment:
			commentFound = true
			// If the comment ends in a new line, we should still respect that while setting the split.
			if strings.HasSuffix(t.Text(), "\n") {
				singleFound = true
			}
		}
		if cursor.PeekSkippable().IsZero() {
			break
		}
		t = cursor.NextSkippable()
		// No longer a skippable token
		if !t.Kind().IsSkippable() {
			break
		}
	}
	return commentFound, singleFound, doubleFound
}
