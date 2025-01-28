package printer

import (
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TODO: I am very bad at writing/explaining things. Here are some notes to convert into
// proper docs later.
//
// There is actually no reason not to pre-apply the rules for the chunks before inserting
// to the blocks. This makes sense because from the block's perspective, we only care about
// manipulating the chunks -- the chunks should already be pre-formatted.
//
// We can basically make this application process configurable for the AST printer.
//
// prefix chunks: these are the chunks from the last non-skippable token to the starting token
// of this current declaration. This is basically all the whitspace and/or comments between
// the start of this current declaration and the end of the last declaration.

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

// block is an ordered slice of chunks. A block represents
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

func fileToBlocks(file ast.File, applyFormatting bool) []block {
	decls := file.Decls()
	var blocks []block
	for i := 0; i < decls.Len(); i++ {
		decl := decls.At(i)
		blocks = append(blocks, blockForDecl(decl, applyFormatting))
	}
	return blocks
}

func blockForDecl(decl ast.DeclAny, applyFormatting bool) block {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration
	case ast.DeclKindSyntax:
		return syntaxBlock(decl.AsSyntax(), applyFormatting)
	case ast.DeclKindPackage:
		return packageBlock(decl.AsPackage(), applyFormatting)
	case ast.DeclKindImport:
		return importBlock(decl.AsImport(), applyFormatting)
	case ast.DeclKindDef:
		// return defBlock(decl.AsDef(), applyFormatting)
	case ast.DeclKindBody:
		// TODO: figure out how to handle this
	case ast.DeclKindRange:
	default:
		panic("ah")
	}
	return block{}
}

func syntaxBlock(decl ast.DeclSyntax, applyFormatting bool) block {
	chunks := parsePrefixChunks(decl.Keyword(), applyFormatting)
	// TODO: should this actually be based on the span start and end? o_o
	tokens, cursor := getTokensFromStartToEndInclusiveAndCursor(decl.Keyword(), decl.Semicolon())
	var text string
	if applyFormatting {
		valueToken := tokenForExprAny(decl.Value())
		for _, t := range tokens {
			// If we are applying formatting, we skip user-defined whitespace and format our own
			if t.Kind() == token.Space {
				continue
			}
			text += t.Text()
			if t.ID() == valueToken.ID() || t.ID() == decl.Semicolon().ID() {
				continue
			}
			text += " "
		}
	} else {
		for _, t := range tokens {
			text += t.Text()
		}
	}
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.NextSkippable())
	if splitKind == splitKindSoft {
		splitKind = splitKindNever
	}
	chunks = append(chunks, chunk{
		text:             text,
		splitKind:        splitKind,
		spaceWhenUnsplit: spaceWhenUnsplit,
	})
	return block{chunks: chunks}
}

func packageBlock(decl ast.DeclPackage, applyFormatting bool) block {
	chunks := parsePrefixChunks(decl.Keyword(), applyFormatting)
	tokens, cursor := getTokensFromStartToEndInclusiveAndCursor(decl.Keyword(), decl.Semicolon())
	var text string
	if applyFormatting {
		for _, t := range tokens {
			if t.Kind() == token.Space {
				continue
			}
			text += t.Text()
			// If the token span falls within the span of the path or if the token is the semicolon,
			// we do not add a space.
			if t.ID() == decl.Semicolon().ID() || checkSpanWithin(t.Span(), decl.Path().Span()) {
				continue
			}
			text += " "
		}
	} else {
		for _, t := range tokens {
			text += t.Text()
		}
	}
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.NextSkippable())
	if splitKind == splitKindSoft {
		splitKind = splitKindNever
	}
	chunks = append(chunks, chunk{
		text:             text,
		splitKind:        splitKind,
		spaceWhenUnsplit: spaceWhenUnsplit,
	})
	return block{chunks: chunks}
}

func importBlock(decl ast.DeclImport, applyFormatting bool) block {
	chunks := parsePrefixChunks(decl.Keyword(), applyFormatting)
	tokens, cursor := getTokensFromStartToEndInclusiveAndCursor(decl.Keyword(), decl.Semicolon())
	var text string
	if applyFormatting {
		for _, t := range tokens {
			if t.Kind() == token.Space {
				continue
			}
			text += t.Text()
			if t.ID() == decl.Semicolon().ID() || checkSpanWithin(t.Span(), decl.ImportPath().Span()) {
				continue
			}
			text += " "
		}
	} else {
		for _, t := range tokens {
			text += t.Text()
		}
	}
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.NextSkippable())
	if splitKind == splitKindSoft {
		splitKind = splitKindNever
	}
	chunks = append(chunks, chunk{
		text:             text,
		splitKind:        splitKind,
		spaceWhenUnsplit: spaceWhenUnsplit,
	})
	return block{chunks: chunks}
}

