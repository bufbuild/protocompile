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
	"fmt"
	"io"
	"strings"
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
		d := parseText(t, bytes.NewBufferString(text), 0, dom.SplitKindUnknown, false)
		require.NotNil(t, d)
		outputs[0] = d.Output()
		require.Equal(t, text, outputs[0])
		d.SetFormatting(100, 2)
		d.Format()
		outputs[1] = d.Output()
	})
}

// Read the input text and parse with the following rules:
//   - Break off a chunk for:
//     -- At a '.'
//     -- At a ','
//     -- At a '\n'
//     -- If consecutive spaces are found, we break the chunk off, and add a chunk for just
//     the whitespace.
//     -- If a '(' is found, then break off the chunk, and all contents between '(' and ')'
//     are inserted as a child dom. The child dom will have a higher indent level. The ')'
//     will be a separate chunk.
//     -- If a '{' is found, then break off the chunk, and all contents between '{' and '}'
//     are inserted as a child dom. The child dom will have a higher indent level. The '}'
//     will be a separate chunk.
//   - When breaking off a chunk, we check the next character. If it is a '\n', then we promote
//     the split (soft split -> hard split, hard split -> double split).
func parseText(t *testing.T, text *bytes.Buffer, indent uint32, lastSplitKind dom.SplitKind, closeBrace bool) *dom.Dom {
	d := dom.NewDom()
	chunk := dom.NewChunk()
	var chunkText string
	r, _, err := text.ReadRune()
	for !errors.Is(err, io.EOF) {
		switch r {
		case ' ':
			chunkText += string(r)
			// Read ahead to see if there are consecutive spaces. If yes, then return all of those
			// as a space chunk, insert, and reset.
			spacesChunk, err := spacesChunk(text)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			if spacesChunk != nil || errors.Is(err, io.EOF) {
				chunk.SetText(chunkText)
				chunk.SetIndent(indent)
				splitKind, _, err := setSplitKind(text, dom.SplitKindSoft)
				if err != nil && !errors.Is(err, io.EOF) {
					require.NoError(t, err)
				}
				chunk.SetSplitKind(splitKind)
				chunk.SetSpaceIfUnsplit(true)
				chunk.SetSplitKindIfSplit(dom.SplitKindHard)
				d.Insert(chunk)
				d.Insert(spacesChunk)
				// Reset
				chunk = dom.NewChunk()
				lastSplitKind = splitKind
				chunkText = ""
			}
		case '.', ',':
			chunkText += string(r)
			chunk.SetText(chunkText)
			chunk.SetIndent(indent)
			splitKind, spaceIfUnsplit, err := setSplitKind(text, dom.SplitKindSoft)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			chunk.SetSplitKind(splitKind)
			chunk.SetSpaceIfUnsplit(spaceIfUnsplit)
			chunk.SetSplitKindIfSplit(dom.SplitKindHard)
			d.Insert(chunk)
			// Reset
			chunk = dom.NewChunk()
			lastSplitKind = splitKind
			chunkText = ""
		case '\n':
			if chunkText != "" {
				chunk.SetText(chunkText)
				chunk.SetIndent(indent)
				splitKind, _, err := setSplitKind(text, dom.SplitKindHard)
				if err != nil && !errors.Is(err, io.EOF) {
					require.NoError(t, err)
				}
				chunk.SetSplitKind(splitKind)
				d.Insert(chunk)
			} else {
				last := d.LastNonWhitespaceChunk()
				if last != nil && last.SplitKind() != dom.SplitKindDouble {
					splitKind, _, err := setSplitKind(text, last.SplitKind())
					if err != nil && !errors.Is(err, io.EOF) {
						require.NoError(t, err)
					}
					last.SetSplitKind(splitKind)
				}
			}
			// Create newline chunk
			chunk = dom.NewChunk()
			chunk.SetText(string(r))
			d.Insert(chunk)
			// Reset
			chunk = dom.NewChunk()
			chunkText = ""
		case '(', '{':
			chunkText += string(r)
			chunk.SetText(chunkText)
			chunk.SetIndent(indent)
			splitKind, spaceIfUnsplit, err := setSplitKind(text, dom.SplitKindSoft)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			chunk.SetSplitKind(splitKind)
			if r == '(' {
				// Never have a space after a (
				spaceIfUnsplit = false
			}
			chunk.SetSpaceIfUnsplit(spaceIfUnsplit)
			chunk.SetSplitKindIfSplit(dom.SplitKindHard)
			chunk.SetChild(parseText(t, text, indent+1, splitKind, true))
			d.Insert(chunk)
			closeBrace = false
			// Reset
			chunk = dom.NewChunk()
			lastSplitKind = splitKind
			chunkText = ""
		case ')', '}':
			if closeBrace {
				if chunkText != "" {
					splitKind := dom.SplitKindSoft
					if strings.HasSuffix(chunkText, "\n") {
						splitKind = dom.SplitKindHard
					}
					if strings.HasSuffix(chunkText, "\n\n") {
						splitKind = dom.SplitKindDouble
					}
					chunk.SetText(chunkText)
					chunk.SetIndent(indent)
					chunk.SetSplitKind(splitKind)
					chunk.SetSpaceIfUnsplit(false)
					chunk.SetSplitKindIfSplit(dom.SplitKindHard)
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
			chunk.SetText(chunkText)
			chunk.SetIndent(indent)
			splitKind, spaceIfUnsplit, err := setSplitKind(text, dom.SplitKindSoft)
			if err != nil && !errors.Is(err, io.EOF) {
				require.NoError(t, err)
			}
			chunk.SetSplitKind(splitKind)
			chunk.SetSpaceIfUnsplit(spaceIfUnsplit)
			chunk.SetSplitKindIfSplit(dom.SplitKindHard)
			d.Insert(chunk)
			// Reset
			closeBrace = true
			chunk = dom.NewChunk()
			lastSplitKind = splitKind
			chunkText = ""
		default:
			chunkText += string(r)
		}
		r, _, err = text.ReadRune()
	}
	return d
}

func setSplitKind(text *bytes.Buffer, base dom.SplitKind) (dom.SplitKind, bool, error) {
	var spaceIfUnsplit bool
	if base == dom.SplitKindDouble {
		return base, spaceIfUnsplit, fmt.Errorf("cannot set %s as base split kind", dom.SplitKindDouble)
	}
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
	for r == ' ' {
		chunkText += string(r)
		r, _, err = text.ReadRune()
		if err != nil {
			// Set whatever we have and return
			chunk = dom.NewChunk()
			chunk.SetText(chunkText)
			return chunk, err
		}
	}
	if chunkText != "" {
		chunk = dom.NewChunk()
		chunk.SetText(chunkText)
	}
	return chunk, text.UnreadRune()
}
