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

package dom_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bufbuild/protocompile/experimental/printer/dom"
	"github.com/bufbuild/protocompile/internal/golden"
)

type parser func(*testing.T, string) *dom.Dom

func TestDom(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:      "testdata",
		Refresh:   "PROTOCOMPILE_REFRESH",
		Extension: "",
		Outputs: []golden.Output{
			{Extension: "unformatted.out"},
			{Extension: "formatted.out"},
		},
	}
	pathToParser := map[string]parser{
		"testdata/paragraphs": paragraphs,
		"testdata/braces":     braces,
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		parser, ok := pathToParser[path]
		require.True(t, ok)

		d := parser(t, text)
		require.NotNil(t, d)
		outputs[0] = d.Output()
		require.Equal(t, text, outputs[0])
		d.Format(100, "  ") // 2 spaces for indents
		outputs[1] = d.Output()
	})
}

func paragraphs(t *testing.T, text string) *dom.Dom {
	d := dom.NewDom()

	reader := bytes.NewBufferString(text)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)

	var chunkText string
	var lastChunk *dom.Chunk
	chunks := dom.NewChunks()
	for err == nil {
		for _, r := range line {
			chunkText += string(r)
			switch r {
			case '.', ',':
				// Treat each sentence as a chunk within the group of chunks.
				lastChunk = newChunk(chunkText, dom.SplitKindSoft, true, 0)
				chunks.Insert(lastChunk)
				chunkText = ""
			case '\n':
				// Treat each paragraph as a group of chunks.
				// The last chunk in the group will always have a double split.
				if lastChunk != nil {
					lastChunk.SetSplitKind(dom.SplitKindDouble)
				}
				// Insert a chunk for the newline.
				chunks.Insert(newChunk(chunkText, dom.SplitKindUnknown, false, 0))
				d.AddChunks(chunks)
				chunks = dom.NewChunks()
				chunkText = ""
			}
		}
		line, err = reader.ReadString('\n')
	}
	return d
}

// TODO: this parser doesn't behave exactly that I'm hoping for.
func braces(t *testing.T, text string) *dom.Dom {
	d := dom.NewDom()
	reader := bytes.NewBufferString(text)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	var block string
	for err == nil {
		block += line
		if line == "\n" {
			chunks := parseChunks(t, bytes.NewBufferString(block), 0, false)
			last := chunks.LastNonWhitespaceOnlyChunk()
			if last != nil {
				last.SetSplitKind(dom.SplitKindDouble)
			}
			d.AddChunks(chunks)
			block = ""
		}
		line, err = reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				d.AddChunks(parseChunks(t, bytes.NewBufferString(block), 0, false))
			} else {
				require.NoError(t, err)
			}
		}
	}
	return d
}

func parseChunks(
	t *testing.T,
	block *bytes.Buffer,
	indentDepth int,
	closeBrace bool,
) *dom.Chunks {
	chunks := dom.NewChunks()
	var chunkText string
	r, _, err := block.ReadRune()
	if err != nil && errors.Is(err, io.EOF) {
		return chunks
	}
	require.NoError(t, err)
	for !errors.Is(err, io.EOF) {
		chunkText += string(r)
		switch r {
		case ')':
			// Check to see if this is method or conditional
			r, _, err = block.ReadRune()
			require.NoError(t, err) // Consider a ) followed by an immediate EOF as a bad input
			if r == ';' {
				// This a a method call, we add the ';', cut the chunk, and continue.
				chunkText += string(r)
				chunks.Insert(newChunk(chunkText, dom.SplitKindSoft, true, indentDepth))
				chunkText = ""
			} else {
				require.NoError(t, block.UnreadRune())
				parent := newChunk(chunkText, dom.SplitKindSoft, true, indentDepth)
				parent.SetChild(parseChunks(t, block, indentDepth, false))
				chunks.Insert(parent)
				chunkText = ""
			}
		case '{':
			open := newChunk(chunkText, dom.SplitKindSoft, true, indentDepth)
			open.SetChild(parseChunks(t, block, indentDepth+1, true))
			chunks.Insert(open)
			chunkText = ""
		case '}':
			// Return anything collected so far as a chunk
			if closeBrace {
				before, after, found := strings.Cut(chunkText, "}")
				require.True(t, found)
				require.Empty(t, after)
				chunks.Insert(newChunk(before, dom.SplitKindSoft, true, indentDepth))
				require.NoError(t, block.UnreadRune())
				return chunks
			}
			chunks.Insert(newChunk(chunkText, dom.SplitKindHard, false, indentDepth))
			return chunks
		case ' ':
			// If this is only a whitespace chunk
			if chunkText == " " {
				chunks.Insert(newChunk(chunkText, dom.SplitKindUnknown, false, indentDepth))
			} else {
				chunks.Insert(newChunk(chunkText, dom.SplitKindNever, true, indentDepth))
			}
			chunkText = ""
		}
		r, _, err = block.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Cut the final chunk, whatever it may be
				if chunkText != "" {
					chunks.Insert(newChunk(chunkText, dom.SplitKindSoft, false, indentDepth))
				}
			} else {
				require.NoError(t, err)
			}
		}
	}
	return chunks
}

func newChunk(
	chunkText string,
	splitKind dom.SplitKind,
	spaceIfUnsplit bool,
	indentDepth int,
) *dom.Chunk {
	c := dom.NewChunk()
	c.SetText(chunkText)
	c.SetSplitKind(splitKind)
	c.SetIndentDepth(indentDepth)
	switch splitKind {
	case dom.SplitKindSoft, dom.SplitKindNever:
		c.SetSpaceIfUnsplit(spaceIfUnsplit)
		c.SetSplitKindIfSplit(dom.SplitKindHard)
	}
	return c
}
