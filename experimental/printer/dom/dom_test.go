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
	"bytes"
	"errors"
	"io"
	"testing"
	"unicode"

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
		d := parseText(bytes.NewBufferString(text))
		outputs[0] = d.Output(false, 0, 0)
		require.Equal(t, text, outputs[0])
		outputs[1] = d.Output(true, 60, 2)
	})
}

// Read the input text and parse with the following rules:
//   - If a newline is found, then we respect the newline by creating a chunk with a hard
//     split
//   - If a punctuation is found:
//   - If it is an opening paren, (, then we create a chunk with a soft split that is
//     indented when the parent is split, but not split when the parent is split. All
//     contents between ( and ) are parsed as children of that chunk. The children are will
//     have an increased indent level, and will be indented and split when the parent is
//     split.
//   - If it is a closing paren, ), then we create a chunk. If it is followed by a ";", then
//     we combine the two into a single chunk. The chunk will be indented but not split when
//     the parent is split. It will have a soft split unless followed by a new-line.
//   - If it is an opening brace, {, then we create a chunk that is indented when the parent
//     is split, but not split when the parent is split. The split will be set based on the
//     following character -- if it is followed by a new-line, it will be a hard split,
//     otherwise it will be a soft split. All contents between { and } are parsed as children
//     of that chunk. The children will have an increased indent level, be indented if the
//     parent has a hard split set, and will be indented and split when the parent is split.
//   - If it is a closing brace, }, then we create a chunk that is split when the parent is
//     split, but not indented when the parent is split, and will have a hard split unless
//     followed by a double new line, in which case, it will have a double split.
//   - If a full stop, ".", is found, then we create a chunk. If it is followed by a new
//     line, then it will have a hard split, otherwise it will have a soft split.
//   - Single whitespace following a word or punctuation will be preserved. The rest will
//     be treated as extraneous whitespace and created into their own chunk that will be
//     outputed only when unformatted.
//   - All other words and symbols will be added to the current chunk, and will be made into
//     arbitrarily large 5 word chunks if nothing else is found. Spaces will be preserved
//     for all text that follow a space.
func parseText(text *bytes.Buffer) *Dom {
	d := NewDom()
	chunk := NewChunk()
	var chunkText string
	var count int
	var last rune
	r, _, err := text.ReadRune()
	for !errors.Is(err, io.EOF) {
		switch r {
		case '(':
			chunkText += string(r)
			chunk.SetText(chunkText)
			chunk.SetSplitKind(SplitKindSoft)
			chunk.SetSpaceWhenUnsplit(true)
			chunk.SetSplitKindIfSplit(SplitKindHard)
			chunk.SetChild(parseText(parens(text)))
			d.Insert(chunk)
			// Reset
			count = 0
			chunk = NewChunk()
			chunkText = ""
		case ')':
			chunkText += string(r)
			splitKind := SplitKindSoft
			if r, _, err = text.ReadRune(); err != nil {
				if !errors.Is(err, io.EOF) {
					panic("unexpected error reading rune after )")
				}
				// If this is a EOF, this will be handled naturally by the Unread.
			} else {
				if r == ';' {
					chunkText += string(r)
				} else {
					if r == '\n' {
						splitKind = SplitKindHard
					}
					text.UnreadRune()
				}
			}
			chunk.SetText(chunkText)
			chunk.SetSplitKind(splitKind)
			chunk.SetSpaceWhenUnsplit(true)
			chunk.SetSplitKindIfSplit(SplitKindHard)
			chunk.SetSplitWithParent(false)
			chunk.SetIndentOnParentSplit(true)
			d.Insert(chunk)
			// Reset
			count = 0
			chunk = NewChunk()
			chunkText = ""
		case ' ':
			if unicode.IsSpace(last) {
				// Break off the last chunk, if there is any content, read ahead until the next
				// non-space character, collect them all up and create a single chunk.
				if chunkText != "" {
					chunk.SetText(chunkText)
					// Since we don't know what it is, we'll simply set no splits.
					chunk.SetSplitKind(SplitKindNever)
					d.Insert(chunk)
					// Reset
					count = 0
					chunk = NewChunk()
					chunkText = ""
				}
				chunkText += string(r)
				chunkText += spaces(text)
				chunk.SetText(chunkText)
				chunk.SetOnlyOutputUnformatted(true)
				d.Insert(chunk)
				// Reset
				count = 0
				chunk = NewChunk()
				chunkText = ""
			} else {
				chunkText += string(r)
				count++
				if count == 5 {
					chunk.SetText(chunkText)
					chunk.SetSplitKind(SplitKindSoft)
					chunk.SetSplitKindIfSplit(SplitKindHard)
					d.Insert(chunk)
					// Reset
					count = 0
					chunk = NewChunk()
					chunkText = ""
				}
			}
		case '.':
			chunkText += string(r)
			splitKind := SplitKindSoft
			if r, _, _ := text.ReadRune(); r == '\n' {
				splitKind = SplitKindHard
			}
			if err := text.UnreadRune(); err != nil {
				panic("failed to unread")
			}
			chunk.SetText(chunkText)
			chunk.SetSplitKind(splitKind)
			chunk.SetSplitKindIfSplit(SplitKindHard)
			d.Insert(chunk)
			// Reset
			count = 0
			chunk = NewChunk()
			chunkText = ""
		case '\n':
			// Break off there is any text
			if chunkText != "" {
				chunk.SetText(chunkText)
				chunk.SetSplitKind(SplitKindHard)
				d.Insert(chunk)
				chunk = NewChunk()
			}
			// Create newline chunk
			chunk.SetText(string(r))
			chunk.SetOnlyOutputUnformatted(true)
			d.Insert(chunk)
			// Reset
			count = 0
			chunk = NewChunk()
			chunkText = ""
		default:
			chunkText += string(r)
		}
		last = r
		r, _, err = text.ReadRune()
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return d
}

func parens(text *bytes.Buffer) *bytes.Buffer {
	var body string
	r, _, err := text.ReadRune()
	for r != ')' {
		body += string(r)
		r, _, err = text.ReadRune()
		if errors.Is(err, io.EOF) {
			panic("unexpected end hit before closing parens")
		}
	}
	// Unread the rune so we can parse the closing parens next
	if err := text.UnreadRune(); err != nil {
		panic("fail on unread")
	}
	return bytes.NewBufferString(body)
}

func spaces(text *bytes.Buffer) string {
	var spaces string
	r, _, err := text.ReadRune()
	for !errors.Is(err, io.EOF) {
		if !unicode.IsSpace(r) {
			break
		}
		spaces += string(r)
		r, _, err = text.ReadRune()
	}
	if err := text.UnreadRune(); err != nil {
		panic("fail on unread")
	}
	return spaces
}
