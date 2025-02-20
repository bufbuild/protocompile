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
	"testing"

	"github.com/bufbuild/protocompile/internal/golden"
)

func TestDom(t *testing.T) {
	t.Parallel()

	corpus := golden.Corpus{
		Root:    "testdata",
		Refresh: "PROTOCOMPILE_REFRESH",
		Outputs: []golden.Output{
			{Extension: "formatted.out"},
			{Extension: "unformatted.out"},
		},
	}

	corpus.Run(t, func(t *testing.T, path, text string, outputs []string) {
		d := NewDom()
		chunk := NewChunk()
		chunk.SetText(text)
		d.Insert(chunk)
		outputs[0] = d.Output(true, 80, 2)
		outputs[1] = d.Output(false, 0, 0)
	})
}
