// Copyright 2020-2026 Buf Technologies, Inc.
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

package printer

import "github.com/bufbuild/protocompile/experimental/dom"

// Named tag helpers for common whitespace patterns. Single-tag helpers
// are package-level vars (allocated once). Multi-tag helpers are
// functions that push pre-cached vars into a sink.

var (
	tagSpace        = dom.Text(" ")
	tagNewline      = dom.Text("\n")
	tagBlankline    = dom.Text("\n\n")
	tagSoftbreak    = dom.TextIf(dom.Broken, "\n")
	tagSoftlineFlat = dom.TextIf(dom.Flat, " ")
)

// softline pushes a space-if-flat, newline-if-broken pair.
func softline(push dom.Sink) {
	push(tagSoftlineFlat, tagSoftbreak)
}

// blankline pushes one blank line (two newlines).
func blankline(push dom.Sink) {
	push(tagBlankline)
}
