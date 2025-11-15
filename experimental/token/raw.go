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

	"github.com/bufbuild/protocompile/experimental/id"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
)

// Implementation notes:
//
// Let n := int(id). If n is zero, it is the nil token. If n is positive, it is
// a natural token, whose index is n - 1. If it is negative, it is a
// synthetic token, whose index is ^n.

// ID is the raw ID of a [Token] separated from its [Context].
//
// The zero value is reserved as a nil representation. All other values are
// opaque.
type ID = id.ID[Token]

// naturalIndex returns the index of this token in the natural stream.
func naturalIndex(t ID) int {
	if t.IsZero() {
		panic("protocompile/token: called naturalIndex on zero token")
	}
	if t < 0 {
		panic("protocompile/token: called naturalIndex on synthetic token")
	}
	// Need to subtract off one, because the zeroth
	// ID is used as a "missing" sentinel.
	return int(t) - 1
}

// syntheticIndex returns the index of this token in the synthetic stream.
func syntheticIndex(t ID) int {
	if t.IsZero() {
		panic("protocompile/token: called syntheticIndex on zero token")
	}
	if t > 0 {
		panic("protocompile/token: called syntheticIndex on natural token")
	}
	// Need to invert the bits, because synthetic tokens are
	// stored as negative numbers.
	return ^int(t)
}

// Constants for extracting the parts of tokenImpl.kindAndOffset.
const (
	kindMask     = 0b00_0_111
	isTreeMask   = 0b00_1_000
	treeKwMask   = 0b11_0_000
	offsetShift  = 6
	keywordShift = 4
)

// nat is the data of a token stored in a [Context].
type nat struct {
	// We store the end of the token, and the start is implicitly
	// given by the end of the previous token. We use the end, rather
	// than the start, it makes adding tokens one by one to the stream
	// easier, because once the token is pushed, its start and end are
	// set correctly, and don't depend on the next token being pushed.
	end uint32

	// This contains compressed metadata about a token in the following format.
	// (Bit ranges [a:b] are exclusive like Go slice syntax.)
	//
	// 1. Bits [0:3] is the Kind.
	// 2. Bit [3] is whether this is a non-leaf.
	//    a. If it is a non-leaf, bits [4:6] determine the Keyword value if
	//	     Kind is Punct, and bits [6:32] are a signed offset to the matching
	//       open/close.
	//    b. If it is a leaf, bits [4:12] are a Keyword value.
	//
	// TODO: One potential optimization for the tree representation is to use
	// fewer bits for kind, since in practice, it is only ever Punct or String.
	// We do not currently make this optimization because it seems that the
	// current 32 million maximum size for separating two tokens is probably
	// sufficient, for now.
	metadata int32
}

// Kind extracts the token's kind, which is stored.
func (t nat) Kind() Kind {
	return Kind(t.metadata & kindMask)
}

// Offset returns the offset from this token to its matching open/close, if any.
func (t nat) Offset() int {
	if t.metadata&isTreeMask == 0 {
		return 0
	}
	return int(t.metadata >> offsetShift)
}

// Keyword returns the keyword for this token, if it is an identifier.
func (t nat) Keyword() keyword.Keyword {
	if t.IsLeaf() {
		return keyword.Keyword(t.metadata >> keywordShift)
	}
	if t.Kind() != Punct {
		return keyword.Unknown
	}
	return keyword.Parens + keyword.Keyword((t.metadata&treeKwMask)>>keywordShift)
}

// IsLeaf checks whether this is a leaf token.
func (t nat) IsLeaf() bool {
	return t.Offset() == 0
}

// IsOpen checks whether this is a open token with a matching closer.
func (t nat) IsOpen() bool {
	return t.Offset() > 0
}

// IsClose checks whether this is a closer token with a matching opener.
func (t nat) IsClose() bool {
	return t.Offset() < 0
}

func (t nat) GoString() string {
	type nat struct {
		End     int
		Kind    Kind
		IsLeaf  bool
		Keyword keyword.Keyword
		Offset  int
	}

	return fmt.Sprintf("%#v", nat{int(t.end), t.Kind(), t.IsLeaf(), t.Keyword(), t.Offset()})
}

// Fuse marks a pair of tokens as their respective open and close.
//
// If open or close are synthetic or not currently a leaf, this function panics.
//
//nolint:predeclared,revive // For close.
func fuseImpl(diff int32, open, close *nat) {
	if diff <= 0 {
		panic("protocompile/token: called Fuse() with out-of-order")
	}

	compressKw := func(kw keyword.Keyword) int32 {
		_, _, fused := kw.Brackets()

		v := int32(fused - keyword.Parens)
		if v >= 0 && v < 4 {
			return v << keywordShift
		}

		return 0
	}

	open.metadata = diff<<offsetShift | compressKw(open.Keyword()) | isTreeMask | int32(open.Kind())
	close.metadata = -diff<<offsetShift | compressKw(close.Keyword()) | isTreeMask | int32(close.Kind())
}

// synth is the data of a synth token stored in a [Context].
type synth struct {
	text string
	kind Kind

	// Non-zero if this token has a matching other end. Whether this is
	// the opener or the closer is determined by whether children is
	// nil: it is nil for the closer.
	otherEnd ID
	children []ID
}

// Keyword returns the keyword for this token, if it is an identifier.
func (t synth) Keyword() keyword.Keyword {
	if !slicesx.Among(t.kind, Ident, Punct) {
		return keyword.Unknown
	}

	kw := keyword.Lookup(t.text)
	if !t.IsLeaf() {
		_, _, kw = kw.Brackets()
	}
	return kw
}

// IsLeaf checks whether this is a leaf token.
func (t synth) IsLeaf() bool {
	return t.otherEnd == 0
}

// IsOpen checks whether this is a open token with a matching closer.
func (t synth) IsOpen() bool {
	return !t.IsLeaf() && t.children != nil
}

// IsClose checks whether this is a closer token with a matching opener.
func (t synth) IsClose() bool {
	return !t.IsLeaf() && t.children == nil
}
