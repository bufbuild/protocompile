package printer

import (
	"github.com/bufbuild/protocompile/experimental/token"
)

type chunk struct {
	tokens       []token.Token
	ident        uint32
	nestingLevel uint32
	blockIndex   uint32
	block        *block
	hardSplit    bool
}

type block struct {
	chunks []chunk
}
