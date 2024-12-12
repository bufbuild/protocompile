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

package token_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/iters"
)

type Context struct {
	S *token.Stream
}

func (c *Context) Stream() *token.Stream {
	return c.S
}

func NewContext(text string) *Context {
	ctx := new(Context)
	ctx.S = &token.Stream{
		Context: ctx,
		File:    report.NewFile("test", text),
	}
	return ctx
}

func TestNilToken(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	var n token.Token
	assert.True(n.Nil())
	assert.False(n.IsLeaf())
	assert.False(n.IsSynthetic())
	assert.Equal(token.Unrecognized, n.Kind())
}

func TestLeafTokens(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := NewContext("abc def ghi")
	s := ctx.Stream()

	abc := s.Push(3, token.Ident)
	s.Push(1, token.Space)
	def := s.Push(3, token.Ident)
	s.Push(1, token.Space)
	ghi := s.Push(3, token.Ident)

	assertIdent := func(tok token.Token, a, b int, text string) {
		s := tok.Span()
		assert.Equal(a, s.Start)
		assert.Equal(b, s.End)

		assert.False(tok.Nil())
		assert.False(tok.IsSynthetic())
		assert.True(tok.IsLeaf())
		assert.Equal(text, tok.Text())
		assert.Equal(token.Ident, abc.Kind())
		tokensEq(t, iters.Collect(tok.Children().Rest()))
	}

	assertIdent(abc, 0, 3, "abc")
	assertIdent(def, 4, 7, "def")
	assertIdent(ghi, 8, 11, "ghi")

	jkl := s.NewIdent("jkl")
	assert.False(jkl.Nil())
	assert.True(jkl.IsLeaf())
	assert.True(jkl.IsSynthetic())
	assert.Equal("jkl", jkl.Text())
	tokensEq(t, iters.Collect(jkl.Children().Rest()))
}

func TestTreeTokens(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := NewContext("abc(def(x), ghi)")
	s := ctx.Stream()

	_ = s.Push(3, token.Ident)
	open := s.Push(1, token.Punct)
	def := s.Push(3, token.Ident)
	open2 := s.Push(1, token.Punct)
	x := s.Push(1, token.Ident)
	close2 := s.Push(1, token.Punct)
	token.Fuse(open2, close2)
	comma := s.Push(1, token.Punct)
	s.Push(1, token.Space)
	ghi := s.Push(3, token.Ident)
	close := s.Push(1, token.Punct) //nolint:revive,predeclared
	token.Fuse(open, close)

	assert.False(open.IsLeaf())
	assert.False(open2.IsLeaf())
	assert.False(close.IsLeaf())
	assert.False(close2.IsLeaf())

	assert.Equal(token.Punct, open.Kind())
	assert.Equal(token.Punct, close.Kind())
	assert.Equal(token.Punct, open2.Kind())
	assert.Equal(token.Punct, close2.Kind())

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

	tokensEq(t, iters.Collect(open2.Children().Rest()), x)
	tokensEq(t, iters.Collect(close2.Children().Rest()), x)

	tokensEq(t, iters.Collect(open.Children().Rest()), def, open2, comma, ghi)
	tokensEq(t, iters.Collect(close.Children().Rest()), def, open2, comma, ghi)

	open3 := s.NewPunct("(")
	close3 := s.NewPunct(")")
	s.NewFused(open3, close3, def, open2)

	assert.False(open3.IsLeaf())
	assert.False(close3.IsLeaf())
	start, end = open3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)
	start, end = close3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)

	tokensEq(t, iters.Collect(close3.Children().Rest()), def, open2)
}

func TestCommentText(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx := NewContext(`
// Foo
// Bar
// Baz

// abcd xyz

/* Foo
   Bar
   Baz
*/

/** Foo
  * Bar
  * Baz */
`)
	s := ctx.Stream()

	s.Push(1, token.Space)

	c1 := s.Push(7, token.Comment)
	_ = s.Push(7, token.Comment)
	c2 := s.Push(7, token.Comment)
	s.Push(1, token.Space)
	token.Fuse(c1, c2)

	c3 := s.Push(12, token.Comment)
	s.Push(1, token.Space)

	c4 := s.Push(23, token.Comment)
	s.Push(2, token.Space)

	c5 := s.Push(26, token.Comment)

	assert.Equal([]string{" Foo", " Bar", " Baz"}, c1.CommentLines(true))
	assert.Equal([]string{" abcd xyz"}, c3.CommentLines(true))
	assert.Equal([]string{" Foo", "Bar", "Baz", ""}, c4.CommentLines(true))
	assert.Equal([]string{"* Foo", " Bar", " Baz "}, c5.CommentLines(true))

	assert.Equal([]string{"Foo", "Bar", "Baz"}, c1.CommentLines(false))
	assert.Equal([]string{"abcd xyz"}, c3.CommentLines(false))
	assert.Equal([]string{" Foo", "Bar", "Baz", ""}, c4.CommentLines(false))
	assert.Equal([]string{"Foo", "Bar", "Baz "}, c5.CommentLines(false))
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
