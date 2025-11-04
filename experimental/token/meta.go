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
	stream := token.Context()
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

	stream := token.Context()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	if stream.meta == nil {
		stream.meta = make(map[ID]any)
	}

	meta, _ := stream.meta[token.ID()].(*M)
	if meta == nil {
		meta = new(M)
		stream.meta[token.ID()] = meta
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

	stream := token.Context()
	if token.nat() != nil && stream.frozen {
		panic("protocompile/token: attempted to mutate frozen stream")
	}

	meta, _ := stream.meta[token.ID()].(*M)
	if meta != nil {
		delete(stream.meta, token.ID())
	}
}
