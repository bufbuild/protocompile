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
// - clean up new compound body logic
// - finish all decl types
// - what does it mean to have no tokens for a decl...
// - debug various decls/defs
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
type addSpace func(token.Token) bool

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
				if t.ID() == syntax.Semicolon().ID() || spanWithinSpan(t.Span(), syntax.Value().Span()) {
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
				if t.ID() == pkg.Semicolon().ID() || spanWithinSpan(t.Span(), pkg.Path().Span()) {
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
				if t.ID() == imprt.Semicolon().ID() || spanWithinSpan(t.Span(), imprt.ImportPath().Span()) {
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
		return bodyChunks(stream, decl.AsBody(), applyFormatting, indentLevel)
	case ast.DeclKindRange:
		// TODO: implement
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
		message := decl.AsMessage()
		return compoundBody(stream, message.Span(), message.Body, applyFormatting, indentLevel)
	case ast.DefKindEnum:
		enum := decl.AsEnum()
		return compoundBody(stream, enum.Span(), enum.Body, applyFormatting, indentLevel)
	case ast.DefKindService:
		service := decl.AsService()
		return compoundBody(stream, service.Span(), service.Body, applyFormatting, indentLevel)
	case ast.DefKindExtend:
		// TODO: implement
	case ast.DefKindField:
		field := decl.AsField()
		return fieldChunks(
			stream,
			field.Span(),
			field.Tag.Span(),
			field.Semicolon,
			field.Options,
			applyFormatting,
			indentLevel,
		)
	case ast.DefKindOneof:
		oneof := decl.AsOneof()
		chunks := compoundBody(stream, oneof.Span(), oneof.Body, applyFormatting, indentLevel)
		return chunks
	case ast.DefKindGroup:
		// TODO: implement
	case ast.DefKindEnumValue:
		enumValue := decl.AsEnumValue()
		return fieldChunks(
			stream,
			enumValue.Span(),
			enumValue.Tag.Span(),
			enumValue.Semicolon,
			enumValue.Options,
			applyFormatting,
			indentLevel,
		)
	case ast.DefKindMethod:
		method := decl.AsMethod()
		return compoundBody(stream, method.Span(), method.Body, applyFormatting, indentLevel)
	case ast.DefKindOption:
		option := decl.AsOption()
		return optionChunks(stream, option.Span(), option.Option, applyFormatting, indentLevel)
	default:
		// This should never happen.
		panic("ah")
	}
	return nil
}

func fieldChunks(
	stream *token.Stream,
	fieldSpan report.Span,
	fieldTagSpan report.Span,
	semicolon token.Token,
	options ast.CompactOptions,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	// No options to handle, so we just process all the tokens like a single one liner
	if options.IsZero() {
		tokens, cursor := getTokensAndCursorForDecl(stream, fieldSpan)
		return oneLiner(
			tokens,
			cursor,
			func(t token.Token) bool {
				// We don't want to add a space after the semicolon
				// We also don't want to add a space after the tag
				// TODO
				if t.ID() == semicolon.ID() || spanWithinSpan(t.Span(), fieldTagSpan) {
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
	// TODO: this is similar but not exactly the same as a compound body, because it's not recursive.
	// Keep this separate for now.
	tokens, cursor := getTokensAndCursorFromStartToEnd(stream, fieldSpan, options.Span())
	chunks := oneLiner(
		tokens,
		cursor,
		func(t token.Token) bool {
			return t.ID() != options.Brackets().ID()
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
	defChunksIndex := len(chunks) - 1
	optionsIndentLevel := indentLevel
	if chunks[len(chunks)-1].splitKind != splitKindSoft {
		optionsIndentLevel++
	}
	optionChunks := optionsChunks(stream, options, applyFormatting, optionsIndentLevel)
	var closingBracketChunk chunk
	var closingBracket bool
	if !options.Brackets().IsZero() {
		closingBracketChunk = optionChunks[len(optionChunks)-1]
		closingBracket = true
		optionChunks = optionChunks[:len(optionChunks)-1]
	}
	if len(optionChunks) > 1 {
		for i := range optionChunks {
			var softSplitDeps []int
			for j := range optionChunks {
				if i == j {
					continue
				}
				softSplitDeps = append(softSplitDeps, j+len(chunks))
			}
			optionChunks[i].softSplitDeps = softSplitDeps
		}
	}
	chunks = append(chunks, optionChunks...)
	chunks[defChunksIndex].softSplitDeps = append(chunks[defChunksIndex].softSplitDeps, len(chunks)-1)
	if closingBracket {
		chunks = append(chunks, closingBracketChunk)
	}
	// TODO: there might be a better way to handle the semicolon
	chunks = append(chunks, oneLiner(
		[]token.Token{semicolon},
		token.NewCursorAt(semicolon),
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
}

func optionsChunks(
	stream *token.Stream,
	options ast.CompactOptions,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	var chunks []chunk
	seq.Values(options.Entries())(func(o ast.Option) bool {
		chunks = append(chunks, optionChunks(stream, o.Span(), o, applyFormatting, indentLevel)...)
		return true
	})
	if !options.Brackets().IsZero() {
		if indentLevel > 0 {
			indentLevel--
		}
		_, end := options.Brackets().StartEnd()
		chunks = append(chunks, oneLiner(
			[]token.Token{end},
			// TODO: pending fix to NewCursorAt
			token.NewCursorAt(end),
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
			indentLevel,
		)...)
	}
	return chunks
}

func optionChunks(
	stream *token.Stream,
	optionSpan report.Span,
	option ast.Option,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	tokens, cursor := getTokensAndCursorForDecl(stream, optionSpan)
	return oneLiner(
		tokens,
		cursor,
		func(t token.Token) bool {
			if spanWithinSpan(t.Span(), option.Path.Span()) || spanWithinSpan(t.Span(), option.Value.Span()) {
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

func bodyChunks(
	stream *token.Stream,
	decl ast.DeclBody,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	var chunks []chunk
	seq.Values(decl.Decls())(func(d ast.DeclAny) bool {
		// TODO: docs
		chunks = append(chunks, declChunks(stream, d, applyFormatting, indentLevel)...)
		return true
	})
	// TODO: figure out what are the edges of this
	// We are already handling the opening brace, so we just want to handle the closing brace here
	if !decl.Braces().IsZero() {
		if indentLevel > 0 {
			indentLevel--
		}
		_, end := decl.Braces().StartEnd()
		chunks = append(chunks, oneLiner(
			[]token.Token{end},
			// TODO: pending fix to NewCursorAt
			token.NewCursorAt(end),
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
			indentLevel,
		)...)
	}
	return chunks
}

func compoundBody(
	stream *token.Stream,
	span report.Span,
	body ast.DeclBody,
	applyFormatting bool,
	indentLevel uint32,
) []chunk {
	tokens, _ := getTokensAndCursorFromStartToEnd(stream, span, body.Span())
	chunks := oneLiner(
		tokens,
		nil,
		func(t token.Token) bool {
			// TODO: This is not true for all compound bodies
			return t.ID() != body.Braces().ID()
		},
		func(_ *token.Cursor) (splitKind, bool) {
			// For compound bodies, we figure out whether to split the first line
			// based on the whitespace between the open brace and then first body decl.
			// So we set the cursor to the first body decl, walk backwards until the open brace
			// and then go from there.
			var first report.Span
			var firstToken token.Token
			seq.Values(body.Decls())(func(d ast.DeclAny) bool {
				first = d.Span()
				return false
			})
			stream.All()(func(t token.Token) bool {
				if spanWithinSpan(t.Span(), first) {
					firstToken = t
					return false
				}
				return true
			})
			if firstToken.IsZero() {
				panic("AAAAAAAAA")
			}
			cursor := token.NewCursorAt(firstToken)
			// Walk back until the open brace
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
				if singleFound {
					break
				}
				if cursor.PeekPrevSkippable().IsZero() {
					break
				}
				t = cursor.PrevSkippable()
			}
			if commentFound {
				return splitKindNever, true
			}
			if doubleFound {
				return splitKindDouble, false
			}
			if singleFound {
				return splitKindHard, false
			}
			return splitKindSoft, false
		},
		applyFormatting,
		indentLevel,
	)
	defChunksIndex := len(chunks) - 1
	bodyChunks := bodyChunks(stream, body, applyFormatting, indentLevel+1)

	var closingBraceChunk chunk
	var closingBrace bool
	if !body.Braces().IsZero() {
		closingBraceChunk = bodyChunks[len(bodyChunks)-1]
		closingBrace = true
		bodyChunks = bodyChunks[:len(bodyChunks)-1]
	}
	if len(bodyChunks) > 1 {
		for i := range bodyChunks {
			var softSplitDeps []int
			for j := range bodyChunks {
				if i == j {
					continue
				}
				softSplitDeps = append(softSplitDeps, j+len(chunks))
			}
			bodyChunks[i].softSplitDeps = softSplitDeps
		}
	}
	chunks = append(chunks, bodyChunks...)
	chunks[defChunksIndex].softSplitDeps = append(chunks[defChunksIndex].softSplitDeps, len(chunks)-1)
	if closingBrace {
		chunks = append(chunks, closingBraceChunk)
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
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		if spanWithinSpan(t.Span(), span) {
			tokens = append(tokens, t)
		}
		// We are past the end, so no need to continue
		if t.Span().Start > span.End {
			return false
		}
		return true
	})
	// TODO: what does it mean to have no tokens?
	if tokens == nil {
		return nil, nil
	}
	return tokens, token.NewCursorAt(tokens[len(tokens)-1])
}

// get all the tokens from the start to the end (inclusive) and the cursor starting at the end.
func getTokensAndCursorFromStartToEnd(stream *token.Stream, start, end report.Span) ([]token.Token, *token.Cursor) {
	var tokens []token.Token
	stream.All()(func(t token.Token) bool {
		if spanWithinRange(t.Span(), start, end) {
			tokens = append(tokens, t)
		}
		// No need to continue if we've moved past the end span
		if t.Span().Start > end.Start {
			return false
		}
		return true
	})
	// TODO: what does it mean to have no tokens?
	if tokens == nil {
		return nil, nil
	}
	return tokens, token.NewCursorAt(tokens[len(tokens)-1])
}

// Check that the given span is within the bounds of the start and end spans.
func spanWithinRange(span, start, end report.Span) bool {
	return span.Start >= start.Start && span.Start <= end.Start
}

// Check that the given span is within the bounds of another span, inclusive.
func spanWithinSpan(span, base report.Span) bool {
	return span.Start >= base.Start && span.End <= base.End
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
		b.hardSplitChunk(dep)
	}
}
