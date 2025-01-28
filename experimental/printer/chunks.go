package printer

import (
	"slices"
	"strings"

	"github.com/bufbuild/protocompile/experimental/ast"
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
	// TODO: implement
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
	cursor := file.Context().Stream().Cursor()
	decls := file.Decls()
	var blocks []block
	for i := 0; i < decls.Len(); i++ {
		decl := decls.At(i)
		blocks = append(blocks, blockForDecl(cursor, decl, applyFormatting))
	}
	return blocks
}

// TODO: take indent nesting levels
func blockForDecl(cursor *token.Cursor, decl ast.DeclAny, applyFormatting bool) block {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
		// TODO: figure out what to do with an empty declaration
	case ast.DeclKindSyntax:
		return syntaxBlock(cursor, decl.AsSyntax(), applyFormatting)
	case ast.DeclKindPackage:
		return packageBlock(cursor, decl.AsPackage(), applyFormatting)
	case ast.DeclKindImport:
		return importBlock(cursor, decl.AsImport(), applyFormatting)
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

func syntaxBlock(cursor *token.Cursor, decl ast.DeclSyntax, applyFormatting bool) block {
	chunks := parsePrefixChunks(cursor, decl.Keyword(), applyFormatting)
	var text string
	valueToken := tokenForExprAny(decl.Value())
	tokens := getTokensFromStartToEndInclusive(cursor, decl.Keyword(), decl.Semicolon())
	for _, t := range tokens {
		if !applyFormatting {
			// If we are not applying formatting, just print all the tokens
			text += t.Text()
			continue
		}
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
	cursor.Seek(decl.Semicolon().ID())
	// Seek sets the cursor to the given ID, so the first thing we pop is the thing we set.
	// TODO: we need to rethink some of the seek/unpop behaviours.
	cursor.PopSkippable()
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.PeekSkippable())
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

func packageBlock(cursor *token.Cursor, decl ast.DeclPackage, applyFormatting bool) block {
	chunks := parsePrefixChunks(cursor, decl.Keyword(), applyFormatting)
	var text string
	tokens := getTokensFromStartToEndInclusive(cursor, decl.Keyword(), decl.Semicolon())
	if applyFormatting {
		// Figure out the path range of tokens
		var pathTokens []token.ID
		decl.Path().Components(func(pc ast.PathComponent) bool {
			if !pc.Separator().IsZero() {
				pathTokens = append(pathTokens, pc.Separator().ID())
			}
			pathTokens = append(pathTokens, pc.Name().ID())
			return true
		})
		for _, t := range tokens {
			if t.Kind() == token.Space {
				continue
			}
			text += t.Text()
			// We don't add a space after path tokens or semicolon
			if slices.Contains(pathTokens, t.ID()) || t.ID() == decl.Semicolon().ID() {
				continue
			}
			text += " "
		}
	} else {
		// If formatting is not applied then we just simply take all the tokens and write them
		for _, t := range tokens {
			text += t.Text()
		}
	}
	cursor.Seek(decl.Semicolon().ID())
	cursor.PopSkippable()
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.PeekSkippable())
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

func importBlock(cursor *token.Cursor, decl ast.DeclImport, applyFormatting bool) block {
	chunks := parsePrefixChunks(cursor, decl.Keyword(), applyFormatting)
	var text string
	if applyFormatting {
		// TODO: handle empty modifier
		text = decl.Keyword().Text() + " " + decl.Modifier().Text() + " " + textForExprAny(decl.ImportPath()) + " " + decl.Semicolon().Text()
	} else {
		// Grab all tokens between the start and the end of the syntax declaration
		// TODO: we should actually grab all the tokens for the formatting version also, in case there are
		// comments in between. We only want to drop the whitespace.
		for _, t := range getTokensFromStartToEndInclusive(cursor, decl.Keyword(), decl.Semicolon()) {
			text += t.Text()
		}
	}
	cursor.Seek(decl.Semicolon().ID())
	cursor.PopSkippable()
	splitKind, spaceWhenUnsplit := splitKindBasedOnNextToken(cursor.PeekSkippable())
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
	// TODO: should we just pass a top-level cursor for the file?
	cursor := decl.Context().Stream().Cursor()
	// TODO: figure out if this works for all definitions
	chunks := parsePrefixChunks(cursor, decl.Keyword(), applyFormatting)
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

func parsePrefixChunks(
	cursor *token.Cursor,
	until token.Token,
	applyFormatting bool,
) []chunk {
	// Set the cursor to until. This is where we want to end.
	cursor.Seek(until.ID())
	// Walk backwards until we hit the last skippable token
	tok := cursor.UnpopSkippable()
	for tok.Kind().IsSkippable() {
		if cursor.UnpeekSkippable().IsZero() {
			break
		}
		tok = cursor.UnpopSkippable()
	}
	var chunks []chunk
	t := cursor.PopSkippable()
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
		t = cursor.PopSkippable()
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

func getTokensFromStartToEndInclusive(cursor *token.Cursor, start, end token.Token) []token.Token {
	var tokens []token.Token
	tok := cursor.Seek(start.ID())
	for {
		tok = cursor.PopSkippable()
		if tok.ID() == end.ID() {
			break
		}
		tokens = append(tokens, tok)
	}
	return append(tokens, end)
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
