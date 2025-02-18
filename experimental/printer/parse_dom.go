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
// - rewrite compound to be better and refactor accordingly
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

func fileToDom(file ast.File, applyFormatting bool) []*dom.Doms {
	var doms []*dom.Doms
	seq.Values(file.Decls())(func(decl ast.DeclAny) bool {
		d := declDoms(decl.Context().Stream(), decl, applyFormatting, 0, true)
		doms = append(doms, d)
		return true
	})
	return doms
}

func declDoms(stream *token.Stream, decl ast.DeclAny, applyFormatting bool, indentLevel uint32, indented bool) *dom.Doms {
	doms := dom.NewDoms()
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration/what exactly causes empty decls
	case ast.DeclKindSyntax:
		syntax := decl.AsSyntax()
		tokens := getTokensForSpan(stream, syntax.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(dom.NewDom([]*dom.Chunk{single(
				tokens,
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
					return dom.SplitKindDouble, false
				},
				applyFormatting,
				indentLevel,
				indented,
			)}))
		}
	case ast.DeclKindPackage:
		pkg := decl.AsPackage()
		tokens := getTokensForSpan(stream, pkg.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(dom.NewDom([]*dom.Chunk{single(
				tokens,
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
				applyFormatting,
				indentLevel,
				indented,
			)}))
		}
	case ast.DeclKindImport:
		imprt := decl.AsImport()
		tokens := getTokensForSpan(stream, imprt.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(dom.NewDom([]*dom.Chunk{single(
				tokens,
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
				applyFormatting,
				indentLevel,
				indented,
			)}))
		}
	case ast.DeclKindDef:
		return defDoms(stream, decl.AsDef(), applyFormatting, indentLevel, indented)
	case ast.DeclKindBody:
		return bodyDoms(stream, decl.AsBody(), applyFormatting, indentLevel, indented)
	case ast.DeclKindRange:
		// TODO: implement
	default:
		panic("unknown DeclKind in File")
	}
	return doms
}

func defDoms(stream *token.Stream, decl ast.DeclDef, applyFormatting bool, indentLevel uint32, indented bool) *dom.Doms {
	doms := dom.NewDoms()
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:
		message := decl.AsMessage()
		tokens := getTokensForCompoundBody(stream, message.Span(), message.Body.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(compound(
				stream,
				tokens,
				message.Body,
				func(t token.Token) bool {
					return t.ID() != message.Body.Braces().ID()
				},
				applyFormatting,
				indentLevel,
				indented,
			))
		}
	case ast.DefKindEnum:
		enum := decl.AsEnum()
		tokens := getTokensForCompoundBody(stream, enum.Span(), enum.Body.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(compound(
				stream,
				tokens,
				enum.Body,
				func(t token.Token) bool {
					return t.ID() != enum.Body.Braces().ID()
				},
				applyFormatting,
				indentLevel,
				indented,
			))
		}
	case ast.DefKindService:
		service := decl.AsService()
		tokens := getTokensForCompoundBody(stream, service.Span(), service.Body.Span())
		// TODO: what does it mean to have no tokens?
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(compound(
				stream,
				tokens,
				service.Body,
				func(t token.Token) bool {
					return t.ID() != service.Body.Braces().ID()
				},
				applyFormatting,
				indentLevel,
				indented,
			))
		}
	case ast.DefKindExtend:
		// TODO: implement
	case ast.DefKindField:
		field := decl.AsField()
		return fieldDoms(
			stream,
			field.Span(),
			field.Tag.Span(),
			field.Semicolon,
			field.Options,
			applyFormatting,
			indentLevel,
			indented,
		)
	case ast.DefKindOneof:
		oneof := decl.AsOneof()
		tokens := getTokensForCompoundBody(stream, oneof.Span(), oneof.Body.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(compound(
				stream,
				tokens,
				oneof.Body,
				func(t token.Token) bool {
					return t.ID() != oneof.Body.Braces().ID()
				},
				applyFormatting,
				indentLevel,
				indented,
			))
		}
	case ast.DefKindGroup:
		// TODO: implement
	case ast.DefKindEnumValue:
		enumValue := decl.AsEnumValue()
		return fieldDoms(
			stream,
			enumValue.Span(),
			enumValue.Tag.Span(),
			enumValue.Semicolon,
			enumValue.Options,
			applyFormatting,
			indentLevel,
			indented,
		)
	case ast.DefKindMethod:
		method := decl.AsMethod()
		tokens := getTokensForCompoundBody(stream, method.Span(), method.Body.Span())
		if tokens != nil {
			doms.Insert(prefix(tokens[0], applyFormatting))
			doms.Insert(compound(
				stream,
				tokens,
				method.Body,

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
				applyFormatting,
				indentLevel,
				indented,
			))
		}
	case ast.DefKindOption:
		// TODO: handle semicolon
		return optionDoms(stream, decl.AsOption().Option, applyFormatting, indentLevel, indented)
	default:
		panic("unknown DefKind in File")
	}
	return doms
}

