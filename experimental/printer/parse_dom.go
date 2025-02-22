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
// - support new structure with SetOnlyPrintUnformatted
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

type bodyDomSetter func(indented bool) *dom.Dom

func fileToDom(file ast.File, format bool) *dom.Dom {
	d := dom.NewDom()
	seq.Values(file.Decls())(func(decl ast.DeclAny) bool {
		d.Insert(declChunks(decl.Context().Stream(), decl, format, 0, true, false)...)
		return true
	})
	return d
}

func declChunks(
	stream *token.Stream,
	decl ast.DeclAny,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) []*dom.Chunk {
	var chunks []*dom.Chunk
	tokens := getTokensForSpan(stream, decl.Span())
	// TODO: what does it mean to have no tokens?
	if tokens == nil {
		return nil
	}
	chunks = append(chunks, prefix(tokens[0], format)...)
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration/what exactly causes empty decls
	case ast.DeclKindSyntax:
		syntax := decl.AsSyntax()
		chunks = append(chunks, single(
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
			false,
		))
	case ast.DeclKindPackage:
		pkg := decl.AsPackage()
		chunks = append(chunks, single(
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
			false,
		))
	case ast.DeclKindImport:
		imprt := decl.AsImport()
		chunks = append(chunks, single(
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
			false,
		))
	case ast.DeclKindDef:
		chunks = append(chunks, defChunks(stream, tokens, decl.AsDef(), format, indentLevel, indented, splitWithParent)...)
	case ast.DeclKindBody:
		chunks = append(chunks, bodyChunks(stream, decl.AsBody(), format, indentLevel, indented)...)
	case ast.DeclKindRange:
		// TODO: implement
	default:
		panic("unknown DeclKind in File")
	}
	return chunks
}

