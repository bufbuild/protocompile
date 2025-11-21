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

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
)

func TestCursor(t *testing.T) {
	t.Parallel()

	// Create a token tree.
	s := &token.Stream{
		File: source.NewFile("test", "abc(def(x), ghi)"),
	}

	abc := s.Push(3, token.Ident)
	open := s.Push(1, token.Keyword)
	def := s.Push(3, token.Ident)
	open2 := s.Push(1, token.Keyword)
	x := s.Push(1, token.Ident)
	close2 := s.Push(1, token.Keyword)
	token.Fuse(open2, close2)
	comma := s.Push(1, token.Keyword)
	space := s.Push(1, token.Space)
	ghi := s.Push(3, token.Ident)
	close := s.Push(1, token.Keyword) //nolint:revive,predeclared
	token.Fuse(open, close)

	// Cursor at root.
	t.Run("root", func(t *testing.T) {
		t.Parallel()
		cursor := s.Cursor()
		tokenEq(t, abc, cursor.PeekSkippable())
		tokenEq(t, abc, cursor.Next())
		tokenEq(t, abc, cursor.PeekPrevSkippable())
		tokenEq(t, abc, cursor.Prev())
		_ = cursor.Next()
		tokenEq(t, open, cursor.Next())
		tokenEq(t, token.Zero, cursor.Next())
		tokenEq(t, close, cursor.Prev()) // Returns the close, not the open.
		tokenEq(t, abc, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, abc, cursor.Next())
	})

	// Cursor at inner definition.
	t.Run("inner", func(t *testing.T) {
		t.Parallel()
		cursor := token.NewCursorAt(def)
		tokenEq(t, def, cursor.Next())
		tokenEq(t, def, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, def, cursor.Next())
		tokenEq(t, open2, cursor.Next())
		tokenEq(t, comma, cursor.Next())
		tokenEq(t, space, cursor.PeekSkippable())
		tokenEq(t, ghi, cursor.Next())
		tokenEq(t, token.Zero, cursor.Next()) // At the closing ), close.
		tokenEq(t, ghi, cursor.Prev())
		tokenEq(t, space, cursor.PeekPrevSkippable())
		tokenEq(t, space, cursor.PrevSkippable())
		tokenEq(t, comma, cursor.PrevSkippable())
		tokenEq(t, close2, cursor.PrevSkippable())
		tokenEq(t, def, cursor.PrevSkippable())
		tokenEq(t, token.Zero, cursor.PrevSkippable()) // At the open (, open.
	})

	// Cursor escape.
	t.Run("escape", func(t *testing.T) {
		t.Parallel()
		cursor := token.NewCursorAt(x)
		tokenEq(t, x, cursor.Next())
		tokenEq(t, token.Zero, cursor.Next())
		tokenEq(t, token.Zero, cursor.Next())
		tokenEq(t, x, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, x, cursor.Next())
		tokenEq(t, x, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, token.Zero, cursor.Prev())
		tokenEq(t, x, cursor.NextSkippable())
		tokenEq(t, token.Zero, cursor.NextSkippable())
		tokenEq(t, x, cursor.PrevSkippable())
	})

	// Test JustAfter EOF.
	t.Run("eof", func(t *testing.T) {
		t.Parallel()
		cursor := token.NewCursorAt(abc)
		tokenEq(t, abc, cursor.NextSkippable())
		tokenEq(t, open, cursor.NextSkippable())
		tokenEq(t, token.Zero, cursor.Next())
		tokenEq(t, close, cursor.Prev())

		tok, span := cursor.SeekToEnd()
		t.Log(tok.Text())
		tokenEq(t, token.Zero, tok)
		assert.Len(t, s.Text(), span.Start)
		assert.Len(t, s.Text(), span.End)
	})

	// Test setting the cursor at the open brace
	t.Run("open", func(t *testing.T) {
		t.Parallel()
		cursor := token.NewCursorAt(open)
		tokenEq(t, open, cursor.NextSkippable())
		tokenEq(t, token.Zero, cursor.NextSkippable())
	})

	// Test setting the cursor at the close brace
	t.Run("close", func(t *testing.T) {
		t.Parallel()
		cursor := token.NewCursorAt(close)
		tokenEq(t, close, cursor.NextSkippable())
		tokenEq(t, token.Zero, cursor.NextSkippable())
	})
}