func defBlock(decl ast.DeclDef, applyFormatting bool) block {
	chunks := parsePrefixChunks(decl.Keyword(), applyFormatting)
	// var text string
	// Classify the definition type
	switch decl.Classify() {
	case ast.DefKindInvalid:
		// TODO: figure out what to do with invalid definitions
	case ast.DefKindMessage:

		return block{} // TODO: implement
	case ast.DefKindEnum:
		return block{} // TODO: implement
	case ast.DefKindService:
		return block{} // TODO: implement
	case ast.DefKindExtend:
		return block{} // TODO: implement
	case ast.DefKindField:
		return block{} // TODO: implement
	case ast.DefKindOneof:
		return block{} // TODO: implement
	case ast.DefKindGroup:
		return block{} // TODO: implement
	case ast.DefKindEnumValue:
		return block{} // TODO: implement
	case ast.DefKindMethod:
		return block{} // TODO: implement
	default:
		// This should never happen.
		panic("ah")
	}
	// TODO: add splitDefs
	return block{chunks: chunks}
}

func tokenForExprAny(exprAny ast.ExprAny) token.Token {
	switch exprAny.Kind() {
	case ast.ExprKindInvalid:
		// TODO: figure out how to handle invalid expressions
		return token.Zero
	case ast.ExprKindError:
		// TODO: figure out how to handle error expressions
		return token.Zero
	case ast.ExprKindLiteral:
		return exprAny.AsLiteral().Token
	case ast.ExprKindPrefixed:
		return token.Zero // TODO: implement
	case ast.ExprKindPath:
		return token.Zero // TODO: implement
	case ast.ExprKindRange:
		return token.Zero // TODO: implement
	case ast.ExprKindArray:
		return token.Zero // TODO: implement
	case ast.ExprKindDict:
		return token.Zero // TODO: implement
	case ast.ExprKindField:
		return token.Zero // TODO: implement
	default:
		// This should never happen
		panic("ah")
	}

}

func textForExprAny(exprAny ast.ExprAny) string {
	switch exprAny.Kind() {
	case ast.ExprKindInvalid:
		// TODO: figure out how to handle invalid expressions
		return ""
	case ast.ExprKindError:
		// TODO: figure out how to handle error expressions
		return ""
	case ast.ExprKindLiteral:
		return exprAny.AsLiteral().Text()
	case ast.ExprKindPrefixed:
		prefixed := exprAny.AsPrefixed()
		// TODO: figure out if we need to space the prefix
		return prefixed.Prefix().String() + textForExprAny(prefixed.Expr())
	case ast.ExprKindPath:
		return textForPath(exprAny.AsPath().Path)
	case ast.ExprKindRange:
		return "" // TODO: implement
	case ast.ExprKindArray:
		return "" // TODO: implement
	case ast.ExprKindDict:
		return "" // TODO: implement
	case ast.ExprKindField:
		return "" // TODO: implement
	default:
		// This should never happen
		panic("ah")
	}
}

func textForPath(p ast.Path) string {
	var text string
	p.Components(func(pc ast.PathComponent) bool {
		text += pc.Separator().Text() + pc.Name().Text()
		return true
	})
	return text
}

// TODO: improve performance (keep track of tokens as we are going backwards, so we don't need
// to iterate twice?
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
			splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.PeekSkippable())
			chunks = append(chunks, chunk{
				text:             t.Text(),
				splitKind:        splitKind,
				spaceWhenUnsplit: spaceWhenUnsplit,
			})
		case token.Unrecognized:
			// TODO: figure out what to do with unrecognized tokens.
		}
		t = cursor.NextSkippable()
	}
	return chunks
}

// To determine the split kind for a chunk, we check the next token. If the
// next token starts with a newline, then we must preserve that and set to a hardsplit.
// Otherwise it's a soft split.
func splitKindBasedOnNextToken(peekNext token.Token) (splitKind, bool) {
	if strings.HasPrefix(peekNext.Text(), "\n") {
		return splitKindHard, false
	}
	var spaceWhenUnsplit bool
	if strings.HasPrefix(peekNext.Text(), " ") {
		spaceWhenUnsplit = true
	}
	return splitKindSoft, spaceWhenUnsplit
}

// TODO: rename/clean-up
func getTokensFromStartToEndInclusiveAndCursor(start, end token.Token) ([]token.Token, *token.Cursor) {
	var tokens []token.Token
	cursor := token.NewCursorAt(start)
	t := cursor.NextSkippable()
	for t.ID() != end.ID() {
		tokens = append(tokens, t)
		t = cursor.NextSkippable()
	}
	return append(tokens, end), cursor
}

// TODO: rename/clean-up
// TODO: is just checking the starts enough?
func checkSpanWithin(have, want report.Span) bool {
	return want.Start >= have.Start && want.Start <= have.End
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
