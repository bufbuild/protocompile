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

package ast_test

import (
	"fmt"
	"testing"

	"github.com/bufbuild/protocompile/experimental/ast"
	"github.com/bufbuild/protocompile/experimental/internal/astx"
	"github.com/bufbuild/protocompile/experimental/report"
	"github.com/bufbuild/protocompile/experimental/token"
	"github.com/bufbuild/protocompile/internal/ext/iterx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/stretchr/testify/assert"
)

func TestNaturalSplit(t *testing.T) {
	t.Parallel()

	c := ast.NewContext(report.NewFile("test.proto", "a.b./*idk*/(a.b.c )/*x*/.d"))

	// Manually lex the Path above.
	s := c.Stream()
	tokens := []token.Token{
		s.Push(1, token.Ident),   //  0 a
		s.Push(1, token.Punct),   //  1 .
		s.Push(1, token.Ident),   //  2 b
		s.Push(1, token.Punct),   //  3 .
		s.Push(7, token.Comment), //  4 /*idk*/
		s.Push(1, token.Punct),   //  5 (
		s.Push(1, token.Ident),   //  6 a
		s.Push(1, token.Punct),   //  7 .
		s.Push(1, token.Ident),   //  8 b
		s.Push(1, token.Punct),   //  9 .
		s.Push(1, token.Ident),   // 10 c
		s.Push(1, token.Space),   // 11
		s.Push(1, token.Punct),   // 12 )
		s.Push(5, token.Comment), // 13 /*x*/
		s.Push(1, token.Punct),   // 14 .
		s.Push(1, token.Ident),   // 15 d
	}

	token.Fuse(tokens[5], tokens[12])

	path := astx.NewPath(c, tokens[0], tokens[15])
	components := [][2]token.Token{
		{token.Zero, tokens[0]},  // a
		{tokens[1], tokens[2]},   // .b
		{tokens[3], tokens[5]},   // .(a.b.c)
		{tokens[14], tokens[15]}, // .d
	}

	pathEq(t, path, components)

	start, end := path.Split(0)
	pathEq(t, start, [][2]token.Token{})
	pathEq(t, end, components)

	start, end = path.Split(2)
	pathEq(t, start, components[:2])
	pathEq(t, end, components[2:])
}

func TestSyntheticSplit(t *testing.T) {
	t.Parallel()

	ctx := ast.NewContext(report.NewFile("test.proto", "a.b.(a.b.c).d"))

	// Manually build this path: a.b.(a.b.c).d
	s := ctx.Stream()
	p := s.NewPunct(".")
	a := s.NewIdent("a")
	b := s.NewIdent("b")
	c := s.NewIdent("c")
	d := s.NewIdent("d")
	inner := ctx.Nodes().NewPath(
		ctx.Nodes().NewPathComponent(token.Zero, a),
		ctx.Nodes().NewPathComponent(p, b),
		ctx.Nodes().NewPathComponent(p, c),
	)
	fmt.Println(inner)
	extn := ctx.Nodes().NewExtensionComponent(p, inner)
	path := ctx.Nodes().NewPath(
		ctx.Nodes().NewPathComponent(token.Zero, a),
		ctx.Nodes().NewPathComponent(p, b),
		extn,
		ctx.Nodes().NewPathComponent(p, d),
	)
	fmt.Println(path)

	components := [][2]token.Token{
		{token.Zero, a},  // a
		{p, b},           // .b
		{p, extn.Name()}, // .(a.b.c)
		{p, d},           // .d
	}

	start, end := path.Split(0)
	pathEq(t, start, [][2]token.Token{})
	pathEq(t, end, components)

	start, end = path.Split(2)
	pathEq(t, start, components[:2])
	pathEq(t, end, components[2:])
}

func pathEq(t *testing.T, path ast.Path, want [][2]token.Token) {
	t.Helper()

	components := slicesx.Collect(iterx.Map(path.Components, func(pc ast.PathComponent) [2]token.Token {
		return [2]token.Token{pc.Separator(), pc.Name()}
	}))
	stringEq(t, components, want)
}

func stringEq[T any](t *testing.T, tokens []T, expected []T) {
	t.Helper()

	a := make([]string, len(tokens))
	for i, t := range tokens {
		a[i] = fmt.Sprint(t)
	}
	b := make([]string, len(expected))
	for i, t := range expected {
		b[i] = fmt.Sprint(t)
	}
	assert.Equal(t, b, a)
}