func bodyDoms(stream *token.Stream, body ast.DeclBody, applyFormatting bool, indentLevel uint32, indented bool) *dom.Doms {
	doms := dom.NewDoms()
	seq.Values(body.Decls())(func(d ast.DeclAny) bool {
		doms.Insert(declDoms(stream, d, applyFormatting, indentLevel, indented).Contents()...)
		if len(doms.Contents()) > 0 {
			switch doms.Contents()[len(doms.Contents())-1].LastSplitKind() {
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
	var chunks []*dom.Chunk
	for t.ID() != start.ID() {
		switch t.Kind() {
		case token.Space:
			// Only create a chunk for spaces if formatting is not applied.
			// Otherwise extraneous whitepsace is dropped and whitespace.
			if !format {
				chunks = append(chunks, dom.NewChunk(
					t.Text(),
					// indent, splitKind, spaceWhenUnsplit do not matter since in this case, we are not formatting
					0,
					false,
					dom.SplitKindNever,
					false,
				))
			}
		case token.Comment:
			chunks = append(chunks, single(
				[]token.Token{t},
				func(t token.Token) bool {
					return false
				},
				func(c *token.Cursor) (dom.SplitKind, bool) {
					// TODO
					return dom.SplitKindSoft, true
				},
				format,
				0,     // TODO: figure out what we should do with this
				false, // TODO: figure out what we should do with this... i think it's never indented...
			))
		}
		t = cursor.NextSkippable()
	}
	return dom.NewDom(chunks)
}

// TODO: we are iterating through the body multiple times super inefficiently here. improve.
func compound(
	stream *token.Stream,
	tokens []token.Token,
	body ast.DeclBody,
	topLineSpacer spaceSetter,
	applyFormatting bool,
	indentLevel uint32,
	indented bool,
) *dom.Dom {
	cursor := token.NewCursorAt(tokens[len(tokens)-1])
	if body.Decls().Len() > 0 {
		var firstSpan report.Span
		var firstToken token.Token
		// TODO: is there a lighter way to do this?
		seq.Values(body.Decls())(func(d ast.DeclAny) bool {
			firstSpan = d.Span()
			return false
		})
		stream.All()(func(t token.Token) bool {
			if spanOverlappingSpan(t.Span(), firstSpan) {
				firstToken = t
				return false
			}
			return true
		})
		cursor = token.NewCursorAt(firstToken)
	}
	topLineChunk := single(
		tokens,
		topLineSpacer,
		func(_ *token.Cursor) (dom.SplitKind, bool) {
			// TODO: Use the cursor here -- this is a shitty pattern, refactor.
			if body.Decls().Len() == 0 {
				// In the case where there are no declarations in the body, we want to check the
				// span in-between the open and close brackets and see if there are any trailing
				// comments and/or new lines.
				start, end := body.Braces().StartEnd()
				textWithin := strings.TrimPrefix(strings.TrimSuffix(body.Braces().Span().Text(), end.Text()), start.Text())
				if len(strings.Fields(textWithin)) > 0 {
					// TODO: this isn't exactly correct, we should still check if there should be splits
					// within, however, we are already priced into all the contents of this? Maybe?
					// Assume that anything here is a comment, and we never split
					return dom.SplitKindNever, true
				}
				if strings.Contains(textWithin, "\n\n") {
					return dom.SplitKindDouble, false
				}
				if strings.Contains(textWithin, "\n") {
					return dom.SplitKindHard, false
				}
			} else {
				// Walk back until the open brace and check for new lines, trailing comments, etc.
				t := cursor.PrevSkippable()
				var commentFound bool
				var singleFound bool
				var doubleFound bool
				for t.ID() != tokens[len(tokens)-1].ID() {
					switch t.Kind() {
					case token.Space:
						if strings.Contains(t.Text(), "\n\n") {
							doubleFound = true
						}
						if strings.Contains(t.Text(), "\n") {
							singleFound = true
						}
					case token.Comment:
						commentFound = true
					}
					if cursor.PeekPrevSkippable().IsZero() {
						break
					}
					t = cursor.PrevSkippable()
				}
				if commentFound {
					return dom.SplitKindNever, true
				}
				if doubleFound {
					return dom.SplitKindDouble, false
				}
				if singleFound {
					return dom.SplitKindHard, false
				}
			}
			return dom.SplitKindSoft, false
		},
		applyFormatting,
		indentLevel,
		indented,
	)
	switch topLineChunk.SplitKind() {
	case dom.SplitKindSoft, dom.SplitKindNever:
		indented = false
	case dom.SplitKindHard, dom.SplitKindDouble:
		indented = true
	}
	bodyDoms := bodyDoms(stream, body, applyFormatting, indentLevel+1, indented)
	var closingBrace *dom.Chunk
	if !body.Braces().IsZero() {
		_, end := body.Braces().StartEnd()
		// To collect the prefixes for the closing brace, we must check between the last decl
		// and the closing brace.
		// These doms get added to the bodyDoms.
		if body.Decls().Len() > 0 {
			var prefixChunks []*dom.Chunk
			var lastSpan report.Span
			var lastToken token.Token
			seq.Values(body.Decls())(func(d ast.DeclAny) bool {
				lastSpan = d.Span()
				return true
			})
			stream.All()(func(t token.Token) bool {
				if spanOverlappingSpan(t.Span(), lastSpan) {
					lastToken = t
					return false
				}
				return true
			})
			cursor = token.NewCursorAt(lastToken)
			t := cursor.NextSkippable()
			for t.ID() != end.ID() {
				switch t.Kind() {
				case token.Space:
					if !applyFormatting {
						prefixChunks = append(prefixChunks, dom.NewChunk(
							t.Text(),
							// indent, splitKind, spaceWhenUnsplit do not matter since in this case, we are not formatting
							0,
							false,
							dom.SplitKindNever,
							false,
						))
					}
				case token.Comment:
					prefixChunks = append(prefixChunks, single(
						[]token.Token{t},
						func(t token.Token) bool {
							return false
						},
						func(c *token.Cursor) (dom.SplitKind, bool) {
							// TODO
							return dom.SplitKindSoft, true
						},
						applyFormatting,
						0,
						false,
					))
				}
				if cursor.PeekSkippable().IsZero() {
					break
				}
				t = cursor.NextSkippable()
			}
			if len(prefixChunks) > 0 {
				bodyDoms.Insert(dom.NewDom(prefixChunks))
			}
		}
		var splitKind dom.SplitKind
		var spaceWhenUnsplit bool
		if applyFormatting {
			trailingComment, single, double := trailingCommentSingleDoubleFound(token.NewCursorAt(end))
			if trailingComment {
				splitKind = dom.SplitKindNever
				spaceWhenUnsplit = true
			} else if double {
				splitKind = dom.SplitKindDouble
			} else if single {
				splitKind = dom.SplitKindHard
			} else {
				splitKind = dom.SplitKindSoft
			}
			if len(bodyDoms.Contents()) > 0 {
				switch bodyDoms.Contents()[len(bodyDoms.Contents())-1].LastSplitKind() {
				case dom.SplitKindSoft, dom.SplitKindNever:
					indented = false
				case dom.SplitKindHard, dom.SplitKindDouble:
					indented = true
				}
			}
		}
		closingBrace = dom.NewChunk(
			end.Text(),
			indentLevel,
			indented, // TODO: check indent level
			splitKind,
			spaceWhenUnsplit,
		)
	}
	topLineChunk.SetChildren(bodyDoms)
	chunks := []*dom.Chunk{topLineChunk}
	if closingBrace != nil {
		chunks = append(chunks, closingBrace)
	}
	return dom.NewDom(chunks)
}

func optionsDoms(stream *token.Stream, options ast.CompactOptions, applyFormatting bool, indentLevel uint32, indented bool) *dom.Doms {
	doms := dom.NewDoms()
	seq.Values(options.Entries())(func(o ast.Option) bool {
		doms.Insert(optionDoms(stream, o, applyFormatting, indentLevel, indented).Contents()...)
		if len(doms.Contents()) > 0 {
			switch doms.Contents()[len(doms.Contents())-1].LastSplitKind() {
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

func optionDoms(stream *token.Stream, option ast.Option, applyFormatting bool, indentLevel uint32, indented bool) *dom.Doms {
	doms := dom.NewDoms()
	tokens := getTokensForSpan(stream, option.Span())
	if tokens == nil {
		return nil
	}
	doms.Insert(prefix(tokens[0], applyFormatting))
	doms.Insert(dom.NewDom([]*dom.Chunk{single(
		tokens,
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
		applyFormatting,
		indentLevel,
		indented,
	)}))
	return doms
}

func fieldDoms(
	stream *token.Stream,
	fieldSpan report.Span,
	fieldTagSpan report.Span,
	semicolon token.Token,
	options ast.CompactOptions,
	applyFormatting bool,
	indentLevel uint32,
	indented bool,
) *dom.Doms {
	doms := dom.NewDoms()
	if options.IsZero() {
		tokens := getTokensForSpan(stream, fieldSpan)
		if tokens == nil {
			return nil
		}
		doms.Insert(prefix(tokens[0], applyFormatting))
		doms.Insert(dom.NewDom([]*dom.Chunk{single(
			tokens,
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
			applyFormatting,
			indentLevel,
			indented,
		)}))
	} else {
		tokens := getTokensForCompoundBody(stream, fieldSpan, options.Span())
		if tokens == nil {
			return nil
		}
		doms.Insert(prefix(tokens[0], applyFormatting))
		cursor := token.NewCursorAt(tokens[len(tokens)-1])
		if options.Entries().Len() > 0 {
			var firstSpan report.Span
			var firstToken token.Token
			seq.Values(options.Entries())(func(o ast.Option) bool {
				firstSpan = o.Span()
				return false
			})
			stream.All()(func(t token.Token) bool {
				if spanOverlappingSpan(t.Span(), firstSpan) {
					firstToken = t
					return false
				}
				return true
			})
			cursor = token.NewCursorAt(firstToken)
		}
		topLineChunk := single(
			tokens,
			func(t token.Token) bool {
				return t.ID() != options.Brackets().ID()
			},
			func(_ *token.Cursor) (dom.SplitKind, bool) {
				if options.Entries().Len() == 0 {
					start, end := options.Brackets().StartEnd()
					textWithin := strings.TrimPrefix(strings.TrimSuffix(options.Brackets().Span().Text(), end.Text()), start.Text())
					if len(strings.Fields(textWithin)) > 0 {
						// TODO: same as compound, this isn't quite right. Fix later
						return dom.SplitKindNever, true
					}
					if strings.Contains(textWithin, "\n\n") {
						return dom.SplitKindDouble, false
					}
					if strings.Contains(textWithin, "\n") {
						return dom.SplitKindHard, false
					}
				} else {
					t := cursor.PrevSkippable()
					var commentFound bool
					var singleFound bool
					var doubleFound bool
					for t.ID() != tokens[len(tokens)-1].ID() {
						switch t.Kind() {
						case token.Space:
							if strings.Contains(t.Text(), "\n\n") {
								doubleFound = true
							}
							if strings.Contains(t.Text(), "\n") {
								singleFound = true
							}
						case token.Comment:
							commentFound = true
						}
						if cursor.PeekPrevSkippable().IsZero() {
							break
						}
						t = cursor.PrevSkippable()
					}
					if commentFound {
						return dom.SplitKindNever, true
					}
					if doubleFound {
						return dom.SplitKindDouble, false
					}
					if singleFound {
						return dom.SplitKindHard, false
					}
				}
				return dom.SplitKindSoft, false
			},
			applyFormatting,
			indentLevel,
			indented,
		)
		optionsIndentLevel := indentLevel
		if topLineChunk.SplitKind() != dom.SplitKindSoft || topLineChunk.SplitKind() != dom.SplitKindNever {
			optionsIndentLevel++
			indented = true
		}
		optionsDoms := optionsDoms(stream, options, applyFormatting, optionsIndentLevel, indented)
		var closingBracket *dom.Chunk
		if !options.Brackets().IsZero() {
			_, end := options.Brackets().StartEnd()
			if options.Entries().Len() > 0 {
				var prefixChunks []*dom.Chunk
				var lastSpan report.Span
				var lastToken token.Token
				seq.Values(options.Entries())(func(o ast.Option) bool {
					lastSpan = o.Span()
					return true
				})
				stream.All()(func(t token.Token) bool {
					if spanOverlappingSpan(t.Span(), lastSpan) {
						lastToken = t
					}
					return true
				})
				cursor = token.NewCursorAt(lastToken)
				t := cursor.NextSkippable()
				for t.ID() != end.ID() {
					switch t.Kind() {
					case token.Space:
						if !applyFormatting {
							prefixChunks = append(prefixChunks, dom.NewChunk(
								t.Text(),
								// indent, splitKind, spaceWhenUnsplit do not matter since in this case, we are not formatting
								0,
								false,
								dom.SplitKindNever,
								false,
							))
						}
					case token.Comment:
						prefixChunks = append(prefixChunks, single(
							[]token.Token{t},
							func(t token.Token) bool {
								return false
							},
							func(c *token.Cursor) (dom.SplitKind, bool) {
								return dom.SplitKindSoft, true
							},
							applyFormatting,
							0,
							false,
						))
					}
					if cursor.PeekSkippable().IsZero() {
						break
					}
					t = cursor.NextSkippable()
				}
				if len(prefixChunks) > 0 {
					optionsDoms.Insert(dom.NewDom(prefixChunks))
				}
			}
			var splitKind dom.SplitKind
			var spaceWhenUnsplit bool
			if applyFormatting {
				trailingComment, single, double := trailingCommentSingleDoubleFound(token.NewCursorAt(end))
				if trailingComment {
					splitKind = dom.SplitKindNever
					spaceWhenUnsplit = true
				} else if double {
					splitKind = dom.SplitKindDouble
				} else if single {
					splitKind = dom.SplitKindHard
				} else {
					splitKind = dom.SplitKindSoft
				}
				if len(optionsDoms.Contents()) > 0 {
					switch optionsDoms.Contents()[len(optionsDoms.Contents())-1].LastSplitKind() {
					case dom.SplitKindSoft, dom.SplitKindNever:
						indented = false
					case dom.SplitKindHard, dom.SplitKindDouble:
						indented = true
					}
				}
			}
			closingBracket = dom.NewChunk(
				end.Text(),
				indentLevel,
				indented,
				splitKind,
				spaceWhenUnsplit,
			)
		}
		topLineChunk.SetChildren(optionsDoms)
		chunks := []*dom.Chunk{topLineChunk}
		if closingBracket != nil {
			chunks = append(chunks, closingBracket)
			if closingBracket.SplitKind() != dom.SplitKindSoft || closingBracket.SplitKind() != dom.SplitKindNever {
				indented = true
			}
		}
		// Handle the semicolon
		chunks[len(chunks)-1].Children().Insert(prefix(semicolon, applyFormatting))
		chunks = append(chunks, single(
			[]token.Token{semicolon},
			func(t token.Token) bool {
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
			applyFormatting,
			indentLevel,
			indented,
		))
		doms.Insert(dom.NewDom(chunks))
	}
	return doms
}

func single(
	tokens []token.Token,
	spacer spaceSetter,
	splitter splitSetter,
	format bool,
	indentLevel uint32,
	indented bool,
) *dom.Chunk {
	var text string
	if format {
		for _, t := range tokens {
			if t.Kind() == token.Space {
				continue
			}
			text += t.Text()
			if spacer(t) {
				text += " "
			}
		}
	} else {
		for _, t := range tokens {
			text += t.Text()
		}
	}
	var splitKind dom.SplitKind
	var spaceWhenUnsplit bool
	if format {
		splitKind, spaceWhenUnsplit = splitter(token.NewCursorAt(tokens[len(tokens)-1]))
	}
	return dom.NewChunk(
		text,
		indentLevel,
		indented,
		splitKind,
		spaceWhenUnsplit,
	)
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

func getTokensForCompoundBody(stream *token.Stream, fullSpan, bodySpan report.Span) []token.Token {
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		if spanWithinRange(t.Span(), fullSpan.Start, bodySpan.Start) {
			tokens = append(tokens, t)
		}
		// No need to continue if we've moved past the body span
		if t.Span().Start > bodySpan.Start {
			return false
		}
		return true
	})
	// TODO: what does it mean to have no tokens?
	return tokens
}

// Check that the given span starts within the bounds of range, inclusive.
func spanWithinRange(span report.Span, start, end int) bool {
	return span.Start >= start && span.Start <= end
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
