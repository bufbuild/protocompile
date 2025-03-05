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

package dom

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunks(t *testing.T) {
	t.Parallel()

	t.Run("single", func(t *testing.T) {
		t.Parallel()

		unformatted := `while (  x ==               y) {
    method_a(   );
  method_b(); }`

		block := NewChunks()
		block.Insert(newChunk("while ", SplitKindNever, true, 0))
		block.Insert(newChunk("( ", SplitKindNever, false, 0))
		block.Insert(newChunk(" ", SplitKindUnknown, false, 0))
		block.Insert(newChunk("x ", SplitKindNever, true, 0))
		block.Insert(newChunk("== ", SplitKindNever, true, 0))
		block.Insert(newChunk("              ", SplitKindUnknown, false, 0))
		block.Insert(newChunk("y", SplitKindNever, false, 0))
		closeParen := newChunk(") ", SplitKindSoft, true, 0)

		braces := NewChunks()
		openBrace := newChunk("{", SplitKindSoft, true, 0)

		inner := NewChunks()
		inner.Insert(newChunk("\n", SplitKindUnknown, false, 1))
		inner.Insert(newChunk("    ", SplitKindUnknown, false, 1))
		inner.Insert(newChunk("method_a(", SplitKindNever, false, 1))
		inner.Insert(newChunk("   ", SplitKindUnknown, false, 1))
		inner.Insert(newChunk(");", SplitKindSoft, true, 1))
		inner.Insert(newChunk("\n", SplitKindUnknown, false, 1))
		inner.Insert(newChunk("  ", SplitKindUnknown, false, 1))
		inner.Insert(newChunk("method_b(", SplitKindNever, false, 1))
		inner.Insert(newChunk("); ", SplitKindSoft, true, 1))

		openBrace.SetChild(inner)
		braces.Insert(openBrace)
		braces.Insert(newChunk("}", SplitKindSoft, false, 0))

		closeParen.SetChild(braces)
		block.Insert(closeParen)

		require.Equal(t, unformatted, block.output())

		// Set 100 char line limit and 2-space indent string
		block.format(100, "  ")

		formattedUnsplit := `while (x == y) { method_a(); method_b(); }`
		require.Equal(t, formattedUnsplit, block.output())

		block.split()
		blockSplit := `while (x == y)
{ method_a(); method_b(); }
`
		require.Equal(t, blockSplit, block.output())

		braces.split()
		bracesSplit := `while (x == y)
{
  method_a(); method_b();
}
`
		require.Equal(t, bracesSplit, block.output())

		inner.split()
		innerSplit := `while (x == y)
{
  method_a();
  method_b();
}
`
		require.Equal(t, innerSplit, block.output())
	})

	t.Run("nested", func(t *testing.T) {
		t.Parallel()

		unformatted := `while (a == b) { method_a();         while (nested_c == nested_d) { while (nested_nested_e == nested_nested_f) { nested_nested_method_b(); while (nested_nested_nested_g ==      nested_nested_nested_h ) {         nested_nested_nested_method_c(); } nested_nested_method_d();       nested_nested_method_e(); } nested_method_f(   ); } method_g(); }`

		block := NewChunks()
		block.Insert(newChunk("while ", SplitKindNever, true, 0))
		block.Insert(newChunk("(", SplitKindNever, false, 0))
		block.Insert(newChunk("a ", SplitKindNever, true, 0))
		block.Insert(newChunk("== ", SplitKindNever, true, 0))
		block.Insert(newChunk("b", SplitKindNever, false, 0))
		closeParen0 := newChunk(") ", SplitKindSoft, true, 0)

		braces0 := NewChunks()
		openBrace0 := newChunk("{ ", SplitKindSoft, true, 0)

		inner0 := NewChunks()
		inner0.Insert(newChunk("method_a(", SplitKindNever, false, 1))
		inner0.Insert(newChunk("); ", SplitKindSoft, true, 1))
		inner0.Insert(newChunk("        ", SplitKindUnknown, false, 1))
		inner0.Insert(newChunk("while ", SplitKindNever, true, 1))
		inner0.Insert(newChunk("(", SplitKindNever, false, 1))
		inner0.Insert(newChunk("nested_c ", SplitKindNever, true, 1))
		inner0.Insert(newChunk("== ", SplitKindNever, true, 1))
		inner0.Insert(newChunk("nested_d", SplitKindNever, false, 1))
		closeParen1 := newChunk(") ", SplitKindSoft, true, 1)

		braces1 := NewChunks()
		openBrace1 := newChunk("{ ", SplitKindSoft, true, 1)

		inner1 := NewChunks()
		inner1.Insert(newChunk("while ", SplitKindNever, true, 2))
		inner1.Insert(newChunk("(", SplitKindNever, false, 2))
		inner1.Insert(newChunk("nested_nested_e ", SplitKindNever, true, 2))
		inner1.Insert(newChunk("== ", SplitKindNever, true, 2))
		inner1.Insert(newChunk("nested_nested_f", SplitKindNever, false, 2))
		closeParen2 := newChunk(") ", SplitKindSoft, true, 2)

		braces2 := NewChunks()
		openBrace2 := newChunk("{ ", SplitKindSoft, true, 2)

		inner2 := NewChunks()
		inner2.Insert(newChunk("nested_nested_method_b(", SplitKindNever, false, 3))
		inner2.Insert(newChunk("); ", SplitKindSoft, true, 3))
		inner2.Insert(newChunk("while ", SplitKindNever, true, 3))
		inner2.Insert(newChunk("(", SplitKindNever, false, 3))
		inner2.Insert(newChunk("nested_nested_nested_g ", SplitKindNever, true, 3))
		inner2.Insert(newChunk("== ", SplitKindNever, true, 3))
		inner2.Insert(newChunk("     ", SplitKindUnknown, false, 3))
		inner2.Insert(newChunk("nested_nested_nested_h ", SplitKindNever, false, 3))
		closeParen3 := newChunk(") ", SplitKindSoft, true, 3)

		braces3 := NewChunks()
		openBrace3 := newChunk("{ ", SplitKindSoft, true, 3)

		inner3 := NewChunks()
		inner3.Insert(newChunk("        ", SplitKindUnknown, false, 4))
		inner3.Insert(newChunk("nested_nested_nested_method_c(", SplitKindNever, false, 4))
		inner3.Insert(newChunk("); ", SplitKindSoft, true, 4))

		openBrace3.SetChild(inner3)
		braces3.Insert(openBrace3)
		braces3.Insert(newChunk("} ", SplitKindSoft, true, 3))

		closeParen3.SetChild(braces3)
		inner2.Insert(closeParen3)
		inner2.Insert(newChunk("nested_nested_method_d(", SplitKindNever, false, 3))
		inner2.Insert(newChunk("); ", SplitKindSoft, true, 3))
		inner2.Insert(newChunk("      ", SplitKindUnknown, false, 3))
		inner2.Insert(newChunk("nested_nested_method_e(", SplitKindNever, false, 3))
		inner2.Insert(newChunk("); ", SplitKindSoft, true, 3))

		openBrace2.SetChild(inner2)
		braces2.Insert(openBrace2)
		braces2.Insert(newChunk("} ", SplitKindSoft, true, 2))

		closeParen2.SetChild(braces2)
		inner1.Insert(closeParen2)
		inner1.Insert(newChunk("nested_method_f(", SplitKindNever, false, 2))
		inner1.Insert(newChunk("   ", SplitKindUnknown, false, 2))
		inner1.Insert(newChunk("); ", SplitKindSoft, true, 2))

		openBrace1.SetChild(inner1)
		braces1.Insert(openBrace1)
		braces1.Insert(newChunk("} ", SplitKindSoft, true, 1))

		closeParen1.SetChild(braces1)
		inner0.Insert(closeParen1)
		inner0.Insert(newChunk("method_g(", SplitKindNever, false, 1))
		inner0.Insert(newChunk("); ", SplitKindSoft, true, 1))

		openBrace0.SetChild(inner0)
		braces0.Insert(openBrace0)
		braces0.Insert(newChunk("}", SplitKindSoft, false, 0))

		closeParen0.SetChild(braces0)
		block.Insert(closeParen0)

		require.Equal(t, unformatted, block.output())

		// Set math.MaxInt32 to line limit (basically ridiculously long/effectively no character limit).
		// Set 2-space indent string.
		block.format(math.MaxInt32, "  ")

		formattedUnsplit := `while (a == b) { method_a(); while (nested_c == nested_d) { while (nested_nested_e == nested_nested_f) { nested_nested_method_b(); while (nested_nested_nested_g == nested_nested_nested_h) { nested_nested_nested_method_c(); } nested_nested_method_d(); nested_nested_method_e(); } nested_method_f(); } method_g(); }`
		require.Equal(t, formattedUnsplit, block.output())

		block.split()
		blockSplit := `while (a == b)
{ method_a(); while (nested_c == nested_d) { while (nested_nested_e == nested_nested_f) { nested_nested_method_b(); while (nested_nested_nested_g == nested_nested_nested_h) { nested_nested_nested_method_c(); } nested_nested_method_d(); nested_nested_method_e(); } nested_method_f(); } method_g(); }
`
		require.Equal(t, blockSplit, block.output())

		braces0.split()
		braces0Split := `while (a == b)
{
  method_a(); while (nested_c == nested_d) { while (nested_nested_e == nested_nested_f) { nested_nested_method_b(); while (nested_nested_nested_g == nested_nested_nested_h) { nested_nested_nested_method_c(); } nested_nested_method_d(); nested_nested_method_e(); } nested_method_f(); } method_g();
}
`
		require.Equal(t, braces0Split, block.output())

		inner0Braces1Split := `while (a == b)
{
  method_a();
  while (nested_c == nested_d)
  {
    while (nested_nested_e == nested_nested_f) { nested_nested_method_b(); while (nested_nested_nested_g == nested_nested_nested_h) { nested_nested_nested_method_c(); } nested_nested_method_d(); nested_nested_method_e(); } nested_method_f();
  }
  method_g();
}
`
		inner0.split()
		braces1.split()
		require.Equal(t, inner0Braces1Split, block.output())

		inner1Braces2Split := `while (a == b)
{
  method_a();
  while (nested_c == nested_d)
  {
    while (nested_nested_e == nested_nested_f)
    {
      nested_nested_method_b(); while (nested_nested_nested_g == nested_nested_nested_h) { nested_nested_nested_method_c(); } nested_nested_method_d(); nested_nested_method_e();
    }
    nested_method_f();
  }
  method_g();
}
`
		inner1.split()
		braces2.split()
		require.Equal(t, inner1Braces2Split, block.output())

		fullySplit := `while (a == b)
{
  method_a();
  while (nested_c == nested_d)
  {
    while (nested_nested_e == nested_nested_f)
    {
      nested_nested_method_b();
      while (nested_nested_nested_g == nested_nested_nested_h)
      {
        nested_nested_nested_method_c();
      }
      nested_nested_method_d();
      nested_nested_method_e();
    }
    nested_method_f();
  }
  method_g();
}
`
		inner2.split()
		braces3.split()
		require.Equal(t, fullySplit, block.output())
		inner3.split() // This is a no-op since inner3 is a single line
		require.Equal(t, fullySplit, block.output())
	})
}

func newChunk(
	chunkText string,
	splitKind SplitKind,
	spaceIfUnsplit bool,
	indentDepth int,
) *Chunk {
	c := NewChunk()
	c.SetText(chunkText)
	c.SetSplitKind(splitKind)
	c.SetIndentDepth(indentDepth)
	switch splitKind {
	case SplitKindSoft, SplitKindNever:
		c.SetSpaceIfUnsplit(spaceIfUnsplit)
		c.SetSplitKindIfSplit(SplitKindHard)
	}
	return c
}
