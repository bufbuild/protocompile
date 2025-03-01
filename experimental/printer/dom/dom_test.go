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
	"testing"

	"github.com/bufbuild/protocompile/experimental/printer/dom"
	"github.com/bufbuild/protocompile/internal/golden"
	"github.com/stretchr/testify/require"
)

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

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		d := parseText(t, bytes.NewBufferString(text), 0, false, nil)
		require.NotNil(t, d)
		outputs[0] = d.Output()
		require.Equal(t, text, outputs[0])
		d.Format(100, "  ") // 2 spaces for indents
		outputs[1] = d.Output()
	})
}

// Read the input text and parse with the following rules:
//   - Break off a chunk for:
//     -- At a '.'
//     -- At a ','
//     -- At a '\n'
//   - If consecutive spaces are found, we break the chunk off, and add a chunk for just
//     the whitespace.
//   - If a '(' is found, then break off the chunk, and all contents between '(' and ')'
//     are inserted as a child dom. The child dom will have a higher indent level. The ')'
//     will be a separate chunk. We do not format a space between '(' and its contents.
//   - If a '{' is found, then break off the chunk, and all contents between '{' and '}'
//     are inserted as a child dom. The child dom will have a higher indent level. The '}'
//     will be a separate chunk. When formatted, a space is added between '{' and its contents
//     if they are not hard split (separated by a newline).
//   - When breaking off a chunk, we check the next character. If it is a '\n', then we promote
//     the split (soft split -> hard split, hard split -> double split).
func parseText(
	t *testing.T,
	text *bytes.Buffer,
	indentDepth int,
	closeBrace bool,
	lastNonWhitespaceChunk *dom.Chunk,
) *dom.Dom {
	d := dom.NewDom()
	var chunkText string
	r, _, err := text.ReadRune()
	for !errors.Is(err, io.EOF) {
		switch r {
		case ' ':
			chunkText += string(r)
			// Read ahead to see if there are consecutive spaces. If yes, then return them as a
			// single new chunk.
			spacesChunk, err := spacesChunk(text)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			if spacesChunk != nil || errors.Is(err, io.EOF) {
				// This preceeds a space, so the splitKind is always soft and we follow with a space
				// if unsplit.
				chunk := newChunk(chunkText, indentDepth, dom.SplitKindSoft, dom.SplitKindHard, true)
				if !chunk.WhitespaceOnly() {
					lastNonWhitespaceChunk = chunk
				}
				d.Insert(chunk)
				if spacesChunk != nil {
					// If a non-whitespace only chunk was added before this, we need to adjust the split,
					// only if the last non-whitespace chunk's split kind of softer. Otherwise, we continue
					// respecting the split set earlier.
					if lastNonWhitespaceChunk != nil && lastNonWhitespaceChunk.SplitKind() < spacesChunk.SplitKind() {
						lastNonWhitespaceChunk.SetSplitKind(spacesChunk.SplitKind())
					}
					d.Insert(spacesChunk)
				}
				// Reset
				chunkText = ""
			}
		case '.', ',':
			chunkText += string(r)
			splitKind, spaceIfUnsplit, err := setSplitKind(text)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			chunk := newChunk(chunkText, indentDepth, splitKind, dom.SplitKindHard, spaceIfUnsplit)
			lastNonWhitespaceChunk = chunk
			d.Insert(chunk)
			// Reset
			chunkText = ""
		case '\n':
			if chunkText != "" {
				// This preceeds a newline, so the splitKind is always hard and there is no need to set
				// splitKindIfSplit, because it is already split.
				chunk := newChunk(chunkText, indentDepth, dom.SplitKindHard, dom.SplitKindUnknown, false)
				if !chunk.WhitespaceOnly() {
					lastNonWhitespaceChunk = chunk
				}
				d.Insert(chunk)
			} else {
				// Check the last non-whitespace only chunk set on the Dom, we need to adjust its split
				// to acount for the newline.
				if lastNonWhitespaceChunk != nil && lastNonWhitespaceChunk.SplitKind() < dom.SplitKindDouble {
					r, _, err := text.ReadRune()
					if err != nil && !errors.Is(err, io.EOF) {
						require.NoError(t, err)
					}
					if err == nil {
						if r == '\n' {
							lastNonWhitespaceChunk.SetSplitKind(lastNonWhitespaceChunk.SplitKind() + 1)
						}
						require.NoError(t, text.UnreadRune())
					}
				}
			}
			// Create newline chunk
			chunk := dom.NewChunk()
			chunk.SetText(string(r))
			d.Insert(chunk)
			// Reset
			chunkText = ""
		case '(', '{':
			chunkText += string(r)
			splitKind, spaceIfUnsplit, err := setSplitKind(text)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			if r == '(' {
				// Do not format a space after an unsplit (
				spaceIfUnsplit = false
			}
			chunk := newChunk(chunkText, indentDepth, splitKind, dom.SplitKindHard, spaceIfUnsplit)
			lastNonWhitespaceChunk = chunk
			chunk.SetChild(parseText(t, text, indentDepth+1, true, lastNonWhitespaceChunk))
			d.Insert(chunk)
			closeBrace = false
			// Reset
			chunkText = ""
		case ')', '}':
			if closeBrace {
				if chunkText != "" {
					chunk := newChunk(chunkText, indentDepth, dom.SplitKindSoft, dom.SplitKindHard, false)
					lastNonWhitespaceChunk = chunk
					d.Insert(chunk)
				}
				require.NoError(t, text.UnreadRune())
				return d
			}
			chunkText += string(r)
			// Account for the semicolon
			r, _, err = text.ReadRune()
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			if r == ';' {
				chunkText += string(r)
			} else if err == nil {
				require.NoError(t, text.UnreadRune())
			}
			splitKind, spaceIfUnsplit, err := setSplitKind(text)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			chunk := newChunk(chunkText, indentDepth, splitKind, dom.SplitKindHard, spaceIfUnsplit)
			lastNonWhitespaceChunk = chunk
			d.Insert(chunk)
			// Reset
			closeBrace = true
			chunkText = ""
		default:
			chunkText += string(r)
		}
		r, _, err = text.ReadRune()
	}
	return d
}