func defChunks(
	stream *token.Stream,
	tokens []token.Token,
	decl ast.DeclDef,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) []*dom.Chunk {
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:
		message := decl.AsMessage()
		return compound(
			tokens,
			format,
			message.Body.Span(),
			func(indentedBody bool) *dom.Dom {
				d := dom.NewDom()
				d.Insert(bodyChunks(stream, message.Body, format, indentLevel+1, indentedBody)...)
				return d
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
			func(indentedBody bool) *dom.Dom {
				d := dom.NewDom()
				d.Insert(bodyChunks(stream, enum.Body, format, indentLevel+1, indentedBody)...)
				return d
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
			func(indentedBody bool) *dom.Dom {
				d := dom.NewDom()
				d.Insert(bodyChunks(stream, service.Body, format, indentLevel+1, indentedBody)...)
				return d
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
		return fieldChunks(
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
			func(indentedBody bool) *dom.Dom {
				d := dom.NewDom()
				d.Insert(bodyChunks(stream, oneof.Body, format, indentLevel+1, indentedBody)...)
				return d
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
		return fieldChunks(
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
			func(indentedBody bool) *dom.Dom {
				d := dom.NewDom()
				d.Insert(bodyChunks(stream, method.Body, format, indentLevel+1, indentedBody)...)
				return d
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
		return []*dom.Chunk{optionChunk(tokens, decl.AsOption().Option, format, indentLevel, indented, splitWithParent)}
	default:
		panic("unknown DefKind in File")
	}
	return nil
}

func fieldChunks(
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
) []*dom.Chunk {
	if options.IsZero() {
		return []*dom.Chunk{single(
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
			true,
		)}
	}
	return compound(
		tokens,
		format,
		options.Span(),
		func(indentedBody bool) *dom.Dom {
			d := dom.NewDom()
			d.Insert(optionsChunks(stream, options, format, indentLevel+1, indentedBody)...)
			return d
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

func optionChunk(
	tokens []token.Token,
	option ast.Option,
	format bool,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) *dom.Chunk {
	return single(
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
		true,
	)
}

func bodyChunks(
	stream *token.Stream,
	body ast.DeclBody,
	format bool,
	indentLevel uint32,
	indented bool,
) []*dom.Chunk {
	var chunks []*dom.Chunk
	seq.Values(body.Decls())(func(d ast.DeclAny) bool {
		chunks = append(chunks, declChunks(stream, d, format, indentLevel, indented, true)...)
		if len(chunks) > 0 {
			switch chunks[len(chunks)-1].SplitKind() {
			case dom.SplitKindSoft, dom.SplitKindNever:
				indented = false
			case dom.SplitKindHard, dom.SplitKindDouble:
				indented = true
			}
		}
		return true
	})
	return chunks
}

func optionsChunks(
	stream *token.Stream,
	options ast.CompactOptions,
	format bool,
	indentLevel uint32,
	indented bool,
) []*dom.Chunk {
	var chunks []*dom.Chunk
	seq.Values(options.Entries())(func(o ast.Option) bool {
		tokens := getTokensForSpan(stream, o.Span())
		if len(tokens) == 0 {
			return true
		}
		chunks = append(chunks, prefix(tokens[0], format)...)
		chunks = append(chunks, optionChunk(tokens, o, format, indentLevel, indented, true))
		if len(chunks) > 0 {
			switch chunks[len(chunks)-1].SplitKind() {
			case dom.SplitKindSoft, dom.SplitKindNever:
				indented = false
			case dom.SplitKindHard, dom.SplitKindDouble:
				indented = true
			}
		}
		return true
	})
	return chunks
}

func prefix(start token.Token, format bool) []*dom.Chunk {
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
				chunk := dom.NewChunk()
				chunk.SetText(t.Text())
				chunks = append(chunks, chunk)
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
				false,
			))
		}
		t = cursor.NextSkippable()
	}
	return chunks
}

func compound(
	tokens []token.Token,
	format bool,
	bodySpan report.Span,
	bodyDomSetter bodyDomSetter,
	braces token.Token,
	topLineSpacer spaceSetter,
	indentLevel uint32,
	indented bool,
	splitWithParent bool,
) []*dom.Chunk {
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
		false,
		indentLevel,
		indented,
		true,
	)
	switch topLineChunk.SplitKind() {
	case dom.SplitKindSoft, dom.SplitKindNever:
		indented = false
	case dom.SplitKindHard, dom.SplitKindDouble:
		indented = true
	}
	bodyDom := bodyDomSetter(indented)
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
						prefixChunk := dom.NewChunk()
						prefixChunk.SetText(t.Text())
						prefixChunks = append(prefixChunks, prefixChunk)
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
						false,
					))
				}
			}
			bodyDom.Insert(prefixChunks...)
		}
		switch bodyDom.LastSplitKind() {
		case dom.SplitKindSoft, dom.SplitKindNever:
			indented = false
		case dom.SplitKindHard, dom.SplitKindDouble:
			indented = true
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
			splitWithParent,
			indentLevel,
			indented,
			false,
		)
	}
	topLineChunk.SetChild(bodyDom)
	chunks := []*dom.Chunk{topLineChunk}
	if closingBrace != nil {
		chunks = append(chunks, closingBrace)
	}
	// TODO: handle all remaining tokens.
	if len(extras) > 0 {
		chunks = append(chunks, prefix(extras[0], format)...)
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
			false,
		))
	}
	return chunks
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
	indentOnParentSplit bool,
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
	chunk := dom.NewChunk()
	chunk.SetText(text)
	if format {
		chunk.SetIndent(indentLevel)
		chunk.SetIndented(indented)
		splitKind, spaceWhenUnsplit := splitter(token.NewCursorAt(tokens[len(tokens)-1]))
		chunk.SetSplitKind(splitKind)
		chunk.SetSpaceWhenUnsplit(spaceWhenUnsplit)
		chunk.SetSplitKindIfSplit(splitKindIfSplit)
		chunk.SetSplitWithParent(splitWithParent)
		chunk.SetIndentOnParentSplit(indentOnParentSplit)
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
