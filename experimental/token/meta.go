package token

import (
	"fmt"

	"github.com/bufbuild/protocompile/experimental/internal/tokenmeta"
)

// GetMeta returns the metadata value associated with this token. This function
// cannot be called outside of protocompile.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func GetMeta[M tokenmeta.Meta](token Token) *M {
	stream := token.Context().Stream()
	if meta, ok := stream.meta[token.ID()].(*M); ok {
		return meta
	}
	return nil
}

// MutateMeta is like [GetMeta], but it first initializes the meta value.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func MutateMeta[M tokenmeta.Meta](token Token) *M {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to MutateMeta: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	if stream.meta == nil {
		stream.meta = make(map[ID]any)
	}

	meta, _ := stream.meta[token.id].(*M)
	if meta == nil {
		meta = new(M)
		stream.meta[token.id] = meta
	}

	return meta
}

// ClearMeta clears the associated literal value of a token.
//
// Panics if the given token is zero, or if the token is natural and the stream
// is frozen.
//
// Note: this function wants to be a method of [Token], but cannot because it
// is generic.
func ClearMeta[M tokenmeta.Meta](token Token) {
	if token.IsZero() {
		panic(fmt.Sprintf("protocompile/token: passed zero token to ClearMeta: %s", token))
	}

	stream := token.Context().Stream()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	meta, _ := stream.meta[token.id].(*M)
	if meta != nil {
		delete(stream.meta, token.id)
	}
}
