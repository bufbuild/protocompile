package ast

import (
	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/seq"
	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

// Block is a block of expressions evaluated one after the other, wrapped in
// braces. Within a block, expressions must be separated by semicolons or
// newlines.
//
// # Grammar
//
// Block := `{` (Expr (`;` | `\n`))* Expr? `}`
type Block id.Node[Block, *File, *rawBlock]

type rawBlock struct {
	braces token.ID
	tags   id.DynSeq[ExprAny, ExprKind, *File]
}

// Braces returns the braces that surround this block.
func (b Block) Braces() token.Token {
	if b.IsZero() {
		return token.Zero
	}
	return id.Wrap(b.Context().Stream(), b.Raw().braces)
}

// Exprs returns an inserter over the expressions in this block.
func (b Block) Exprs() seq.Inserter[ExprAny] {
	var tags *id.DynSeq[ExprAny, ExprKind, *File]
	if !b.IsZero() {
		tags = &b.Raw().tags
	}
	return tags.Inserter(b.Context())
}

// Span implements [source.Spanner].
func (b Block) Span() source.Span {
	return b.Braces().Span()
}
