package printer

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/token"
)

// TODO: chunk is...
type chunk struct {
	tokens       []token.Token
	rule         rule
	ident        uint32
	nestingLevel uint32
	blockIndex   uint32
	block        *block
	hardSplit    bool //new line
	double       bool // if hardSplit == false, then this can't be true
	softSplit    bool
}

// TODO: rule is...
type rule struct {
	// The token IDs to keep, ordered
	keep       []token.ID
	whitespace []insertWhitespace
}

// TODO: insertWhitespace is...
type insertWhitespace struct {
	// The ID of the token to insert the whitepsace after.
	after token.ID
	// The kind of whitespace to insert.
	token token.Token
}

// TODO: block is...
type block struct {
	chunks []chunk
}

func fileToBlocks(file ast.File) []block {
	decls := file.Decls()
	var blocks []block
	for i := 0; i < decls.Len(); i++ {
		decl := decls.At(i)
		blocks = append(blocks, blockForDecl(decl))
	}
	return blocks
}

func blockForDecl(decl ast.DeclAny) block {
	switch decl.Kind() {
	case ast.DeclKindEmpty:
	case ast.DeclKindSyntax:
		return syntaxBlock(decl.AsSyntax())
	case ast.DeclKindPackage:
	case ast.DeclKindImport:
	case ast.DeclKindDef:
	case ast.DeclKindBody:
	case ast.DeclKindRange:
	default:
		panic("ah")
	}
	return block{}
}

func syntaxBlock(decl ast.DeclSyntax) block {
	var chunks []chunk
	// Get all the tokens from the last skippable to the start of the decl
	// Create chunks for these with rules
	cursor := decl.Context().Stream().Cursor()
	preTokens := getTokensFromLastSkippable(cursor, decl.Keyword())
	for _, pre := range preTokens {
		// TODO create chunks
		chunks = append(chunks, chunk{
			tokens:    []token.Token{pre},
			softSplit: true,
		})
	}
	// Create a chunk for the declaration itself
	declTokens := getTokensFromStartToEndInclusive(cursor, decl.Keyword(), decl.Semicolon())
	chunks = append(chunks, chunk{
		tokens:    declTokens,
		hardSplit: true,
		double:    true,
	})

	// TODO: rules

	// There is only a single chunk in the syntax block, since we expect the syntax declaration
	// to only be in a single line.
	return block{chunks: chunks}
	// chunks for comments,
	// chunk for decl (keyword, space, equal, space, value, semicolon)
}

func getTokensFromLastSkippable(cursor *token.Cursor, start token.Token) []token.Token {
	cursor.Seek(start.ID())
	tok := cursor.UnpopSkippable()
	for tok.Kind().IsSkippable() {
		tok = cursor.UnpopSkippable()
	}
	var tokens []token.Token
	// This is the token we hit
	tok = cursor.PopSkippable()
	for {
		tok = cursor.PopSkippable()
		if tok.ID() == start.ID() {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
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

func getLastTokenForExpr(exprAny ast.ExprAny) token.Token {
	switch exprAny.Kind() {
	case ast.ExprKindInvalid:
		panic("ah!")
	case ast.ExprKindError:
		panic("unimplemented")
	case ast.ExprKindLiteral:
		return exprAny.AsLiteral().Token
	case ast.ExprKindPrefixed:
		panic("unimplemented")
	case ast.ExprKindPath:
		panic("unimplemented")
	case ast.ExprKindRange:
		panic("unimplemented")
	case ast.ExprKindArray:
		panic("unimplemented")
	case ast.ExprKindDict:
		panic("unimplemented")
	case ast.ExprKindField:
		panic("unimplemented")
	default:
		panic("ah!")
	}
}

// TODO: for testing only, get rid of this later
func blocksToString(blocks []block) string {
	var output string
	for _, block := range blocks {
		for _, chunk := range block.chunks {
			fmt.Println("@@@@@@@@@")
			for _, token := range chunk.tokens {
				fmt.Println("!!!!!!!!!!!!!")
				fmt.Println(token.Text(), token.ID(), token.Kind().IsSkippable())
				fmt.Println("!!!!!!!!!!!!!")
				output += token.Text()
			}
			fmt.Println("@@@@@@@@@")
		}
	}
	return output
}
