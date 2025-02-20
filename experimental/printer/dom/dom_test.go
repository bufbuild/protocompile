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
	"strings"
	"testing"

	"github.com/bufbuild/protocompile/internal/golden"
)

func TestDom(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:      "testdata",
		Refresh:   "PROTOCOMPILE_REFRESH",
		Extension: "",
		Outputs: []golden.Output{
			{Extension: "formatted.out"},
			{Extension: "unformatted.out"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		d := NewDom()
		chunk := NewChunk()
		var chunkText string
		var count int
		strings.Lines(text)(func(line string) bool {
			// Make arbitary 10 word chunks.
			for _, word := range strings.Split(line, space) {
				if cut, ok := strings.CutSuffix(word, "\n"); ok {
					// Last word in the line, we trim the whitespace, set the chunk, and reset.
					chunkText += cut
					chunk.SetText(chunkText)
					chunk.SetSplitKind(SplitKindHard)
					d.Insert(chunk)
					count = 0
					chunkText = ""
					chunk = NewChunk()
					continue
				}
				chunkText += word
				count++
				if count == 5 {
					chunk.SetText(chunkText)
					chunk.SetSplitKind(SplitKindSoft)
					chunk.SetSpaceWhenUnsplit(true)
					chunk.SetSplitKindIfSplit(SplitKindHard)
					d.Insert(chunk)
					count = 0
					chunkText = ""
					chunk = NewChunk()
					continue
				}
				chunkText += space
			}
			return true
		})
		outputs[1] = d.Output(false, 0, 0)
		outputs[0] = d.Output(true, 20, 2)
	})
}