func newChunk(text string, indentDepth int, splitKind, splitKindIfSplit dom.SplitKind, spaceIfUnsplit bool) *dom.Chunk {
	chunk := dom.NewChunk()
	chunk.SetText(text)
	chunk.SetIndentDepth(indentDepth)
	chunk.SetSplitKind(splitKind)
	if splitKindIfSplit == dom.SplitKindHard || splitKindIfSplit == dom.SplitKindDouble {
		chunk.SetSplitKindIfSplit(splitKindIfSplit)
	}
	chunk.SetSpaceIfUnsplit(spaceIfUnsplit)
	return chunk
}

func setSplitKind(text *bytes.Buffer) (dom.SplitKind, bool, error) {
	var spaceIfUnsplit bool
	base := dom.SplitKindSoft
	r, _, err := text.ReadRune()
	if err != nil {
		return base, spaceIfUnsplit, err
	}
	if r == ' ' {
		spaceIfUnsplit = true
	}
	if r == '\n' {
		base++
	}
	return base, spaceIfUnsplit, text.UnreadRune()
}

func spacesChunk(text *bytes.Buffer) (*dom.Chunk, error) {
	var chunk *dom.Chunk
	var chunkText string
	r, _, err := text.ReadRune()
	if err != nil {
		return nil, err
	}
	splitKind := dom.SplitKindSoft
	for r == ' ' {
		chunkText += string(r)
		r, _, err = text.ReadRune()
		if err != nil {
			if chunkText != "" {
				// Set whatever we have and return
				chunk = dom.NewChunk()
				chunk.SetText(chunkText)
				chunk.SetSplitKind(splitKind)
			}
			return chunk, err
		}
	}
	if r == '\n' {
		splitKind = dom.SplitKindHard
	}
	if chunkText != "" {
		chunk = dom.NewChunk()
		chunk.SetText(chunkText)
		chunk.SetSplitKind(splitKind)
	}
	return chunk, text.UnreadRune()
}
