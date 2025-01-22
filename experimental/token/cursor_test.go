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

package token_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/token"
)

func TestCursor(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := NewContext("abc(def(x), ghi)")
	s := ctx.Stream()

	abc := s.Push(3, token.Ident)
	open := s.Push(1, token.Punct)
	def := s.Push(3, token.Ident)
	open2 := s.Push(1, token.Punct)
	s.Push(1, token.Ident)
	close2 := s.Push(1, token.Punct)
	token.Fuse(open2, close2)
	comma := s.Push(1, token.Punct)
	space := s.Push(1, token.Space)
	ghi := s.Push(3, token.Ident)
	close := s.Push(1, token.Punct) //nolint:revive,predeclared
	token.Fuse(open, close)

	cursor := s.Cursor()
	tokenEq(t, abc, cursor.PeekSkippable())
	tokenEq(t, abc, cursor.Next())
	tokenEq(t, abc, cursor.BeforeSkippable())
	tokenEq(t, abc, cursor.Prev())
	_ = cursor.Next()
	tokenEq(t, open, cursor.Next())
	tokenEq(t, token.Zero, cursor.Next())
	tokenEq(t, close, cursor.Prev()) // Returns the close, not the open.
	tokenEq(t, abc, cursor.Prev())
	tokenEq(t, token.Zero, cursor.Prev())
	tokenEq(t, abc, cursor.Next())

	// Seek to close.
	assert.True(cursor.Seek(close.ID()))
	tokenEq(t, abc, cursor.Prev())
	tokenEq(t, abc, cursor.Next())
	tokenEq(t, open, cursor.Next())
	tokenEq(t, token.Zero, cursor.Next())

	// Seek to an internal token.
	assert.True(cursor.Seek(open2.ID()))
	tokenEq(t, def, cursor.Prev())
	tokenEq(t, def, cursor.Next())
	tokenEq(t, open2, cursor.Next())
	tokenEq(t, comma, cursor.Next())
	tokenEq(t, space, cursor.PeekSkippable())
	tokenEq(t, ghi, cursor.Next()) // Cursor at ).
	tokenEq(t, abc, cursor.BeforeSkippable())
	tokenEq(t, close, cursor.Next())
	tokenEq(t, token.Zero, cursor.Next()) // At end.
	tokenEq(t, close, cursor.Prev())
	tokenEq(t, abc, cursor.Prev())
	tokenEq(t, token.Zero, cursor.Prev()) // At start.
}
