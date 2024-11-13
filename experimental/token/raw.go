// Copyright 2020-2024 Buf Technologies, Inc.
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

	"github.com/bufbuild/protocompile/experimental/internal"
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
type ID int32

// In associated this token ID with a context. This allows token metadata,
// such as position, text, and kind, to be looked up.
//
// No checks are performed to validate that this ID came from this context; the
// caller is responsible for ensuring that themselves.
func (t ID) In(c Context) Token {
	if t == 0 {
		return Nil
	}
	return Token{internal.NewWith(c), t}
}

func (t ID) String() string {
	if t == 0 {
		return "Token(<nil>)"
	}
	if t < 0 {
		return fmt.Sprintf("Token(^%d)", ^int(t))
	}

	return fmt.Sprintf("Token(%d)", int(t)-1)
}

// Constants for extracting the parts of tokenImpl.kindAndOffset.
const (
	kindMask    = 0b111
	offsetShift = 3
)

// nat is the data of a token stored in a [Context].
type nat struct {
	// We store the end of the token, and the start is implicitly
	// given by the end of the previous token. We use the end, rather
	// than the start, it makes adding tokens one by one to the stream
	// easier, because once the token is pushed, its start and end are
	// set correctly, and don't depend on the next token being pushed.
	end           uint32
	kindAndOffset int32
}

// Kind extracts the token's kind, which is stored.
func (t nat) Kind() Kind {
	return Kind(t.kindAndOffset & kindMask)
}

// Offset returns the offset from this token to its matching open/close, if any.
func (t nat) Offset() int {
	return int(t.kindAndOffset >> offsetShift)
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

// Fuse marks a pair of tokens as their respective open and close.
//
// If open or close are synthetic or not currently a leaf, this function panics.
//
//nolint:predeclared // For close.
func fuseImpl(diff int32, open, close *nat) {
	if diff <= 0 {
		panic("protocompile/token: called Fuse() with out-of-order")
	}

	open.kindAndOffset |= diff << offsetShift
	close.kindAndOffset |= -diff << offsetShift
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

// IsLeaf checks whether this is a leaf token.
func (t synth) IsLeaf() bool {
	return t.otherEnd == 0
}

// IsLeaf checks whether this is a open token with a matching closer.
func (t synth) IsOpen() bool {
	return !t.IsLeaf() && t.children != nil
}

// IsLeaf checks whether this is a closer token with a matching opener.
func (t synth) IsClose() bool {
	return !t.IsLeaf() && t.children == nil
}
