package printer

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TODO list
//
// - finish all decl types
// - what does it mean to have no tokens for a decl...
// - a bunch of refactors to consolidate logic
// - a bunch of performance optimizations
// - refactor splitChunk behaviour/callback?
// - improve performance with cursors
// - clean-up parsePrefixChunks, getTokensAndCursorForDecl
// - a bunch of docs
// - do a naming sanity check with Ed/Miguel

// TODO: docs
type splitKind int8

const (
	splitKindUnknown = iota
	// splitKindSoft represents a soft split, which means that when the block containing the
	// chunk is evaluated, this chunk may be split to a hard split.
	//
	// If the chunk remains a soft split, spaceWhenUnsplit will add a space after the chunk if
	// true and will add nothing if false. spaceWhenUnsplit is ignored for all other split kinds.
	splitKindSoft
	// splitKindHard represents a hard split, which means the chunk must be followed by a newline.
	splitKindHard
	// splitKindDouble represents a double hard split, which means the chunk must be followed by
	// two newlines.
	splitKindDouble
	// splitKindNever represents a chunk that must never be split. This is treated similar to
	// a soft split, in that it will respect spaceWhenUnsplit.
	splitKindNever
)

// TODO: docs
type splitChunk func(*token.Cursor) (splitKind, bool)

// addSpace is a function that returns true if a space should be added after the given token.
type addSpace func(t token.Token) bool

// chunk represents a line of text with some configurations around indentation and splitting
// (what whitespace should follow, if any).
//
// A chunk is preformatted.
type chunk struct {
	text             string
	indentLevel      uint32
	splitKind        splitKind
	spaceWhenUnsplit bool
	// If this chunk is split, then these other chunks are also split.
	// These are the indices in the block.
	softSplitDeps []int
}

// TODO: block is an ordered slice of chunks. A block represents...
type block struct {
	chunks []chunk
}

func fileToBlocks(file ast.File, applyFormatting bool) []block {
	decls := file.Decls()
	var blocks []block
	for i := 0; i < decls.Len(); i++ {
		decl := decls.At(i)
		blocks = append(blocks, declBlock(decl.Context().Stream(), decl, applyFormatting))
	}
	return blocks
}

func declBlock(stream *token.Stream, decl ast.DeclAny, applyFormatting bool) block {
	return block{chunks: declChunks(stream, decl, applyFormatting, 0)}
}

func declChunks(stream *token.Stream, decl ast.DeclAny, applyFormatting bool, indentLevel uint32) []chunk {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration
	case ast.DeclKindSyntax:
		syntax := decl.AsSyntax()
		tokens, cursor := getTokensAndCursorForDecl(stream, syntax.Span())
		return oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				if t.ID() == syntax.Semicolon().ID() || checkSpanWithin(syntax.Value().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				return splitKindDouble, false
			},
			applyFormatting,
			indentLevel,
		)
	case ast.DeclKindPackage:
		pkg := decl.AsPackage()
		tokens, cursor := getTokensAndCursorForDecl(stream, pkg.Span())
		return oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				if t.ID() == pkg.Semicolon().ID() || checkSpanWithin(pkg.Path().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				return splitKindDouble, false
			},
			applyFormatting,
			indentLevel,
		)
	case ast.DeclKindImport:
		imprt := decl.AsImport()
		tokens, cursor := getTokensAndCursorForDecl(stream, imprt.Span())
		return oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				if t.ID() == imprt.Semicolon().ID() || checkSpanWithin(imprt.ImportPath().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				// TODO: there is some consideration for the last import in an import block should
				// always be a double. Something to look at after we have a working prototype.
				if double {
					return splitKindDouble, false
				}
				return splitKindHard, false
			},
			applyFormatting,
			indentLevel,
		)
	case ast.DeclKindDef:
		return defChunks(stream, decl.AsDef(), applyFormatting, indentLevel)
	case ast.DeclKindBody:
		// TODO: Figure what to do with indentLevel here
		return bodyChunks(stream, decl.AsBody(), applyFormatting, indentLevel, indentLevel)
	case ast.DeclKindRange:
	default:
		panic("ah")
	}
	return []chunk{}
}

