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

package ast

import (
	"testing"

	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/stretchr/testify/assert"
)

func TestNilToken(t *testing.T) {
	assert := assert.New(t)

	var n Token
	assert.True(n.Nil())
	assert.False(n.IsLeaf())
	assert.False(n.IsSynthetic())
	assert.Equal(n.Kind(), TokenUnrecognized)
}

func TestLeafTokens(t *testing.T) {
	assert := assert.New(t)

	ctx := newContext(report.File{Path: "test", Text: "abc def ghi"})

	abc := ctx.PushToken(3, TokenIdent)
	ctx.PushToken(1, TokenSpace)
	def := ctx.PushToken(3, TokenIdent)
	ctx.PushToken(1, TokenSpace)
	ghi := ctx.PushToken(3, TokenIdent)

	assertIdent := func(tok Token, a, b int, text string) {
		start, end := tok.Span().Offsets()
		assert.Equal(a, start)
		assert.Equal(b, end)

		assert.False(tok.Nil())
		assert.False(tok.IsSynthetic())
		assert.True(tok.IsLeaf())
		assert.Equal(text, tok.Text())
		assert.Equal(TokenIdent, abc.Kind())
		tokensEq(t, collect(tok.Children().Iter))
	}

	assertIdent(abc, 0, 3, "abc")
	assertIdent(def, 4, 7, "def")
	assertIdent(ghi, 8, 11, "ghi")

	jkl := ctx.NewIdent("jkl")
	assert.False(jkl.Nil())
	assert.True(jkl.IsLeaf())
	assert.True(jkl.IsSynthetic())
	assert.Equal("jkl", jkl.Text())
	tokensEq(t, collect(jkl.Children().Iter))
}

func TestTreeTokens(t *testing.T) {
	assert := assert.New(t)

	ctx := newContext(report.File{Path: "test", Text: "abc(def(x), ghi)"})

	_ = ctx.PushToken(3, TokenIdent)
	open := ctx.PushToken(1, TokenPunct)
	def := ctx.PushToken(3, TokenIdent)
	open2 := ctx.PushToken(1, TokenPunct)
	x := ctx.PushToken(1, TokenIdent)
	close2 := ctx.PushToken(1, TokenPunct)
	ctx.FuseTokens(open2, close2)
	comma := ctx.PushToken(1, TokenPunct)
	space := ctx.PushToken(1, TokenSpace)
	ghi := ctx.PushToken(3, TokenIdent)
	close := ctx.PushToken(1, TokenPunct)
	ctx.FuseTokens(open, close)

	_ = space

	assert.True(!open.IsLeaf())
	assert.True(!open2.IsLeaf())
	assert.True(!close.IsLeaf())
	assert.True(!close2.IsLeaf())

	assert.Equal(TokenPunct, open.Kind())
	assert.Equal(TokenPunct, close.Kind())
	assert.Equal(TokenPunct, open2.Kind())
	assert.Equal(TokenPunct, close2.Kind())

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

	tokensEq(t, collect(open2.Children().Iter), x)
	tokensEq(t, collect(close2.Children().Iter), x)

	tokensEq(t, collect(open.Children().Iter), def, open2, comma, ghi)
	tokensEq(t, collect(close.Children().Iter), def, open2, comma, ghi)

	open3 := ctx.NewPunct("(")
	close3 := ctx.NewPunct(")")
	ctx.NewOpenClose(open3, close3, def, open2)

	assert.True(!open3.IsLeaf())
	assert.True(!close3.IsLeaf())
	start, end = open3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)
	start, end = close3.StartEnd()
	tokenEq(t, start, open3)
	tokenEq(t, end, close3)

	tokensEq(t, collect(open3.Children().Iter), def, open2)
	tokensEq(t, collect(close3.Children().Iter), def, open2)
}

// tokenEq is the singular version of tokensEq.
func tokenEq(t *testing.T, a, b Token) {
	tokensEq(t, []Token{a}, b)
}

// tokensEq is a helper for comparing tokens that results in more readable printouts.
func tokensEq(t *testing.T, tokens []Token, expected ...Token) {
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

// collect is a polyfill for [slices.Collect].
func collect[T any](iter func(func(T) bool)) (s []T) {
	iter(func(t T) bool {
		s = append(s, t)
		return true
	})
	return
}
