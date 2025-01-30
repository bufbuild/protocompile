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
// - add indent handling
// - block level split logic (split deps)
// - what does it mean to have no tokens for a decl...
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

// chunk represents a line of text with some configurations around indentation and splitting
// (what whitespace should follow, if any).
//
// A chunk is preformatted.
type chunk struct {
	text             string
	nestingLevel     uint32
	splitKind        splitKind
	spaceWhenUnsplit bool
}

// TODO: block is an ordered slice of chunks. A block represents...
type block struct {
	chunks []chunk
	// TODO: improve this explanation
	//
	// All chunks here have splitKind = soft
	// If I am splitting chunk indx = key, then i must also split chunk indx = value
	// map[int]int:
	// {
	//    0: 3,
	//    1: 2,
	// }
	softSplitDeps map[int]int
}

// addSpace is a function that returns true if a space should be added after the given token.
type addSpace func(t token.Token) bool

func fileToBlocks(file ast.File, applyFormatting bool) []block {
	decls := file.Decls()
	var blocks []block
	for i := 0; i < decls.Len(); i++ {
		decl := decls.At(i)
		blocks = append(blocks, declBlock(decl.Context().Stream(), decl, applyFormatting))
	}
	return blocks
}

// TODO: block-level split logic
func declBlock(stream *token.Stream, decl ast.DeclAny, applyFormatting bool) block {
	return block{chunks: declChunks(stream, decl, applyFormatting)}
}

// TODO: account for indents
func declChunks(stream *token.Stream, decl ast.DeclAny, applyFormatting bool) []chunk {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration
	case ast.DeclKindSyntax:
		syntax := decl.AsSyntax()
		return oneLiner(
			stream,
			syntax.Span(),
			func(t token.Token) bool {
				if t.ID() == syntax.Semicolon().ID() || checkSpanWithin(syntax.Value().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindSoft, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				return splitKindDouble, false
			},
			applyFormatting,
		)
	case ast.DeclKindPackage:
		pkg := decl.AsPackage()
		return oneLiner(
			stream,
			pkg.Span(),
			func(t token.Token) bool {
				if t.ID() == pkg.Semicolon().ID() || checkSpanWithin(pkg.Path().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, _ := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					// If a comment was found before a new line or the next skippable token was found,
					// we set this to a soft split with a space following.
					return splitKindSoft, true
				}
				// Otherwise, by default we always want a double split after a syntax declaration
				return splitKindDouble, false
			},
			applyFormatting,
		)
	case ast.DeclKindImport:
		imprt := decl.AsImport()
		return oneLiner(
			stream,
			imprt.Span(),
			func(t token.Token) bool {
				if t.ID() == imprt.Semicolon().ID() || checkSpanWithin(imprt.ImportPath().Span(), t.Span()) {
					return false
				}
				return true
			},
			func(cursor *token.Cursor) (splitKind, bool) {
				trailingComment, _, double := trailingCommentSingleDoubleFound(cursor)
				if trailingComment {
					return splitKindSoft, true
				}
				// TODO: there is some consideration for the last import in an import block should
				// always be a double. Something to look at after we have a working prototype.
				if double {
					return splitKindDouble, false
				}
				return splitKindHard, false
			},
			applyFormatting,
		)
	case ast.DeclKindDef:
		return defChunks(stream, decl.AsDef(), applyFormatting)
	case ast.DeclKindBody:
		// TODO: figure out how to handle this
	case ast.DeclKindRange:
	default:
		panic("ah")
	}
	return []chunk{}
}

func defChunks(stream *token.Stream, decl ast.DeclDef, applyFormatting bool) []chunk {
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:
		message := decl.AsMessage()
		// First handle everything up to the message body
		// TODO: We should figure out what to do if there are no braces
		tokens, _ := getTokensAndCursorFromStartToEnd(stream, message.Span(), message.Body.Braces().Span())
		// TODO: figure out what to do in this case/what even does this case mean?
		if len(tokens) == 0 {
		}
		chunks := parsePrefixChunks(tokens[0], applyFormatting)
		var msgDefText string
		if applyFormatting {
			for _, t := range tokens {
				if t.Kind() == token.Space {
					continue
				}
				msgDefText += t.Text()
				if t.Kind() == token.Punct {
					continue
				}
				msgDefText += " "
			}
		} else {
			for _, t := range tokens {
				msgDefText += t.Text()
			}
		}
		// TODO: implement splitting logic
		chunks = append(chunks, chunk{
			text:             msgDefText,
			splitKind:        splitKindHard,
			spaceWhenUnsplit: false,
		})
		// Then process the message body
		seq.Values(message.Body.Decls())(func(d ast.DeclAny) bool {
			chunks = append(chunks, declChunks(stream, d, applyFormatting)...)
			return true
		})
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
			return oneLiner(
				stream,
				field.Span(),
				func(t token.Token) bool {
					if t.ID() == field.Semicolon.ID() {
						return false
					}
					return true
				},
				func(_ *token.Cursor) (splitKind, bool) {
					// TODO: implement
					return splitKindHard, false
				},
				applyFormatting,
			)
		}
		// TODO figure out how to handle options
		return nil
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

// TODO: docs
// maybe rename to single?
func oneLiner(
	stream *token.Stream,
	span report.Span,
	spacer addSpace,
	splitter splitChunk,
	applyFormatting bool,
) []chunk {
	tokens, cursor := getTokensAndCursorForDecl(stream, span)
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
		t = cursor.NextSkippable()
	}
	return chunks
}

func getTokensAndCursorForDecl(stream *token.Stream, span report.Span) ([]token.Token, *token.Cursor) {
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
	// TODO: clarify why we are shaving off the last token
	return tokens[:len(tokens)-1], token.NewCursorAt(tokens[len(tokens)-1])
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
	for i, c := range b.chunks {
		if chunkCost := len(c.text) - lineLimit; chunkCost > 0 {
			cost += chunkCost
		}
		if c.splitKind == splitKindSoft && !outermostSplittableChunkSet {
			outermostSplittableChunk = i
			outermostSplittableChunkSet = true
		}
	}
	if cost > 0 {
		// No more splits are available, return as is.
		if !outermostSplittableChunkSet {
			return
		}
		b.chunks[outermostSplittableChunk].splitKind = splitKindHard
		if end, ok := b.softSplitDeps[outermostSplittableChunk]; ok {
			// If there is an end for this split, then we need to set the end to a hard split.
			// And we need to set the first indent.
			b.chunks[end].splitKind = splitKindHard
			var lastSeen chunk
			for _, c := range b.chunks[outermostSplittableChunk+1 : end] {
				if c.splitKind == splitKindSoft && lastSeen.splitKind != splitKindSoft {
					c.nestingLevel += 1
				}
				if c.splitKind == splitKindHard {
					c.nestingLevel += 1
				}
				lastSeen = c
			}
		}
		b.calculateSplits(lineLimit)
	}
}