func defChunks(stream *token.Stream, decl ast.DeclDef, applyFormatting bool, indentLevel uint32) []chunk {
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:
		var defChunksIndex int
		message := decl.AsMessage()
		tokens, cursor := getTokensAndCursorFromStartToEnd(stream, message.Span(), message.Body.Span())
		chunks := oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				// We don't want to add a space after the brace, if there is one.
				return t.ID() != message.Body.Braces().ID()
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if single {
					return splitKindHard, false
				}
				if double {
					return splitKindDouble, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			indentLevel,
		)
		bodyDeclIndentLevel := indentLevel
		if chunks[len(chunks)-1].splitKind != splitKindSoft {
			// If we are doing any kind of splitting, we want to increment the indent.
			bodyDeclIndentLevel++
		}
		// We also need to create a soft split dep
		// This is technically not a thing if it was not a soft split? TODO: make this more clear
		defChunksIndex = len(chunks) - 1
		bodyChunks := bodyChunks(stream, message.Body, applyFormatting, bodyDeclIndentLevel, indentLevel)
		// If there is a closing brace, we don't need to adjust it.
		var closingBraceChunk chunk
		var closingBrace bool
		if !message.Body.Braces().IsZero() {
			closingBraceChunk = bodyChunks[len(bodyChunks)-1]
			closingBrace = true
			bodyChunks = bodyChunks[:len(bodyChunks)-1]
		}
		// For each body chunk, we must create adjusted softSplitDeps on the other chunks
		if len(bodyChunks) > 1 {
			for i := range bodyChunks {
				softSplitDeps := make([]int, len(bodyChunks))
				for j := range bodyChunks {
					if i == j {
						continue
					}
					softSplitDeps[j] = j + len(chunks)
				}
				bodyChunks[i].softSplitDeps = softSplitDeps
			}
		}
		// TODO: probably should document this in a coherent way
		chunks = append(chunks, bodyChunks...)
		chunks[defChunksIndex].softSplitDeps = append(chunks[defChunksIndex].softSplitDeps, len(chunks)-1)
		if closingBrace {
			chunks = append(chunks, closingBraceChunk)
		}
		return chunks
	case ast.DefKindEnum:
		// TODO: implement
	case ast.DefKindService:
		// TODO: implement
	case ast.DefKindExtend:
		// TODO: implement
	case ast.DefKindField:
		field := decl.AsField()
		// No options to handle, so we just process all the tokens like a single one liner
		if field.Options.IsZero() {
			tokens, cursor := getTokensAndCursorForDecl(stream, field.Span())
			return oneLiner(
				tokens,
				cursor,
				func(t token.Token) bool {
					// We don't want to add a space after the semicolon
					// We also don't want to add a space after the tag
					if t.ID() == field.Semicolon.ID() || checkSpanWithin(field.Tag.Span(), t.Span()) {
						return false
					}
					return true
				},
				func(cursor *token.Cursor) (splitKind, bool) {
					trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
					if trailingComment {
						return splitKindNever, true
					}
					if double {
						return splitKindDouble, false
					}
					if single {
						return splitKindHard, false
					}
					return splitKindSoft, false
				},
				applyFormatting,
				indentLevel,
			)
		}
		// TODO: this looks just like the message body logic, perhaps there is something we can
		// consolidate here.
		//
		// In the case where there are options, we treat everything up to the options declaration
		// as one line.
		tokens, cursor := getTokensAndCursorFromStartToEnd(stream, field.Span(), field.Options.Span())
		chunks := oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				// We don't want to add a space after the bracket, if there is one.
				return t.ID() != field.Options.Brackets().ID()
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if double {
					return splitKindDouble, false
				}
				if single {
					return splitKindHard, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			indentLevel,
		)
		var entriesIndentLevel uint32
		if chunks[len(chunks)-1].splitKind != splitKindSoft {
			entriesIndentLevel = indentLevel + 1
		}
		defChunksIndex := len(chunks) - 1
		optionChunks := optionChunks(stream, field.Options, applyFormatting, entriesIndentLevel)
		// If there is a closing brace, we don't need to adjust it
		var closingBracketChunk chunk
		var closingBracket bool
		if !field.Options.Brackets().IsZero() {
			closingBracketChunk = optionChunks[len(optionChunks)-1]
			closingBracket = true
			optionChunks = optionChunks[:len(optionChunks)-1]
		}
		// For each option chunk, we must create adjusted softSplitDeps on the other chunks
		if len(optionChunks) > 1 {
			for i := range optionChunks {
				softSplitDeps := make([]int, len(optionChunks))
				for j := range optionChunks {
					if i == j {
						continue
					}
					softSplitDeps[j] = j + len(chunks)
				}
				optionChunks[i].softSplitDeps = softSplitDeps
			}
		}
		chunks = append(chunks, optionChunks...)
		chunks[defChunksIndex].softSplitDeps = append(chunks[defChunksIndex].softSplitDeps, len(chunks)-1)
		if closingBracket {
			chunks = append(chunks, closingBracketChunk)
		}
		// Handle the semicolon
		// TODO: there must be a better way
		_, cursor = getTokensAndCursorForDecl(stream, field.Span())
		chunks = append(chunks, oneLiner(
			[]token.Token{field.Semicolon},
			cursor,
			func(t token.Token) bool {
				// No spaces
				return false
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if double {
					return splitKindDouble, false
				}
				if single {
					return splitKindHard, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			0, // No indents for the semicolon
		)...)
		return chunks
	case ast.DefKindOneof:
		// TODO: implement
	case ast.DefKindGroup:
		// TODO: implement
	case ast.DefKindEnumValue:
		// TODO: implement
	case ast.DefKindMethod:
		// TODO: implement
	default:
		// This should never happen.
		panic("ah")
	}
	return nil
}

func bodyChunks(
	stream *token.Stream,
	decl ast.DeclBody,
	applyFormatting bool,
	bodyDeclIndentLevel uint32,
	closingBraceIndentLevel uint32,
) []chunk {
	var chunks []chunk
	seq.Values(decl.Decls())(func(d ast.DeclAny) bool {
		chunks = append(chunks, declChunks(stream, d, applyFormatting, bodyDeclIndentLevel)...)
		return true
	})
	// TODO: figure out what are the edges of this
	// We are already handling the opening brace, so we just want to handle the closing brace here
	if !decl.Braces().IsZero() {
		_, end := decl.Braces().StartEnd()
		chunks = append(chunks, oneLiner(
			[]token.Token{end},
			// TODO: We use `decl.Braces()` instead of end because the closing brace Token is not in the token
			// stream...
			token.NewCursorAt(decl.Braces()),
			func(t token.Token) bool {
				// No spaces
				return false
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if double {
					return splitKindDouble, false
				}
				if single {
					return splitKindHard, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			closingBraceIndentLevel,
		)...)
	}
	return chunks
}

func optionChunks(
	stream *token.Stream,
	options ast.CompactOptions,
	applyFormatting bool,
	entriesIndentLevel uint32,
) []chunk {
	var chunks []chunk
	seq.Values(options.Entries())(func(o ast.Option) bool {
		tokens, cursor := getTokensAndCursorForOption(stream, o.Span())
		chunks = append(chunks, oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				// No spaces within path
				// TODO: we want a space after the last path element
				if checkSpanWithin(o.Path.Span(), t.Span()) {
					return false
				}
				if checkSpanWithin(o.Value.Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if double {
					return splitKindDouble, false
				}
				if single {
					return splitKindHard, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			entriesIndentLevel,
		)...)
		return true
	})
	if !options.Brackets().IsZero() {
		var closingBracketIndentLevel uint32
		if chunks[len(chunks)-1].splitKind != splitKindSoft {
			// TODO: check out of bounds
			closingBracketIndentLevel = entriesIndentLevel - 1
		}
		_, end := options.Brackets().StartEnd()
		chunks = append(chunks, oneLiner(
			[]token.Token{end},
			// TODO: We use options.Brackets() instead of end because the closing bracket Token is
			// not in the token stream...
			token.NewCursorAt(options.Brackets()),
			func(t token.Token) bool {
				// No spaces
				return false
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, single, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindNever, true
				}
				if double {
					return splitKindDouble, false
				}
				if single {
					return splitKindHard, false
				}
				return splitKindSoft, false
			},
			applyFormatting,
			closingBracketIndentLevel,
		)...)
	}
	return chunks
}

// TODO: docs
// maybe rename to single? it also includes prefix tokens though... so i don't know
func oneLiner(
	tokens []token.Token,
	cursor *token.Cursor,
	spacer addSpace,
	splitter splitChunk,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	// TODO: figure out what to do in this case/what even does this case mean?
	if len(tokens) == 0 {
		return nil
	}
	chunks := parsePrefixChunks(tokens[0], applyFormatting)
	var text string
	if applyFormatting {
		for _, t := range tokens {
			// If we are applying formatting, we skip user-defined whitespace and format our own
			if t.Kind() == token.Space {
				continue
			}
			// Add the text
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
	splitKind, spaceWhenUnsplit := splitter(cursor)
	chunks = append(chunks, chunk{
		text:             text,
		splitKind:        splitKind,
		spaceWhenUnsplit: spaceWhenUnsplit,
		indentLevel:      indentLevel,
	})
	return chunks
}

func parsePrefixChunks(until token.Token, applyFormatting bool) []chunk {
	cursor := token.NewCursorAt(until)
	t := cursor.PrevSkippable()
	for t.Kind().IsSkippable() {
		if cursor.PeekPrevSkippable().IsZero() {
			break
		}
		t = cursor.PrevSkippable()
	}
	var chunks []chunk
	t = cursor.NextSkippable()
	for t.ID() != until.ID() {
		switch t.Kind() {
		case token.Space:
			// Only create a chunk for spaces if formatting is not applied.
			// Otherwise, extraneous whitespace is dropped when formatting, so
			// no chunk is added.
			if !applyFormatting {
				chunks = append(chunks, chunk{
					text:      t.Text(),
					splitKind: splitKindSoft,
				})
			}
		case token.Comment:
			var splitKind splitKind
			_, single, _ := trailingCommentSingleDoubleFound(cursor)
			if single {
				splitKind = splitKindHard
			} else {
				splitKind = splitKindSoft
			}
			chunks = append(chunks, chunk{
				text:             t.Text(),
				splitKind:        splitKind,
				spaceWhenUnsplit: true, // Always want to provide a space after an unsplit comment
			})
		case token.Unrecognized:
			// TODO: figure out what to do with unrecognized tokens.
		}
		if cursor.PeekSkippable().IsZero() {
			break
		}
		t = cursor.NextSkippable()
	}
	return chunks
}

func getTokensAndCursorForDecl(stream *token.Stream, span report.Span) ([]token.Token, *token.Cursor) {
	tokens, cursor := getTokensAndCursorForOption(stream, span)
	// TODO: docs. For everything else we do.
	return tokens[:len(tokens)-1], cursor
}

func getTokensAndCursorForOption(stream *token.Stream, span report.Span) ([]token.Token, *token.Cursor) {
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		// If the token is within the declartion range, then add it to tokens, otherwise skip
		if checkSpanWithin(span, t.Span()) {
			tokens = append(tokens, t)
		}
		// We are past the end, so no need to continue
		if t.Span().Start > span.End {
			return false
		}
		return true
	})
	if tokens == nil {
		return nil, nil
	}
	// TODO: docs. For options, we don't cut off the last one
	return tokens, token.NewCursorAt(tokens[len(tokens)-1])
}

// get all the tokens from the start to the end (inclusive) and the cursort starting at the end.
func getTokensAndCursorFromStartToEnd(stream *token.Stream, start, end report.Span) ([]token.Token, *token.Cursor) {
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		if checkSpanWithin(start, t.Span()) || checkSpanWithin(end, t.Span()) {
			tokens = append(tokens, t)
		}
		if t.Span().Start > end.Start {
			return false
		}
		return true
	})
	if tokens == nil {
		return nil, nil
	}
	// TODO: docs
	return tokens[:len(tokens)-1], token.NewCursorAt(tokens[len(tokens)-1])
}

// TODO: rename/clean-up
// TODO: is just checking the starts enough?
func checkSpanWithin(have, want report.Span) bool {
	return want.Start >= have.Start && want.Start <= have.End
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
	for !cursor.Done() || t.Kind().IsSkippable() {
		switch t.Kind() {
		case token.Space:
			if strings.Contains(t.Text(), "\n\n") {
				doubleFound = true
			}
			// If the whitepsace contains a string anywhere, we can break out and return early.
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
	}
	return commentFound, singleFound, doubleFound
}

// TODO: improve this explanation, lol.
// Calculates the splits for the chunks in the block. If the chunks in the block exceed the
// line limit, then chunks that are softSplits are split.
func (b block) calculateSplits(lineLimit int) {
	var cost int
	var outermostSplittableChunk int
	var outermostSplittableChunkSet bool
	// The cost is actually the cost of a single contiguous line of text, not just from the chunk.
	for i, c := range b.chunks {
		// Add the length to the cost
		if c.splitKind == splitKindSoft && !outermostSplittableChunkSet {
			outermostSplittableChunk = i
			outermostSplittableChunkSet = true
		}
		// Reset the cost for each hard split chunk
		if c.splitKind != splitKindSoft {
			cost = 0
		}
		cost += len(c.text)
		// Check if cost has exceeded line limit, if so, we break early
		if cost > lineLimit {
			break
		}
	}
	if cost > lineLimit {
		// No more splits are available, return as is.
		if !outermostSplittableChunkSet {
			return
		}
		b.hardSplitChunk(outermostSplittableChunk)
		b.calculateSplits(lineLimit)
	}
}

func (b block) hardSplitChunk(chunkIndex int) {
	b.chunks[chunkIndex].splitKind = splitKindHard
	for _, dep := range b.chunks[chunkIndex].softSplitDeps {
		// Already split this chunk, we can return
		if b.chunks[dep].splitKind == splitKindHard {
			continue
		}
		b.chunks[dep].splitKind = splitKindHard
		b.chunks[dep].indentLevel++
		b.hardSplitChunk(dep)
	}
}
