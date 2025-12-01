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
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/source"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/experimental/token/keyword"
)

func TestNilToken(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	var n token.Token
	assert.True(n.IsZero())
	assert.False(n.IsLeaf())
	assert.False(n.IsSynthetic())
	assert.Equal(token.Unrecognized, n.Kind())
}

func TestLeafTokens(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	s := &token.Stream{
		File: source.NewFile("test", "abc def ghi"),
	}

	abc := s.Push(3, token.Ident)
	s.Push(1, token.Space)
	def := s.Push(3, token.Ident)
	s.Push(1, token.Space)
	ghi := s.Push(3, token.Ident)

	assertIdent := func(tok token.Token, a, b int, text string) {
		s := tok.Span()
		assert.Equal(a, s.Start)
		assert.Equal(b, s.End)

		assert.False(tok.IsZero())
		assert.False(tok.IsSynthetic())
		assert.True(tok.IsLeaf())
		assert.Equal(text, tok.Text())
		assert.Equal(token.Ident, abc.Kind())
		tokensEq(t, slices.Collect(tok.Children().Rest()))
	}

	assertIdent(abc, 0, 3, "abc")
	assertIdent(def, 4, 7, "def")
	assertIdent(ghi, 8, 11, "ghi")

	jkl := s.NewIdent("jkl")
	assert.False(jkl.IsZero())
	assert.True(jkl.IsLeaf())
	assert.True(jkl.IsSynthetic())
	assert.Equal("jkl", jkl.Text())
	tokensEq(t, slices.Collect(jkl.Children().Rest()))
}

func TestTreeTokens(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	s := &token.Stream{
		File: source.NewFile("test", "abc(def(x), message)"),
	}

	_ = s.Push(3, token.Ident)
	open := s.Push(1, token.Keyword)
	def := s.Push(3, token.Ident)
	open2 := s.Push(1, token.Keyword)
	x := s.Push(1, token.Ident)
	close2 := s.Push(1, token.Keyword)
	token.Fuse(open2, close2)
	comma := s.Push(1, token.Keyword)
	s.Push(1, token.Space)
	message := s.PushKeyword(7, token.Ident, keyword.Message)
	close := s.Push(1, token.Keyword) //nolint:revive,predeclared
	token.Fuse(open, close)

	assert.False(open.IsLeaf())
	assert.False(open2.IsLeaf())
	assert.False(close.IsLeaf())
	assert.False(close2.IsLeaf())

	assert.Equal(token.Keyword, open.Kind())
	assert.Equal(token.Keyword, close.Kind())
	assert.Equal(token.Keyword, open2.Kind())
	assert.Equal(token.Keyword, close2.Kind())

	start, end := open2.StartEnd()
	tokenEq(t, start, open2)
	tokenEq(t, end, close2)
	start, end = close2.StartEnd()
	tokenEq(t, start, open2)
	tokenEq(t, end, close2)

	start, end = open.StartEnd()
	tokenEq(t, start, open)
	tokenEq(t, end, close)
	start, end = close.StartEnd()
	tokenEq(t, start, open)
	tokenEq(t, end, close)

	tokensEq(t, slices.Collect(open2.Children().Rest()), x)
	tokensEq(t, slices.Collect(close2.Children().Rest()), x)

	tokensEq(t, slices.Collect(open.Children().Rest()), def, open2, comma, message)
	tokensEq(t, slices.Collect(close.Children().Rest()), def, open2, comma, message)

	open3 := s.NewPunct("(")
	close3 := s.NewPunct(")")
	s.NewFused(open3, close3, def, open2)

	assert.Equal(keyword.Message, message.Keyword())
	assert.Equal(keyword.Parens, open3.Keyword())
	assert.Equal(keyword.Parens, close3.Keyword())
	assert.False(open3.IsLeaf())
	assert.False(close3.IsLeaf())
	start, end = open3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)
	start, end = close3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)

	tokensEq(t, slices.Collect(close3.Children().Rest()), def, open2)
}

// tokenEq is the singular version of tokensEq.
func tokenEq(t *testing.T, a, b token.Token) {
	tokensEq(t, []token.Token{a}, b)
}

// tokensEq is a helper for comparing tokens that results in more readable printouts.
func tokensEq(t *testing.T, tokens []token.Token, expected ...token.Token) {
	a := make([]string, len(tokens))
	for i, t := range tokens {
		a[i] = t.String()
	}
	b := make([]string, len(expected))
	for i, t := range expected {
		b[i] = t.String()
	}
	assert.Equal(t, b, a)
}
